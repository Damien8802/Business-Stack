package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"subscription-system/config"
	"subscription-system/database"
	"subscription-system/models"
	"subscription-system/services"
)

var notifier *services.NotificationService

// InitNotifier инициализирует сервис уведомлений (вызывается из main)
func InitNotifier(cfg *config.Config) {
	notifier = services.NewNotificationService(cfg)
}

// Customer представляет клиента в CRM
type Customer struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Email       string          `json:"email"`
	Phone       string          `json:"phone"`
	Company     string          `json:"company"`
	Status      string          `json:"status"`
	Responsible string          `json:"responsible"`
	Source      string          `json:"source"`
	Comment     string          `json:"comment"`
	UserID      string          `json:"user_id,omitempty"`
	LeadScore   float64         `json:"lead_score"`
	CreatedAt   time.Time       `json:"created_at"`
	LastSeen    time.Time       `json:"last_seen"`
	City        string          `json:"city,omitempty"`
	SocialMedia json.RawMessage `json:"social_media,omitempty"`
	Birthday    *time.Time      `json:"birthday,omitempty"`
	Notes       string          `json:"notes,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	TenantID    string          `json:"tenant_id,omitempty"`
}

// Deal представляет сделку в CRM
type Deal struct {
	ID              string     `json:"id"`
	CustomerID      *string    `json:"customer_id"`
	Title           string     `json:"title"`
	Value           float64    `json:"value"`
	Stage           string     `json:"stage"`
	Probability     int        `json:"probability"`
	Responsible     string     `json:"responsible"`
	Source          string     `json:"source"`
	Comment         string     `json:"comment"`
	UserID          string     `json:"user_id,omitempty"`
	ExpectedClose   *time.Time `json:"expected_close,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
	ProductCategory string     `json:"product_category,omitempty"`
	Discount        float64    `json:"discount,omitempty"`
	NextActionDate  *time.Time `json:"next_action_date,omitempty"`
	Tags            []string   `json:"tags,omitempty"`
	TenantID        string     `json:"tenant_id,omitempty"`
}

// Partner представляет партнера
type Partner struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Phone     string    `json:"phone"`
    Inn       string    `json:"inn"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
    TenantID  string    `json:"tenant_id,omitempty"`
}
// HistoryRecord представляет запись истории
type HistoryRecord struct {
	ID         string          `json:"id"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Action     string          `json:"action"`
	UserID     *string         `json:"user_id,omitempty"`
	Changes    json.RawMessage `json:"changes,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Tag представляет тег
type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

// Activity представляет активность
type Activity struct {
	ID           string     `json:"id"`
	EntityType   string     `json:"entity_type"`
	EntityID     string     `json:"entity_id"`
	ActivityType string     `json:"activity_type"`
	Content      string     `json:"content"`
	UserID       *string    `json:"user_id,omitempty"`
	UserName     *string    `json:"user_name,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Attachment представляет вложение
type Attachment struct {
	ID         string    `json:"id"`
	FileName   string    `json:"file_name"`
	FilePath   string    `json:"file_path"`
	FileSize   int64     `json:"file_size"`
	MimeType   string    `json:"mime_type"`
	UploadedBy *string   `json:"uploaded_by,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

const uploadDir = "./uploads/crm"

// ========== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ==========

// getPaginationParams извлекает page и page_size из запроса
func getPaginationParams(c *gin.Context) (page, pageSize int) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}

// getUserIDFromContext извлекает user_id из контекста
func getUserIDFromContext(c *gin.Context) string {
	userID, exists := c.Get("user_id")
	if !exists {
		userID, exists = c.Get("userID")
		if !exists {
			return ""
		}
	}
	if idStr, ok := userID.(string); ok {
		return idStr
	}
	if idUUID, ok := userID.(uuid.UUID); ok {
		return idUUID.String()
	}
	return ""
}

// getTenantIDFromContext извлекает tenant_id из контекста
func getTenantIDFromContext(c *gin.Context) string {
	tenantID := c.GetString("tenant_id")
	if tenantID != "" {
		return tenantID
	}

	if userVal, exists := c.Get("user"); exists {
		switch user := userVal.(type) {
		case *models.User:
			if user.TenantID != "" {
				return user.TenantID
			}
		case models.User:
			if user.TenantID != "" {
				return user.TenantID
			}
		}
	}
	return ""
}

// getRoleFromContext извлекает роль пользователя из контекста
func getRoleFromContext(c *gin.Context) string {
	role, exists := c.Get("role")
	if !exists {
		return "user"
	}
	if roleStr, ok := role.(string); ok {
		return roleStr
	}
	return "user"
}

// isAdmin проверяет, является ли пользователь администратором
func isAdmin(c *gin.Context) bool {
	role := getRoleFromContext(c)
	return role == "admin" || role == "owner" || role == "platform_owner"
}

// getUserFilterSQL возвращает SQL-условие для фильтрации по пользователю
func getUserFilterSQL(c *gin.Context, startArg int) (string, []interface{}) {
	userID := getUserIDFromContext(c)
	if isAdmin(c) || userID == "" {
		return "", nil
	}
	return fmt.Sprintf(" AND user_id = $%d", startArg), []interface{}{userID}
}

// ========== РАСЧЁТ ЛИД-СКОРА ==========

// calculateLeadScore вычисляет скор для клиента
func calculateLeadScore(ctx context.Context, customerID string) (float64, error) {
	var totalValue float64
	err := database.Pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(value), 0) FROM crm_deals WHERE customer_id = $1
	`, customerID).Scan(&totalValue)
	if err != nil {
		return 0, err
	}
	score := totalValue / 10000
	if score > 1 {
		score = 1
	}
	return score, nil
}

// updateLeadScore обновляет скор для клиента
func updateLeadScore(ctx context.Context, customerID string) error {
	score, err := calculateLeadScore(ctx, customerID)
	if err != nil {
		return err
	}
	_, err = database.Pool.Exec(ctx, `
		UPDATE crm_customers SET lead_score = $1 WHERE id = $2
	`, score, customerID)
	return err
}

// ========== ИСТОРИЯ ==========

// addHistory записывает действие в историю
func addHistory(ctx context.Context, entityType, entityID, action string, userID *string, changes interface{}) error {
	var changesJSON []byte
	var err error
	if changes != nil {
		changesJSON, err = json.Marshal(changes)
		if err != nil {
			return err
		}
	}

	_, err = database.Pool.Exec(ctx, `
		INSERT INTO crm_history (entity_type, entity_id, action, user_id, changes)
		VALUES ($1, $2, $3, $4, $5)
	`, entityType, entityID, action, userID, changesJSON)
	return err
}

// GetEntityHistory возвращает историю сущности
func GetEntityHistory(c *gin.Context) {
	entityType := c.Param("type")
	entityID := c.Param("id")

	if entityType != "customer" && entityType != "deal" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid entity type"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	if !admin {
		var ownerID string
		var err error
		if entityType == "customer" {
			err = database.Pool.QueryRow(c.Request.Context(),
				"SELECT user_id FROM crm_customers WHERE id = $1", entityID).Scan(&ownerID)
		} else {
			err = database.Pool.QueryRow(c.Request.Context(),
				"SELECT user_id FROM crm_deals WHERE id = $1", entityID).Scan(&ownerID)
		}
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Entity not found"})
			return
		}
		if ownerID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
	}

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id, entity_type, entity_id, action, user_id, changes, created_at
		FROM crm_history
		WHERE entity_type = $1 AND entity_id = $2
		ORDER BY created_at DESC
	`, entityType, entityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	history := make([]HistoryRecord, 0)
	for rows.Next() {
		var h HistoryRecord
		var userIDNull sql.NullString
		var changes []byte
		err := rows.Scan(&h.ID, &h.EntityType, &h.EntityID, &h.Action, &userIDNull, &changes, &h.CreatedAt)
		if err != nil {
			continue
		}
		if userIDNull.Valid {
			h.UserID = &userIDNull.String
		}
		h.Changes = json.RawMessage(changes)
		history = append(history, h)
	}

	c.JSON(http.StatusOK, history)
}

// ========== СТРАНИЦЫ ==========

// CRMHandler отображает страницу CRM
func CRMHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "crm.html", gin.H{
		"Title": "CRM система - Business Stack",
	})
}

// CalendarHandler отображает страницу календаря сделок
func CalendarHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "calendar.html", gin.H{
		"Title": "Календарь сделок - Business Stack",
	})
}

// CRMHealthHandler возвращает статус CRM
func CRMHealthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "operational",
		"crm":    "online",
		"time":   time.Now().Unix(),
	})
}

// ========== КЛИЕНТЫ ==========

// GetCustomers возвращает список клиентов с фильтрацией
func GetCustomers(c *gin.Context) {
    tenantID := getTenantIDFromContext(c)
    
    // Принудительная установка tenant_id для владельца
    if userVal, exists := c.Get("user"); exists {
        if user, ok := userVal.(*models.User); ok {
            if user.Email == "owner@businesstack.ru" || user.Email == "dev@businessstack.ru" {
                tenantID = "c5517fa9-ef93-49c5-aaf2-bc3a62c34253"
                log.Printf("🔧 GetCustomers: принудительно установлен tenant_id: %s", tenantID)
            }
        }
    }
    
    if tenantID == "" {
        log.Printf("❌ GetCustomers: tenant_id not found")
        c.JSON(http.StatusOK, gin.H{
            "data":        []Customer{},
            "total":       0,
            "page":        1,
            "page_size":   20,
            "total_pages": 0,
        })
        return
    }

    log.Printf("🔍 GetCustomers: tenantID = %s", tenantID)

    page, pageSize := getPaginationParams(c)
    offset := (page - 1) * pageSize

    // Подсчет общего количества
    var total int
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM crm_customers WHERE tenant_id = $1
    `, tenantID).Scan(&total)
    if err != nil {
        log.Printf("❌ GetCustomers count error: %v", err)
        c.JSON(http.StatusOK, gin.H{
            "data":        []Customer{},
            "total":       0,
            "page":        page,
            "page_size":   pageSize,
            "total_pages": 0,
        })
        return
    }

    // Получение данных
    query := `SELECT id, name, email, phone, company, status, responsible,
             source, comment, lead_score, created_at, last_seen, city,
             social_media, birthday, notes
             FROM crm_customers
             WHERE tenant_id = $1
             ORDER BY created_at DESC
             LIMIT $2 OFFSET $3`


    rows, err := database.Pool.Query(c.Request.Context(), query, tenantID, pageSize, offset)
    if err != nil {
        log.Printf("❌ GetCustomers query error: %v", err)
        c.JSON(http.StatusOK, gin.H{
            "data":        []Customer{},
            "total":       0,
            "page":        page,
            "page_size":   pageSize,
            "total_pages": 0,
        })
        return
    }
    defer rows.Close()

    customers := make([]Customer, 0)
    for rows.Next() {
        var cst Customer
        var socialMedia []byte
        var birthday sql.NullTime

        err := rows.Scan(&cst.ID, &cst.Name, &cst.Email, &cst.Phone, &cst.Company, &cst.Status,
            &cst.Responsible, &cst.Source, &cst.Comment, &cst.LeadScore, &cst.CreatedAt, &cst.LastSeen,
            &cst.City, &socialMedia, &birthday, &cst.Notes)
        if err != nil {
            log.Printf("❌ Scan error: %v", err)
            continue
        }

        if socialMedia != nil {
            cst.SocialMedia = json.RawMessage(socialMedia)
        }
        if birthday.Valid {
            cst.Birthday = &birthday.Time
        }
        customers = append(customers, cst)
    }

    log.Printf("✅ GetCustomers: найдено %d клиентов для tenant_id %s", len(customers), tenantID)

    c.JSON(http.StatusOK, gin.H{
        "data":        customers,
        "total":       total,
        "page":        page,
        "page_size":   pageSize,
        "total_pages": (total + pageSize - 1) / pageSize,
    })
}
// CreateCustomer создаёт нового клиента
func CreateCustomer(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tenant not found"})
		return
	}

	var req Customer
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := getUserIDFromContext(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if req.Email != "" {
		var exists bool
		err := database.Pool.QueryRow(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM crm_customers WHERE email = $1 AND tenant_id = $2)",
			req.Email, tenantID).Scan(&exists)
		if err != nil {
			log.Printf("❌ CreateCustomer check email error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking email"})
			return
		}
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "Клиент с таким email уже существует"})
			return
		}
	}

	var id string
	err := database.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO crm_customers (id, name, email, phone, company, status, responsible, source, 
			comment, user_id, tenant_id, created_at, last_seen, city, social_media, birthday, notes, lead_score)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW(), $11, $12, $13, $14, 0)
		RETURNING id
	`, req.Name, req.Email, req.Phone, req.Company, req.Status,
		req.Responsible, req.Source, req.Comment, userID, tenantID,
		req.City, req.SocialMedia, req.Birthday, req.Notes).Scan(&id)

	if err != nil {
		log.Printf("❌ CreateCustomer insert error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(req.Tags) > 0 {
		if err := updateEntityTags(c.Request.Context(), "customer", id, req.Tags); err != nil {
			log.Printf("⚠️ Ошибка сохранения тегов для клиента %s: %v", id, err)
		}
	}

	if err := updateLeadScore(c.Request.Context(), id); err != nil {
		log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", id, err)
	}

	if notifier != nil {
		notifier.NotifyCustomerCreated(req.Name, req.Email, req.Phone, req.Company, req.Responsible)
	}

	go addHistory(c.Request.Context(), "customer", id, "create", &userID, nil)

	c.JSON(http.StatusCreated, gin.H{"id": id, "success": true})
}

// UpdateCustomer обновляет данные клиента
func UpdateCustomer(c *gin.Context) {
	id := c.Param("id")
	var req Customer
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var ownerID string
	err := database.Pool.QueryRow(c.Request.Context(),
		"SELECT user_id FROM crm_customers WHERE id = $1", id).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Customer not found"})
		return
	}
	if !admin && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var oldData Customer
	var oldSocialMedia []byte
	var oldBirthday sql.NullTime
	err = database.Pool.QueryRow(c.Request.Context(), `
		SELECT name, email, phone, company, status, responsible, source, comment, 
			city, social_media, birthday, notes
		FROM crm_customers WHERE id = $1
	`, id).Scan(&oldData.Name, &oldData.Email, &oldData.Phone, &oldData.Company,
		&oldData.Status, &oldData.Responsible, &oldData.Source, &oldData.Comment,
		&oldData.City, &oldSocialMedia, &oldBirthday, &oldData.Notes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if oldSocialMedia != nil {
		oldData.SocialMedia = json.RawMessage(oldSocialMedia)
	}
	if oldBirthday.Valid {
		oldData.Birthday = &oldBirthday.Time
	}

	if req.Email != "" && req.Email != oldData.Email {
		tenantID := getTenantIDFromContext(c)
		var exists bool
		err := database.Pool.QueryRow(c.Request.Context(),
			"SELECT EXISTS(SELECT 1 FROM crm_customers WHERE email = $1 AND tenant_id = $2 AND id != $3)",
			req.Email, tenantID, id).Scan(&exists)
		if err != nil {
			log.Printf("❌ UpdateCustomer check email error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking email"})
			return
		}
		if exists {
			c.JSON(http.StatusConflict, gin.H{"error": "Клиент с таким email уже существует"})
			return
		}
	}

	_, err = database.Pool.Exec(c.Request.Context(), `
		UPDATE crm_customers
		SET name = $1, email = $2, phone = $3, company = $4, status = $5,
			responsible = $6, source = $7, comment = $8, last_seen = NOW(),
			city = $9, social_media = $10, birthday = $11, notes = $12
		WHERE id = $13
	`, req.Name, req.Email, req.Phone, req.Company, req.Status,
		req.Responsible, req.Source, req.Comment,
		req.City, req.SocialMedia, req.Birthday, req.Notes, id)

	if err != nil {
		log.Printf("❌ UpdateCustomer error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if req.Tags != nil {
		if err := updateEntityTags(c.Request.Context(), "customer", id, req.Tags); err != nil {
			log.Printf("⚠️ Ошибка обновления тегов для клиента %s: %v", id, err)
		}
	}

	if err := updateLeadScore(c.Request.Context(), id); err != nil {
		log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", id, err)
	}

	changes := make(map[string]interface{})
	if oldData.Name != req.Name {
		changes["name"] = map[string]string{"old": oldData.Name, "new": req.Name}
	}
	if oldData.Email != req.Email {
		changes["email"] = map[string]string{"old": oldData.Email, "new": req.Email}
	}
	if oldData.Phone != req.Phone {
		changes["phone"] = map[string]string{"old": oldData.Phone, "new": req.Phone}
	}
	if oldData.Company != req.Company {
		changes["company"] = map[string]string{"old": oldData.Company, "new": req.Company}
	}
	if oldData.Status != req.Status {
		changes["status"] = map[string]string{"old": oldData.Status, "new": req.Status}
	}
	if oldData.Responsible != req.Responsible {
		changes["responsible"] = map[string]string{"old": oldData.Responsible, "new": req.Responsible}
	}
	if oldData.Source != req.Source {
		changes["source"] = map[string]string{"old": oldData.Source, "new": req.Source}
	}
	if oldData.Comment != req.Comment {
		changes["comment"] = map[string]string{"old": oldData.Comment, "new": req.Comment}
	}
	if oldData.City != req.City {
		changes["city"] = map[string]string{"old": oldData.City, "new": req.City}
	}
	if string(oldData.SocialMedia) != string(req.SocialMedia) {
		changes["social_media"] = map[string]string{"old": string(oldData.SocialMedia), "new": string(req.SocialMedia)}
	}

	oldBirthdayStr := ""
	newBirthdayStr := ""
	if oldData.Birthday != nil {
		oldBirthdayStr = oldData.Birthday.Format("2006-01-02")
	}
	if req.Birthday != nil {
		newBirthdayStr = req.Birthday.Format("2006-01-02")
	}
	if oldBirthdayStr != newBirthdayStr {
		changes["birthday"] = map[string]string{"old": oldBirthdayStr, "new": newBirthdayStr}
	}

	if oldData.Notes != req.Notes {
		changes["notes"] = map[string]string{"old": oldData.Notes, "new": req.Notes}
	}

	if len(changes) > 0 {
		go addHistory(c.Request.Context(), "customer", id, "update", &userID, changes)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteCustomer удаляет клиента
func DeleteCustomer(c *gin.Context) {
	id := c.Param("id")
	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var ownerID string
	err := database.Pool.QueryRow(c.Request.Context(),
		"SELECT user_id FROM crm_customers WHERE id = $1", id).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Customer not found"})
		return
	}
	if !admin && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	go addHistory(c.Request.Context(), "customer", id, "delete", &userID, nil)

	_, err = database.Pool.Exec(c.Request.Context(), "DELETE FROM crm_customers WHERE id = $1", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// BatchDeleteCustomers массовое удаление клиентов
func BatchDeleteCustomers(c *gin.Context) {
	var ids []string
	if err := c.ShouldBindJSON(&ids); err != nil || len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	tx, err := database.Pool.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	if !admin {
		var count int
		err := tx.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM crm_customers 
			WHERE id = ANY($1) AND user_id != $2
		`, ids, userID).Scan(&count)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if count > 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only delete your own customers"})
			return
		}
	}

	for _, id := range ids {
		if err := addHistory(c.Request.Context(), "customer", id, "delete", &userID, nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "History error"})
			return
		}
	}

	_, err = tx.Exec(c.Request.Context(), "DELETE FROM crm_customers WHERE id = ANY($1)", ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "deleted": len(ids)})
}

// BatchUpdateCustomersStatus массовое обновление статуса клиентов
func BatchUpdateCustomersStatus(c *gin.Context) {
	var req struct {
		IDs    []string `json:"ids"`
		Status string   `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 || req.Status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	tx, err := database.Pool.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	if !admin {
		var count int
		err := tx.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM crm_customers 
			WHERE id = ANY($1) AND user_id != $2
		`, req.IDs, userID).Scan(&count)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if count > 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only update your own customers"})
			return
		}
	}

	for _, id := range req.IDs {
		changes := map[string]interface{}{
			"status": map[string]string{"new": req.Status},
		}
		if err := addHistory(c.Request.Context(), "customer", id, "update", &userID, changes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "History error"})
			return
		}
	}

	_, err = tx.Exec(c.Request.Context(), "UPDATE crm_customers SET status = $1 WHERE id = ANY($2)", req.Status, req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "updated": len(req.IDs)})
}

// ========== СДЕЛКИ ==========

// GetDeals возвращает список сделок с фильтрацией
func GetDeals(c *gin.Context) {
    tenantID := getTenantIDFromContext(c)
    
    // Принудительная установка tenant_id для владельца
    if userVal, exists := c.Get("user"); exists {
        if user, ok := userVal.(*models.User); ok {
            if user.Email == "owner@businesstack.ru" || user.Email == "dev@businessstack.ru" {
                tenantID = "c5517fa9-ef93-49c5-aaf2-bc3a62c34253"
                log.Printf("🔧 GetDeals: принудительно установлен tenant_id: %s", tenantID)
            }
        }
    }
    
    if tenantID == "" {
        log.Printf("❌ GetDeals: tenant_id not found")
        c.JSON(http.StatusOK, gin.H{
            "data":        []Deal{},
            "total":       0,
            "page":        1,
            "page_size":   20,
            "total_pages": 0,
        })
        return
    }

    log.Printf("🔍 GetDeals: tenantID = %s", tenantID)

    page, pageSize := getPaginationParams(c)
    offset := (page - 1) * pageSize

    // Подсчет общего количества
    var total int
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM crm_deals WHERE tenant_id = $1
    `, tenantID).Scan(&total)
    if err != nil {
        log.Printf("❌ GetDeals count error: %v", err)
        c.JSON(http.StatusOK, gin.H{
            "data":        []Deal{},
            "total":       0,
            "page":        page,
            "page_size":   pageSize,
            "total_pages": 0,
        })
        return
    }

    // Получение данных
    query := `SELECT id, customer_id, title, value, stage, probability, responsible,
             source, comment, expected_close, created_at, closed_at,
             product_category, discount, next_action_date
             FROM crm_deals
             WHERE tenant_id = $1
             ORDER BY created_at DESC
             LIMIT $2 OFFSET $3`

   rows, err := database.Pool.Query(c.Request.Context(), query, tenantID, pageSize, offset)
if err != nil {
    log.Printf("ERROR: %v", err)
}
    if err != nil {
        log.Printf("❌ GetDeals query error: %v", err)
        c.JSON(http.StatusOK, gin.H{
            "data":        []Deal{},
            "total":       0,
            "page":        page,
            "page_size":   pageSize,
            "total_pages": 0,
        })
        return
    }
    defer rows.Close()

    deals := make([]Deal, 0)
    for rows.Next() {
        var d Deal
        var nextActionDate sql.NullTime
        var customerID sql.NullString

        err := rows.Scan(&d.ID, &customerID, &d.Title, &d.Value, &d.Stage, &d.Probability,
            &d.Responsible, &d.Source, &d.Comment, &d.ExpectedClose, &d.CreatedAt, &d.ClosedAt,
            &d.ProductCategory, &d.Discount, &nextActionDate)
        if err != nil {
            log.Printf("❌ Scan error: %v", err)
            continue
        }
        if customerID.Valid {
            d.CustomerID = &customerID.String
        }
        if nextActionDate.Valid {
            d.NextActionDate = &nextActionDate.Time
        }
        deals = append(deals, d)
    }

    log.Printf("✅ GetDeals: найдено %d сделок для tenant_id %s", len(deals), tenantID)

    c.JSON(http.StatusOK, gin.H{
        "data":        deals,
        "total":       total,
        "page":        page,
        "page_size":   pageSize,
        "total_pages": (total + pageSize - 1) / pageSize,
    })
}
// CreateDeal создаёт новую сделку
func CreateDeal(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tenant not found"})
		return
	}

	var d Deal
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := getUserIDFromContext(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err := database.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO crm_deals (id, customer_id, title, value, stage, probability, responsible, 
			source, comment, expected_close, user_id, tenant_id, created_at, 
			product_category, discount, next_action_date)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), $12, $13, $14)
		RETURNING id
	`, d.CustomerID, d.Title, d.Value, d.Stage, d.Probability,
		d.Responsible, d.Source, d.Comment, d.ExpectedClose, userID, tenantID,
		d.ProductCategory, d.Discount, d.NextActionDate).Scan(&d.ID)

	if err != nil {
		log.Printf("❌ CreateDeal error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if len(d.Tags) > 0 {
		if err := updateEntityTags(c.Request.Context(), "deal", d.ID, d.Tags); err != nil {
			log.Printf("⚠️ Ошибка сохранения тегов для сделки %s: %v", d.ID, err)
		}
	}

	if d.CustomerID != nil {
		if err := updateLeadScore(c.Request.Context(), *d.CustomerID); err != nil {
			log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", *d.CustomerID, err)
		}
	}

	if notifier != nil {
		customerIDStr := ""
		if d.CustomerID != nil {
			customerIDStr = *d.CustomerID
		}
		notifier.NotifyDealCreated(d.Title, d.Value, d.Stage, d.Responsible, customerIDStr)
	}

	go addHistory(c.Request.Context(), "deal", d.ID, "create", &userID, nil)

	c.JSON(http.StatusCreated, d)
}

// UpdateDeal обновляет сделку
func UpdateDeal(c *gin.Context) {
	id := c.Param("id")
	var d Deal
	if err := c.ShouldBindJSON(&d); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var ownerID string
	err := database.Pool.QueryRow(c.Request.Context(),
		"SELECT user_id FROM crm_deals WHERE id = $1", id).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deal not found"})
		return
	}
	if !admin && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var oldData Deal
	var oldNextActionDate sql.NullTime
	err = database.Pool.QueryRow(c.Request.Context(), `
		SELECT title, value, stage, probability, responsible, source, comment, 
			expected_close, customer_id, product_category, discount, next_action_date
		FROM crm_deals WHERE id = $1
	`, id).Scan(&oldData.Title, &oldData.Value, &oldData.Stage, &oldData.Probability,
		&oldData.Responsible, &oldData.Source, &oldData.Comment, &oldData.ExpectedClose,
		&oldData.CustomerID, &oldData.ProductCategory, &oldData.Discount, &oldNextActionDate)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deal not found"})
		return
	}
	if oldNextActionDate.Valid {
		oldData.NextActionDate = &oldNextActionDate.Time
	}

	_, err = database.Pool.Exec(c.Request.Context(), `
		UPDATE crm_deals
		SET title = $1, value = $2, stage = $3, probability = $4,
			responsible = $5, source = $6, comment = $7, expected_close = $8,
			product_category = $9, discount = $10, next_action_date = $11
		WHERE id = $12
	`, d.Title, d.Value, d.Stage, d.Probability,
		d.Responsible, d.Source, d.Comment, d.ExpectedClose,
		d.ProductCategory, d.Discount, d.NextActionDate, id)

	if err != nil {
		log.Printf("❌ UpdateDeal error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if d.Tags != nil {
		if err := updateEntityTags(c.Request.Context(), "deal", id, d.Tags); err != nil {
			log.Printf("⚠️ Ошибка обновления тегов для сделки %s: %v", id, err)
		}
	}

	oldCustomerID := ""
	if oldData.CustomerID != nil {
		oldCustomerID = *oldData.CustomerID
	}
	newCustomerID := ""
	if d.CustomerID != nil {
		newCustomerID = *d.CustomerID
	}

	if oldCustomerID != newCustomerID {
		if oldCustomerID != "" {
			if err := updateLeadScore(c.Request.Context(), oldCustomerID); err != nil {
				log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", oldCustomerID, err)
			}
		}
		if newCustomerID != "" {
			if err := updateLeadScore(c.Request.Context(), newCustomerID); err != nil {
				log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", newCustomerID, err)
			}
		}
	} else if newCustomerID != "" {
		if err := updateLeadScore(c.Request.Context(), newCustomerID); err != nil {
			log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", newCustomerID, err)
		}
	}

	changes := make(map[string]interface{})
	if oldData.Title != d.Title {
		changes["title"] = map[string]string{"old": oldData.Title, "new": d.Title}
	}
	if oldData.Value != d.Value {
		changes["value"] = map[string]float64{"old": oldData.Value, "new": d.Value}
	}
	if oldData.Stage != d.Stage {
		changes["stage"] = map[string]string{"old": oldData.Stage, "new": d.Stage}
	}
	if oldData.Probability != d.Probability {
		changes["probability"] = map[string]int{"old": oldData.Probability, "new": d.Probability}
	}
	if oldData.Responsible != d.Responsible {
		changes["responsible"] = map[string]string{"old": oldData.Responsible, "new": d.Responsible}
	}
	if oldData.Source != d.Source {
		changes["source"] = map[string]string{"old": oldData.Source, "new": d.Source}
	}
	if oldData.Comment != d.Comment {
		changes["comment"] = map[string]string{"old": oldData.Comment, "new": d.Comment}
	}

	oldExpectedClose := ""
	if oldData.ExpectedClose != nil {
		oldExpectedClose = oldData.ExpectedClose.Format("2006-01-02")
	}
	newExpectedClose := ""
	if d.ExpectedClose != nil {
		newExpectedClose = d.ExpectedClose.Format("2006-01-02")
	}
	if oldExpectedClose != newExpectedClose {
		changes["expected_close"] = map[string]string{"old": oldExpectedClose, "new": newExpectedClose}
	}

	if oldCustomerID != newCustomerID {
		changes["customer_id"] = map[string]string{"old": oldCustomerID, "new": newCustomerID}
	}
	if oldData.ProductCategory != d.ProductCategory {
		changes["product_category"] = map[string]string{"old": oldData.ProductCategory, "new": d.ProductCategory}
	}
	if oldData.Discount != d.Discount {
		changes["discount"] = map[string]float64{"old": oldData.Discount, "new": d.Discount}
	}

	oldNextAction := ""
	if oldData.NextActionDate != nil {
		oldNextAction = oldData.NextActionDate.Format("2006-01-02")
	}
	newNextAction := ""
	if d.NextActionDate != nil {
		newNextAction = d.NextActionDate.Format("2006-01-02")
	}
	if oldNextAction != newNextAction {
		changes["next_action_date"] = map[string]string{"old": oldNextAction, "new": newNextAction}
	}

	if len(changes) > 0 {
		go addHistory(c.Request.Context(), "deal", id, "update", &userID, changes)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// UpdateDealStage обновляет стадию сделки
func UpdateDealStage(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Stage       string `json:"stage"`
		Probability int    `json:"probability"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var oldStage string
	var oldProb int
	var customerID string
	err := database.Pool.QueryRow(c.Request.Context(),
		"SELECT stage, probability, customer_id FROM crm_deals WHERE id = $1", id).Scan(&oldStage, &oldProb, &customerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deal not found"})
		return
	}

	var ownerID string
	err = database.Pool.QueryRow(c.Request.Context(),
		"SELECT user_id FROM crm_deals WHERE id = $1", id).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deal not found"})
		return
	}
	if !admin && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	_, err = database.Pool.Exec(c.Request.Context(), `
		UPDATE crm_deals
		SET stage = $1, probability = $2
		WHERE id = $3
	`, req.Stage, req.Probability, id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if customerID != "" {
		if err := updateLeadScore(c.Request.Context(), customerID); err != nil {
			log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", customerID, err)
		}
	}

	changes := make(map[string]interface{})
	if oldStage != req.Stage {
		changes["stage"] = map[string]string{"old": oldStage, "new": req.Stage}
	}
	if oldProb != req.Probability {
		changes["probability"] = map[string]int{"old": oldProb, "new": req.Probability}
	}
	if len(changes) > 0 {
		go addHistory(c.Request.Context(), "deal", id, "update", &userID, changes)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteDeal удаляет сделку
func DeleteDeal(c *gin.Context) {
	id := c.Param("id")
	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var customerID sql.NullString
	err := database.Pool.QueryRow(c.Request.Context(),
		"SELECT customer_id FROM crm_deals WHERE id = $1", id).Scan(&customerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deal not found"})
		return
	}

	var ownerID string
	err = database.Pool.QueryRow(c.Request.Context(),
		"SELECT user_id FROM crm_deals WHERE id = $1", id).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deal not found"})
		return
	}
	if !admin && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	go addHistory(c.Request.Context(), "deal", id, "delete", &userID, nil)

	_, err = database.Pool.Exec(c.Request.Context(), "DELETE FROM crm_deals WHERE id = $1", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if customerID.Valid && customerID.String != "" {
		if err := updateLeadScore(c.Request.Context(), customerID.String); err != nil {
			log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", customerID.String, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// BatchDeleteDeals массовое удаление сделок
func BatchDeleteDeals(c *gin.Context) {
	var ids []string
	if err := c.ShouldBindJSON(&ids); err != nil || len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	tx, err := database.Pool.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	if !admin {
		var count int
		err := tx.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM crm_deals 
			WHERE id = ANY($1) AND user_id != $2
		`, ids, userID).Scan(&count)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if count > 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only delete your own deals"})
			return
		}
	}

	rows, err := tx.Query(c.Request.Context(),
		"SELECT DISTINCT customer_id FROM crm_deals WHERE id = ANY($1) AND customer_id IS NOT NULL", ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	customerIDs := make([]string, 0)
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err == nil && cid != "" {
			customerIDs = append(customerIDs, cid)
		}
	}
	rows.Close()

	for _, id := range ids {
		if err := addHistory(c.Request.Context(), "deal", id, "delete", &userID, nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "History error"})
			return
		}
	}

	_, err = tx.Exec(c.Request.Context(), "DELETE FROM crm_deals WHERE id = ANY($1)", ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	for _, cid := range customerIDs {
		if err := updateLeadScore(c.Request.Context(), cid); err != nil {
			log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", cid, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "deleted": len(ids)})
}

// BatchUpdateDealsStage массовое обновление стадии сделок
func BatchUpdateDealsStage(c *gin.Context) {
	var req struct {
		IDs         []string `json:"ids"`
		Stage       string   `json:"stage"`
		Probability int      `json:"probability"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 || req.Stage == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	tx, err := database.Pool.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	if !admin {
		var count int
		err := tx.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM crm_deals 
			WHERE id = ANY($1) AND user_id != $2
		`, req.IDs, userID).Scan(&count)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if count > 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only update your own deals"})
			return
		}
	}

	rows, err := tx.Query(c.Request.Context(),
		"SELECT DISTINCT customer_id FROM crm_deals WHERE id = ANY($1) AND customer_id IS NOT NULL", req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	customerIDs := make([]string, 0)
	for rows.Next() {
		var cid string
		if err := rows.Scan(&cid); err == nil && cid != "" {
			customerIDs = append(customerIDs, cid)
		}
	}
	rows.Close()

	for _, id := range req.IDs {
		changes := map[string]interface{}{
			"stage":       map[string]string{"new": req.Stage},
			"probability": map[string]int{"new": req.Probability},
		}
		if err := addHistory(c.Request.Context(), "deal", id, "update", &userID, changes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "History error"})
			return
		}
	}

	_, err = tx.Exec(c.Request.Context(), `
		UPDATE crm_deals 
		SET stage = $1, probability = $2 
		WHERE id = ANY($3)
	`, req.Stage, req.Probability, req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	for _, cid := range customerIDs {
		if err := updateLeadScore(c.Request.Context(), cid); err != nil {
			log.Printf("⚠️ Не удалось обновить lead_score для клиента %s: %v", cid, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "updated": len(req.IDs)})
}

// BatchUpdateDealsResponsible массовое обновление ответственного
func BatchUpdateDealsResponsible(c *gin.Context) {
	var req struct {
		IDs         []string `json:"ids"`
		Responsible string   `json:"responsible"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	tx, err := database.Pool.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	if !admin {
		var count int
		err := tx.QueryRow(c.Request.Context(), `
			SELECT COUNT(*) FROM crm_deals 
			WHERE id = ANY($1) AND user_id != $2
		`, req.IDs, userID).Scan(&count)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		if count > 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "You can only update your own deals"})
			return
		}
	}

	for _, id := range req.IDs {
		changes := map[string]interface{}{
			"responsible": map[string]string{"new": req.Responsible},
		}
		if err := addHistory(c.Request.Context(), "deal", id, "update", &userID, changes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "History error"})
			return
		}
	}

	_, err = tx.Exec(c.Request.Context(), `
		UPDATE crm_deals 
		SET responsible = $1 
		WHERE id = ANY($2)
	`, req.Responsible, req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "updated": len(req.IDs)})
}

// ========== АНАЛИТИКА ==========

// GetCRMStats возвращает основную статистику CRM
func GetCRMStats(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tenant not found"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	userFilter := ""
	args := []interface{}{tenantID}
	if !admin && userID != "" {
		userFilter = " AND user_id = $2"
		args = append(args, userID)
	}

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT stage, COUNT(*) as count, COALESCE(SUM(value), 0) as total_value
		FROM crm_deals
		WHERE tenant_id = $1`+userFilter+`
		GROUP BY stage
		ORDER BY 
			CASE stage
				WHEN 'lead' THEN 1
				WHEN 'negotiation' THEN 2
				WHEN 'proposal' THEN 3
				WHEN 'closed_won' THEN 4
				WHEN 'closed_lost' THEN 5
				ELSE 6
			END
	`, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	type stageStat struct {
		Stage      string  `json:"stage"`
		Count      int     `json:"count"`
		TotalValue float64 `json:"total_value"`
	}
	stageStats := make([]stageStat, 0)
	for rows.Next() {
		var s stageStat
		if err := rows.Scan(&s.Stage, &s.Count, &s.TotalValue); err != nil {
			continue
		}
		stageStats = append(stageStats, s)
	}

	rows, err = database.Pool.Query(c.Request.Context(), `
		SELECT 
			TO_CHAR(date_trunc('month', created_at), 'YYYY-MM') as month,
			COUNT(*) as deals_created,
			COALESCE(SUM(value), 0) as total_value
		FROM crm_deals
		WHERE tenant_id = $1`+userFilter+`
			AND created_at >= NOW() - INTERVAL '12 months'
		GROUP BY date_trunc('month', created_at)
		ORDER BY month
	`, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	type monthlyStat struct {
		Month        string  `json:"month"`
		DealsCreated int     `json:"deals_created"`
		TotalValue   float64 `json:"total_value"`
	}
	monthlyStats := make([]monthlyStat, 0)
	for rows.Next() {
		var m monthlyStat
		if err := rows.Scan(&m.Month, &m.DealsCreated, &m.TotalValue); err != nil {
			continue
		}
		monthlyStats = append(monthlyStats, m)
	}

	var totalDeals, totalCustomers int
	var totalValue float64

	if !admin && userID != "" {
		database.Pool.QueryRow(c.Request.Context(),
			"SELECT COUNT(*) FROM crm_deals WHERE tenant_id = $1 AND user_id = $2", tenantID, userID).Scan(&totalDeals)
		database.Pool.QueryRow(c.Request.Context(),
			"SELECT COUNT(*) FROM crm_customers WHERE tenant_id = $1 AND user_id = $2", tenantID, userID).Scan(&totalCustomers)
		database.Pool.QueryRow(c.Request.Context(),
			"SELECT COALESCE(SUM(value), 0) FROM crm_deals WHERE tenant_id = $1 AND user_id = $2", tenantID, userID).Scan(&totalValue)
	} else {
		database.Pool.QueryRow(c.Request.Context(),
			"SELECT COUNT(*) FROM crm_deals WHERE tenant_id = $1", tenantID).Scan(&totalDeals)
		database.Pool.QueryRow(c.Request.Context(),
			"SELECT COUNT(*) FROM crm_customers WHERE tenant_id = $1", tenantID).Scan(&totalCustomers)
		database.Pool.QueryRow(c.Request.Context(),
			"SELECT COALESCE(SUM(value), 0) FROM crm_deals WHERE tenant_id = $1", tenantID).Scan(&totalValue)
	}

	c.JSON(http.StatusOK, gin.H{
		"stage_stats":     stageStats,
		"monthly_stats":   monthlyStats,
		"total_deals":     totalDeals,
		"total_customers": totalCustomers,
		"total_value":     totalValue,
	})
}

// GetSalesForecast возвращает прогноз продаж
func GetSalesForecast(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tenant not found"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	args := []interface{}{tenantID}
	userFilter := ""
	if !admin && userID != "" {
		userFilter = " AND user_id = $2"
		args = append(args, userID)
	}

	var avgMonthly float64
	queryAvg := `
		SELECT COALESCE(AVG(monthly_total), 0)
		FROM (
			SELECT DATE_TRUNC('month', created_at) as month, SUM(value) as monthly_total
			FROM crm_deals
			WHERE tenant_id = $1` + userFilter + `
				AND stage = 'closed_won'
				AND created_at >= NOW() - INTERVAL '6 months'
			GROUP BY DATE_TRUNC('month', created_at)
		) t
	`
	err := database.Pool.QueryRow(c.Request.Context(), queryAvg, args...).Scan(&avgMonthly)
	if err != nil {
		log.Printf("❌ GetSalesForecast avg error: %v", err)
	}

	var weightedForecast float64
	queryWeighted := `
		SELECT COALESCE(SUM(value * probability::float / 100), 0)
		FROM crm_deals
		WHERE tenant_id = $1` + userFilter + `
			AND stage NOT IN ('closed_won', 'closed_lost')
	`
	err = database.Pool.QueryRow(c.Request.Context(), queryWeighted, args...).Scan(&weightedForecast)
	if err != nil {
		log.Printf("❌ GetSalesForecast weighted error: %v", err)
	}

	var conversion float64
	queryConv := `
		SELECT 
			COALESCE(
				SUM(CASE WHEN stage = 'closed_won' THEN 1 ELSE 0 END) * 1.0 / 
				NULLIF(SUM(CASE WHEN stage IN ('closed_won', 'closed_lost') THEN 1 ELSE 0 END), 0),
				0
			) * 100
		FROM crm_deals
		WHERE tenant_id = $1` + userFilter
	err = database.Pool.QueryRow(c.Request.Context(), queryConv, args...).Scan(&conversion)
	if err != nil {
		log.Printf("❌ GetSalesForecast conversion error: %v", err)
	}

	months := make([]map[string]interface{}, 3)
	for i := 0; i < 3; i++ {
		monthTime := time.Now().AddDate(0, i+1, 0)
		monthStr := monthTime.Format("2006-01")
		months[i] = map[string]interface{}{
			"month": monthStr,
			"value": avgMonthly,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"avg_monthly_value": avgMonthly,
		"weighted_forecast": weightedForecast,
		"conversion":        conversion,
		"months":            months,
	})
}

// GetStageConversion возвращает конверсию по этапам
func GetStageConversion(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tenant not found"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	args := []interface{}{tenantID}
	userFilter := ""
	if !admin && userID != "" {
		userFilter = " AND user_id = $2"
		args = append(args, userID)
	}

	stages := []string{"lead", "negotiation", "proposal", "closed_won", "closed_lost"}
	result := make([]map[string]interface{}, 0, len(stages))

	var prevCount int
	for i, stage := range stages {
		var count int
		query := `SELECT COUNT(*) FROM crm_deals WHERE tenant_id = $1` + userFilter + ` AND stage = $` + strconv.Itoa(len(args)+1)
		queryArgs := append(args, stage)
		err := database.Pool.QueryRow(c.Request.Context(), query, queryArgs...).Scan(&count)
		if err != nil {
			log.Printf("❌ GetStageConversion error for stage %s: %v", stage, err)
			count = 0
		}

		percent := 0.0
		if i > 0 && prevCount > 0 {
			percent = float64(count) / float64(prevCount) * 100
		}

		result = append(result, gin.H{
			"stage":   stage,
			"count":   count,
			"percent": percent,
		})

		prevCount = count
	}

	c.JSON(http.StatusOK, result)
}

// ========== ТЕГИ ==========

// GetTags возвращает список всех тегов
func GetTags(c *gin.Context) {
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id, name, color, created_at FROM tags ORDER BY name
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	tags := make([]Tag, 0)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.CreatedAt); err != nil {
			continue
		}
		tags = append(tags, t)
	}
	c.JSON(http.StatusOK, tags)
}

// CreateTag создаёт новый тег
func CreateTag(c *gin.Context) {
	var req Tag
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}
	if req.Color == "" {
		req.Color = "#6c757d"
	}

	var id string
	err := database.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO tags (id, name, color, created_at)
		VALUES (gen_random_uuid(), $1, $2, NOW())
		RETURNING id
	`, req.Name, req.Color).Scan(&id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "success": true})
}

// UpdateTag обновляет тег
func UpdateTag(c *gin.Context) {
	id := c.Param("id")
	var req Tag
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
		return
	}
	if req.Color == "" {
		req.Color = "#6c757d"
	}

	_, err := database.Pool.Exec(c.Request.Context(), `
		UPDATE tags SET name = $1, color = $2 WHERE id = $3
	`, req.Name, req.Color, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteTag удаляет тег
func DeleteTag(c *gin.Context) {
	id := c.Param("id")
	_, err := database.Pool.Exec(c.Request.Context(), "DELETE FROM tags WHERE id = $1", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// getTagsForEntity возвращает теги сущности
func getTagsForEntity(ctx context.Context, entityType, entityID string) ([]string, error) {
	var tableName string
	switch entityType {
	case "customer":
		tableName = "customer_tags"
	case "deal":
		tableName = "deal_tags"
	default:
		return nil, fmt.Errorf("invalid entity type: %s", entityType)
	}

	rows, err := database.Pool.Query(ctx, fmt.Sprintf("SELECT tag_id FROM %s WHERE %s_id = $1", tableName, entityType), entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// updateEntityTags обновляет теги сущности
func updateEntityTags(ctx context.Context, entityType, entityID string, tagIDs []string) error {
	var tableName string
	switch entityType {
	case "customer":
		tableName = "customer_tags"
	case "deal":
		tableName = "deal_tags"
	default:
		return fmt.Errorf("invalid entity type: %s", entityType)
	}

	tx, err := database.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s_id = $1", tableName, entityType), entityID); err != nil {
		return err
	}

	for _, tagID := range tagIDs {
		if tagID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf("INSERT INTO %s (%s_id, tag_id) VALUES ($1, $2)", tableName, entityType), entityID, tagID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// ========== АКТИВНОСТИ ==========

// AddActivity добавляет активность
func AddActivity(c *gin.Context) {
	var req Activity
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.EntityType == "" || req.EntityID == "" || req.ActivityType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required fields"})
		return
	}

	userID := getUserIDFromContext(c)
	var userIDPtr *string
	if userID != "" {
		userIDPtr = &userID
	}

	var id string
	err := database.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO activities (id, entity_type, entity_id, activity_type, content, user_id, created_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, NOW())
		RETURNING id
	`, req.EntityType, req.EntityID, req.ActivityType, req.Content, userIDPtr).Scan(&id)
	if err != nil {
		log.Printf("❌ AddActivity error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "success": true})
}

// GetActivities возвращает активности для сущности
func GetActivities(c *gin.Context) {
	entityType := c.Param("type")
	entityID := c.Param("id")

	if entityType != "customer" && entityType != "deal" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid entity type"})
		return
	}

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT a.id, a.entity_type, a.entity_id, a.activity_type, a.content, a.user_id, u.email as user_name, a.created_at
		FROM activities a
		LEFT JOIN users u ON u.id = a.user_id
		WHERE a.entity_type = $1 AND a.entity_id = $2
		ORDER BY a.created_at DESC
	`, entityType, entityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	activities := make([]Activity, 0)
	for rows.Next() {
		var act Activity
		var userID, userName sql.NullString
		if err := rows.Scan(&act.ID, &act.EntityType, &act.EntityID, &act.ActivityType, &act.Content, &userID, &userName, &act.CreatedAt); err != nil {
			continue
		}
		if userID.Valid {
			act.UserID = &userID.String
		}
		if userName.Valid {
			act.UserName = &userName.String
		}
		activities = append(activities, act)
	}
	c.JSON(http.StatusOK, activities)
}

// DeleteActivity удаляет активность
func DeleteActivity(c *gin.Context) {
	activityID := c.Param("id")
	if activityID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "activity_id required"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	if userID == "" && !admin {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var entityType, entityID string
	var activityUserID sql.NullString
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT entity_type, entity_id, user_id FROM activities WHERE id = $1
	`, activityID).Scan(&entityType, &entityID, &activityUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Activity not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if !admin {
		if activityUserID.Valid && activityUserID.String == userID {
		} else {
			var ownerID string
			switch entityType {
			case "deal":
				err = database.Pool.QueryRow(c.Request.Context(),
					"SELECT user_id FROM crm_deals WHERE id = $1", entityID).Scan(&ownerID)
			case "customer":
				err = database.Pool.QueryRow(c.Request.Context(),
					"SELECT user_id FROM crm_customers WHERE id = $1", entityID).Scan(&ownerID)
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid entity type"})
				return
			}
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Entity not found"})
				return
			}
			if ownerID != userID {
				c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
				return
			}
		}
	}

	_, err = database.Pool.Exec(c.Request.Context(), "DELETE FROM activities WHERE id = $1", activityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ========== ВЛОЖЕНИЯ ==========

// UploadDealAttachment загружает вложение для сделки
func UploadDealAttachment(c *gin.Context) {
	dealID := c.Param("id")
	if dealID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deal_id required"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var ownerID string
	err := database.Pool.QueryRow(c.Request.Context(),
		"SELECT user_id FROM crm_deals WHERE id = $1", dealID).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deal not found"})
		return
	}
	if !admin && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot create upload directory"})
		return
	}

	ext := filepath.Ext(file.Filename)
	newFileName := uuid.New().String() + ext
	filePath := filepath.Join(uploadDir, newFileName)

	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	var attachmentID string
	err = database.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO deal_attachments (id, deal_id, file_name, file_path, file_size, mime_type, uploaded_by, uploaded_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, NOW())
		RETURNING id
	`, dealID, file.Filename, filePath, file.Size, file.Header.Get("Content-Type"), userID).Scan(&attachmentID)

	if err != nil {
		os.Remove(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          attachmentID,
		"file_name":   file.Filename,
		"file_size":   file.Size,
		"mime_type":   file.Header.Get("Content-Type"),
		"uploaded_at": time.Now(),
	})
}

// GetDealAttachments возвращает вложения сделки
func GetDealAttachments(c *gin.Context) {
	dealID := c.Param("id")
	if dealID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deal_id required"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var ownerID string
	err := database.Pool.QueryRow(c.Request.Context(),
		"SELECT user_id FROM crm_deals WHERE id = $1", dealID).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deal not found"})
		return
	}
	if !admin && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id, file_name, file_path, file_size, mime_type, uploaded_by, uploaded_at
		FROM deal_attachments
		WHERE deal_id = $1
		ORDER BY uploaded_at DESC
	`, dealID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	attachments := make([]Attachment, 0)
	for rows.Next() {
		var a Attachment
		var uploadedBy sql.NullString
		err := rows.Scan(&a.ID, &a.FileName, &a.FilePath, &a.FileSize, &a.MimeType, &uploadedBy, &a.UploadedAt)
		if err != nil {
			continue
		}
		if uploadedBy.Valid {
			a.UploadedBy = &uploadedBy.String
		}
		attachments = append(attachments, a)
	}

	c.JSON(http.StatusOK, attachments)
}

// DownloadDealAttachment скачивает вложение
func DownloadDealAttachment(c *gin.Context) {
	attachmentID := c.Param("attachment_id")
	if attachmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "attachment_id required"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var dealOwnerID, filePath, fileName string
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT da.file_path, da.file_name, d.user_id
		FROM deal_attachments da
		JOIN crm_deals d ON d.id = da.deal_id
		WHERE da.id = $1
	`, attachmentID).Scan(&filePath, &fileName, &dealOwnerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}

	if !admin && dealOwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.FileAttachment(filePath, fileName)
}

// DeleteDealAttachment удаляет вложение
func DeleteDealAttachment(c *gin.Context) {
	attachmentID := c.Param("attachment_id")
	if attachmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "attachment_id required"})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var dealOwnerID, filePath string
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT da.file_path, d.user_id
		FROM deal_attachments da
		JOIN crm_deals d ON d.id = da.deal_id
		WHERE da.id = $1
	`, attachmentID).Scan(&filePath, &dealOwnerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}

	if !admin && dealOwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	_, err = database.Pool.Exec(c.Request.Context(), "DELETE FROM deal_attachments WHERE id = $1", attachmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	os.Remove(filePath)

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ========== ПАРТНЁРЫ ==========

// GetPartners возвращает список партнёров
func GetPartners(c *gin.Context) {
    tenantID := getTenantIDFromContext(c)
    
    // Принудительная установка tenant_id для владельца
    if userVal, exists := c.Get("user"); exists {
        if user, ok := userVal.(*models.User); ok {
            if user.Email == "owner@businesstack.ru" || user.Email == "dev@businessstack.ru" {
                tenantID = "c5517fa9-ef93-49c5-aaf2-bc3a62c34253"
                log.Printf("🔧 Принудительно установлен tenant_id: %s для пользователя %s", tenantID, user.Email)
            }
        }
    }
    
    if tenantID == "" {
        log.Printf("❌ GetPartners: tenant_id not found")
        c.JSON(http.StatusOK, gin.H{
            "data":        []Partner{}, // Всегда возвращаем массив, даже пустой
            "total":       0,
            "page":        1,
            "page_size":   20,
            "total_pages": 0,
        })
        return
    }

    page, pageSize := getPaginationParams(c)
    offset := (page - 1) * pageSize

    // Подсчет общего количества
    var total int
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM crm_partners WHERE tenant_id = $1
    `, tenantID).Scan(&total)
    if err != nil {
        log.Printf("❌ GetPartners count error: %v", err)
        c.JSON(http.StatusOK, gin.H{
            "data":        []Partner{},
            "total":       0,
            "page":        page,
            "page_size":   pageSize,
            "total_pages": 0,
        })
        return
    }

    // Получение данных
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, email, phone, inn, status, created_at
        FROM crm_partners 
        WHERE tenant_id = $1
        ORDER BY created_at DESC 
        LIMIT $2 OFFSET $3
    `, tenantID, pageSize, offset)
    if err != nil {
        log.Printf("❌ GetPartners query error: %v", err)
        c.JSON(http.StatusOK, gin.H{
            "data":        []Partner{},
            "total":       0,
            "page":        page,
            "page_size":   pageSize,
            "total_pages": 0,
        })
        return
    }
    defer rows.Close()

    partners := make([]Partner, 0)
    for rows.Next() {
        var p Partner
        err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.Phone, &p.Inn, &p.Status, &p.CreatedAt)
        if err != nil {
            log.Printf("❌ Scan error: %v", err)
            continue
        }
        partners = append(partners, p)
    }

    log.Printf("✅ GetPartners: найдено %d партнеров для tenant_id %s", len(partners), tenantID)

    c.JSON(http.StatusOK, gin.H{
        "data":        partners,
        "total":       total,
        "page":        page,
        "page_size":   pageSize,
        "total_pages": (total + pageSize - 1) / pageSize,
    })
}
// CreatePartner создаёт партнёра
func CreatePartner(c *gin.Context) {
    tenantID := getTenantIDFromContext(c)
    if tenantID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Tenant not found"})
        return
    }

    var req Partner
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    userID := getUserIDFromContext(c)
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var id string
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO crm_partners (id, tenant_id, name, inn, phone, email, status, created_by, created_at, updated_at)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
        RETURNING id
    `, tenantID, req.Name, req.Inn, req.Phone, req.Email, req.Status, userID).Scan(&id)

    if err != nil {
        log.Printf("❌ CreatePartner error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusCreated, gin.H{"id": id, "success": true})
}
// UpdatePartner обновляет партнёра
func UpdatePartner(c *gin.Context) {
	id := c.Param("id")
	var req Partner
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	var ownerID string
	err := database.Pool.QueryRow(c.Request.Context(),
		"SELECT user_id FROM crm_partners WHERE id = $1", id).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Partner not found"})
		return
	}
	if !admin && ownerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	_, err = database.Pool.Exec(c.Request.Context(), `
		UPDATE crm_partners
		SET name = $1, email = $2, phone = $3, inn = $4, status = $5
		WHERE id = $6
	`, req.Name, req.Email, req.Phone, req.Inn, req.Status, id)

	if err != nil {
		log.Printf("❌ UpdatePartner error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeletePartner удаляет партнёра
func DeletePartner(c *gin.Context) {
    id := c.Param("id")
    tenantID := getTenantIDFromContext(c)

    // Проверяем существование партнера
    var partnerTenantID string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT tenant_id FROM crm_partners WHERE id = $1", id).Scan(&partnerTenantID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Partner not found"})
        return
    }

    // Проверка tenant
    if partnerTenantID != tenantID {
        c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
        return
    }

    // Удаляем
    _, err = database.Pool.Exec(c.Request.Context(), "DELETE FROM crm_partners WHERE id = $1", id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}
// ========== ЭКСПОРТ (CSV/Excel) ==========

// exportFilteredCustomers экспортирует отфильтрованных клиентов
func exportFilteredCustomers(c *gin.Context) ([]Customer, error) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant not found")
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	status := c.Query("status")
	search := c.Query("search")
	city := c.Query("city")
	createdFrom := c.Query("created_from")
	createdTo := c.Query("created_to")

	query := `SELECT id, name, email, phone, company, status, responsible, source, comment, 
			 created_at, last_seen, city, social_media, birthday, notes
			 FROM crm_customers`
	args := []interface{}{tenantID}
	whereParts := []string{"tenant_id = $1"}
	argCounter := 1

	nextArg := func() int {
		argCounter++
		return argCounter
	}

	if !admin && userID != "" {
		whereParts = append(whereParts, fmt.Sprintf("user_id = $%d", nextArg()))
		args = append(args, userID)
	}
	if status != "" {
		whereParts = append(whereParts, fmt.Sprintf("status = $%d", nextArg()))
		args = append(args, status)
	}
	if search != "" {
		whereParts = append(whereParts, fmt.Sprintf("(name ILIKE '%%' || $%d || '%%' OR email ILIKE '%%' || $%d || '%%' OR city ILIKE '%%' || $%d || '%%')",
			nextArg(), nextArg(), nextArg()))
		args = append(args, search, search, search)
	}
	if city != "" {
		whereParts = append(whereParts, fmt.Sprintf("city = $%d", nextArg()))
		args = append(args, city)
	}
	if createdFrom != "" {
		whereParts = append(whereParts, fmt.Sprintf("created_at >= $%d::date", nextArg()))
		args = append(args, createdFrom)
	}
	if createdTo != "" {
		whereParts = append(whereParts, fmt.Sprintf("created_at < ($%d::date + '1 day'::interval)", nextArg()))
		args = append(args, createdTo)
	}

	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	query += " ORDER BY created_at DESC"

	rows, err := database.Pool.Query(c.Request.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	customers := make([]Customer, 0)
	for rows.Next() {
		var cst Customer
		var socialMedia []byte
		var birthday sql.NullTime
		err := rows.Scan(&cst.ID, &cst.Name, &cst.Email, &cst.Phone, &cst.Company, &cst.Status,
			&cst.Responsible, &cst.Source, &cst.Comment, &cst.CreatedAt, &cst.LastSeen,
			&cst.City, &socialMedia, &birthday, &cst.Notes)
		if err != nil {
			continue
		}
		if socialMedia != nil {
			cst.SocialMedia = json.RawMessage(socialMedia)
		}
		if birthday.Valid {
			cst.Birthday = &birthday.Time
		}
		customers = append(customers, cst)
	}
	return customers, nil
}

// ExportCustomersCSV экспортирует клиентов в CSV
func ExportCustomersCSV(c *gin.Context) {
	customers, err := exportFilteredCustomers(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	buf := new(bytes.Buffer)
	writer := csv.NewWriter(buf)
	defer writer.Flush()

	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	headers := []string{"ID", "Имя", "Email", "Телефон", "Компания", "Статус", "Ответственный",
		"Источник", "Комментарий", "Дата создания", "Последний визит", "Город", "Соцсети", "День рождения", "Заметки"}
	writer.Write(headers)

	for _, cst := range customers {
		birthday := ""
		if cst.Birthday != nil {
			birthday = cst.Birthday.Format("2006-01-02")
		}
		writer.Write([]string{
			cst.ID,
			cst.Name,
			cst.Email,
			cst.Phone,
			cst.Company,
			cst.Status,
			cst.Responsible,
			cst.Source,
			cst.Comment,
			cst.CreatedAt.Format("2006-01-02 15:04:05"),
			cst.LastSeen.Format("2006-01-02 15:04:05"),
			cst.City,
			string(cst.SocialMedia),
			birthday,
			cst.Notes,
		})
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename=customers.csv")
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportCustomersExcel экспортирует клиентов в Excel
func ExportCustomersExcel(c *gin.Context) {
	customers, err := exportFilteredCustomers(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	sheet := "Клиенты"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"ID", "Имя", "Email", "Телефон", "Компания", "Статус", "Ответственный",
		"Источник", "Комментарий", "Дата создания", "Последний визит", "Город", "Соцсети", "День рождения", "Заметки"}
	for i, h := range headers {
		cell := fmt.Sprintf("%c%d", 'A'+i, 1)
		f.SetCellValue(sheet, cell, h)
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 12},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "bottom", Color: "#000000", Style: 1},
			{Type: "top", Color: "#000000", Style: 1},
			{Type: "left", Color: "#000000", Style: 1},
			{Type: "right", Color: "#000000", Style: 1},
		},
	})
	f.SetCellStyle(sheet, "A1", fmt.Sprintf("%c%d", 'A'+len(headers)-1, 1), headerStyle)

	for i, cst := range customers {
		row := i + 2
		birthday := ""
		if cst.Birthday != nil {
			birthday = cst.Birthday.Format("2006-01-02")
		}

		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), cst.ID)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), cst.Name)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), cst.Email)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), cst.Phone)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), cst.Company)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), cst.Status)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), cst.Responsible)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", row), cst.Source)
		f.SetCellValue(sheet, fmt.Sprintf("I%d", row), cst.Comment)
		f.SetCellValue(sheet, fmt.Sprintf("J%d", row), cst.CreatedAt.Format("2006-01-02 15:04:05"))
		f.SetCellValue(sheet, fmt.Sprintf("K%d", row), cst.LastSeen.Format("2006-01-02 15:04:05"))
		f.SetCellValue(sheet, fmt.Sprintf("L%d", row), cst.City)
		f.SetCellValue(sheet, fmt.Sprintf("M%d", row), string(cst.SocialMedia))
		f.SetCellValue(sheet, fmt.Sprintf("N%d", row), birthday)
		f.SetCellValue(sheet, fmt.Sprintf("O%d", row), cst.Notes)
	}

	for i := 1; i <= len(headers); i++ {
		col := string(rune('A' + i - 1))
		f.SetColWidth(sheet, col, col, 20)
	}

	f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	buf, err := f.WriteToBuffer()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate Excel"})
		return
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename=customers.xlsx")
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

// exportFilteredDeals экспортирует отфильтрованные сделки
func exportFilteredDeals(c *gin.Context) ([]Deal, error) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant not found")
	}

	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	stage := c.Query("stage")
	search := c.Query("search")
	category := c.Query("category")
	valueMin := c.Query("value_min")
	valueMax := c.Query("value_max")
	closeFrom := c.Query("close_from")
	closeTo := c.Query("close_to")

	query := `SELECT id, customer_id, title, value, stage, probability, responsible, source, 
			 comment, expected_close, created_at, closed_at, product_category, discount, next_action_date
			 FROM crm_deals`
	args := []interface{}{tenantID}
	whereParts := []string{"tenant_id = $1"}
	argCounter := 1

	nextArg := func() int {
		argCounter++
		return argCounter
	}

	if !admin && userID != "" {
		whereParts = append(whereParts, fmt.Sprintf("user_id = $%d", nextArg()))
		args = append(args, userID)
	}
	if stage != "" {
		whereParts = append(whereParts, fmt.Sprintf("stage = $%d", nextArg()))
		args = append(args, stage)
	}
	if search != "" {
		whereParts = append(whereParts, fmt.Sprintf("title ILIKE '%%' || $%d || '%%'", nextArg()))
		args = append(args, search)
	}
	if category != "" {
		whereParts = append(whereParts, fmt.Sprintf("product_category = $%d", nextArg()))
		args = append(args, category)
	}
	if valueMin != "" {
		whereParts = append(whereParts, fmt.Sprintf("value >= $%d", nextArg()))
		args = append(args, valueMin)
	}
	if valueMax != "" {
		whereParts = append(whereParts, fmt.Sprintf("value <= $%d", nextArg()))
		args = append(args, valueMax)
	}
	if closeFrom != "" {
		whereParts = append(whereParts, fmt.Sprintf("expected_close >= $%d::date", nextArg()))
		args = append(args, closeFrom)
	}
	if closeTo != "" {
		whereParts = append(whereParts, fmt.Sprintf("expected_close < ($%d::date + '1 day'::interval)", nextArg()))
		args = append(args, closeTo)
	}

	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	query += " ORDER BY created_at DESC"

	rows, err := database.Pool.Query(c.Request.Context(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deals := make([]Deal, 0)
	for rows.Next() {
		var d Deal
		var nextActionDate sql.NullTime
		var customerID sql.NullString
		err := rows.Scan(&d.ID, &customerID, &d.Title, &d.Value, &d.Stage, &d.Probability,
			&d.Responsible, &d.Source, &d.Comment, &d.ExpectedClose, &d.CreatedAt, &d.ClosedAt,
			&d.ProductCategory, &d.Discount, &nextActionDate)
		if err != nil {
			continue
		}
		if customerID.Valid {
			d.CustomerID = &customerID.String
		}
		if nextActionDate.Valid {
			d.NextActionDate = &nextActionDate.Time
		}
		deals = append(deals, d)
	}
	return deals, nil
}

// ExportDealsCSV экспортирует сделки в CSV
func ExportDealsCSV(c *gin.Context) {
	deals, err := exportFilteredDeals(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	buf := new(bytes.Buffer)
	writer := csv.NewWriter(buf)
	defer writer.Flush()

	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	headers := []string{"ID", "Клиент ID", "Название", "Сумма", "Стадия", "Вероятность", "Ответственный",
		"Источник", "Комментарий", "Ожидаемая дата", "Дата создания", "Дата закрытия", "Категория", "Скидка", "Следующее действие"}
	writer.Write(headers)

	for _, d := range deals {
		expectedClose := ""
		if d.ExpectedClose != nil {
			expectedClose = d.ExpectedClose.Format("2006-01-02")
		}
		closedAt := ""
		if d.ClosedAt != nil {
			closedAt = d.ClosedAt.Format("2006-01-02 15:04:05")
		}
		nextAction := ""
		if d.NextActionDate != nil {
			nextAction = d.NextActionDate.Format("2006-01-02")
		}

		customerIDStr := ""
		if d.CustomerID != nil {
			customerIDStr = *d.CustomerID
		}

		writer.Write([]string{
			d.ID,
			customerIDStr,
			d.Title,
			strconv.FormatFloat(d.Value, 'f', 2, 64),
			d.Stage,
			strconv.Itoa(d.Probability),
			d.Responsible,
			d.Source,
			d.Comment,
			expectedClose,
			d.CreatedAt.Format("2006-01-02 15:04:05"),
			closedAt,
			d.ProductCategory,
			strconv.FormatFloat(d.Discount, 'f', 2, 64),
			nextAction,
		})
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename=deals.csv")
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportDealsExcel экспортирует сделки в Excel
func ExportDealsExcel(c *gin.Context) {
	deals, err := exportFilteredDeals(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	sheet := "Сделки"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"ID", "Клиент ID", "Название", "Сумма", "Стадия", "Вероятность", "Ответственный",
		"Источник", "Комментарий", "Ожидаемая дата", "Дата создания", "Дата закрытия", "Категория", "Скидка", "Следующее действие"}
	for i, h := range headers {
		cell := fmt.Sprintf("%c%d", 'A'+i, 1)
		f.SetCellValue(sheet, cell, h)
	}

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 12},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	f.SetCellStyle(sheet, "A1", fmt.Sprintf("%c%d", 'A'+len(headers)-1, 1), headerStyle)

	moneyStyle, _ := f.NewStyle(&excelize.Style{
		CustomNumFmt: &[]string{"#,##0.00 ₽"}[0],
	})

	percentStyle, _ := f.NewStyle(&excelize.Style{
		CustomNumFmt: &[]string{"0%"}[0],
	})

	for i, d := range deals {
		row := i + 2
		expectedClose := ""
		if d.ExpectedClose != nil {
			expectedClose = d.ExpectedClose.Format("2006-01-02")
		}
		closedAt := ""
		if d.ClosedAt != nil {
			closedAt = d.ClosedAt.Format("2006-01-02 15:04:05")
		}
		nextAction := ""
		if d.NextActionDate != nil {
			nextAction = d.NextActionDate.Format("2006-01-02")
		}

		customerIDStr := ""
		if d.CustomerID != nil {
			customerIDStr = *d.CustomerID
		}

		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), d.ID)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), customerIDStr)
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), d.Title)
		f.SetCellValue(sheet, fmt.Sprintf("D%d", row), d.Value)
		f.SetCellStyle(sheet, fmt.Sprintf("D%d", row), fmt.Sprintf("D%d", row), moneyStyle)
		f.SetCellValue(sheet, fmt.Sprintf("E%d", row), d.Stage)
		f.SetCellValue(sheet, fmt.Sprintf("F%d", row), float64(d.Probability)/100)
		f.SetCellStyle(sheet, fmt.Sprintf("F%d", row), fmt.Sprintf("F%d", row), percentStyle)
		f.SetCellValue(sheet, fmt.Sprintf("G%d", row), d.Responsible)
		f.SetCellValue(sheet, fmt.Sprintf("H%d", row), d.Source)
		f.SetCellValue(sheet, fmt.Sprintf("I%d", row), d.Comment)
		f.SetCellValue(sheet, fmt.Sprintf("J%d", row), expectedClose)
		f.SetCellValue(sheet, fmt.Sprintf("K%d", row), d.CreatedAt.Format("2006-01-02 15:04:05"))
		f.SetCellValue(sheet, fmt.Sprintf("L%d", row), closedAt)
		f.SetCellValue(sheet, fmt.Sprintf("M%d", row), d.ProductCategory)
		f.SetCellValue(sheet, fmt.Sprintf("N%d", row), d.Discount)
		f.SetCellStyle(sheet, fmt.Sprintf("N%d", row), fmt.Sprintf("N%d", row), moneyStyle)
		f.SetCellValue(sheet, fmt.Sprintf("O%d", row), nextAction)
	}

	for i := 1; i <= len(headers); i++ {
		col := string(rune('A' + i - 1))
		f.SetColWidth(sheet, col, col, 20)
	}

	f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})

	buf, err := f.WriteToBuffer()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate Excel"})
		return
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", "attachment; filename=deals.xlsx")
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

// GetCRMAdvancedStats возвращает расширенную аналитику CRM
func GetCRMAdvancedStats(c *gin.Context) {
	tenantID := getTenantIDFromContext(c)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tenant not found"})
		return
	}

	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")
	userID := getUserIDFromContext(c)
	admin := isAdmin(c)

	args := []interface{}{tenantID}
	dateFilter := ""
	argCounter := 1

	nextArg := func() int {
		argCounter++
		return argCounter
	}

	if dateFrom != "" {
		dateFilter += fmt.Sprintf(" AND created_at >= $%d::date", nextArg())
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		dateFilter += fmt.Sprintf(" AND created_at < ($%d::date + '1 day'::interval)", nextArg())
		args = append(args, dateTo)
	}

	userFilter := ""
	if !admin && userID != "" {
		userFilter = fmt.Sprintf(" AND user_id = $%d", nextArg())
		args = append(args, userID)
	}

	// Статистика по ответственным
	responsibleQuery := `
		SELECT 
			COALESCE(responsible, 'Не назначен') as responsible,
			COUNT(*) as deals_count,
			COALESCE(SUM(value), 0) as total_value
		FROM crm_deals
		WHERE tenant_id = $1` + dateFilter + userFilter + `
		GROUP BY responsible
		ORDER BY total_value DESC
	`

	rows, err := database.Pool.Query(c.Request.Context(), responsibleQuery, args...)
	if err != nil {
		log.Printf("❌ GetCRMAdvancedStats responsible error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	responsibleStats := make([]gin.H, 0)
	for rows.Next() {
		var responsible string
		var dealsCount int
		var totalValue float64
		if err := rows.Scan(&responsible, &dealsCount, &totalValue); err != nil {
			continue
		}
		responsibleStats = append(responsibleStats, gin.H{
			"responsible": responsible,
			"deals_count": dealsCount,
			"total_value": totalValue,
		})
	}

	// Статистика по источникам
	sourceQuery := `
		SELECT 
			COALESCE(source, 'Не указан') as source,
			COUNT(*) as deals_count,
			COALESCE(SUM(value), 0) as total_value
		FROM crm_deals
		WHERE tenant_id = $1` + dateFilter + userFilter + `
		GROUP BY source
		ORDER BY total_value DESC
	`

	rows, err = database.Pool.Query(c.Request.Context(), sourceQuery, args...)
	if err != nil {
		log.Printf("❌ GetCRMAdvancedStats source error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	sourceStats := make([]gin.H, 0)
	for rows.Next() {
		var source string
		var dealsCount int
		var totalValue float64
		if err := rows.Scan(&source, &dealsCount, &totalValue); err != nil {
			continue
		}
		sourceStats = append(sourceStats, gin.H{
			"source":      source,
			"deals_count": dealsCount,
			"total_value": totalValue,
		})
	}

	// Месячная статистика
	monthlyQuery := `
		SELECT 
			TO_CHAR(date_trunc('month', created_at), 'YYYY-MM') as month,
			COUNT(*) as deals_created,
			COALESCE(SUM(value), 0) as total_value
		FROM crm_deals
		WHERE tenant_id = $1` + dateFilter + userFilter + `
		GROUP BY date_trunc('month', created_at)
		ORDER BY month
	`

	rows, err = database.Pool.Query(c.Request.Context(), monthlyQuery, args...)
	if err != nil {
		log.Printf("❌ GetCRMAdvancedStats monthly error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	defer rows.Close()

	monthlyStats := make([]gin.H, 0)
	for rows.Next() {
		var month string
		var dealsCount int
		var totalValue float64
		if err := rows.Scan(&month, &dealsCount, &totalValue); err != nil {
			continue
		}
		monthlyStats = append(monthlyStats, gin.H{
			"month":       month,
			"deals_count": dealsCount,
			"total_value": totalValue,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"responsible_stats": responsibleStats,
		"source_stats":      sourceStats,
		"monthly_stats":     monthlyStats,
	})
}