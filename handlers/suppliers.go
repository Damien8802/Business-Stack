package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

// ============================================
// 1. СТРУКТУРЫ
// ============================================

type Supplier struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	TenantID         uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	UserID           uuid.UUID  `json:"user_id" db:"user_id"`
	Name             string     `json:"name" db:"name" binding:"required"`
	Inn              string     `json:"inn" db:"inn"`
	Kpp              string     `json:"kpp" db:"kpp"`
	Ogrn             string     `json:"ogrn" db:"ogrn"`
	Phone            string     `json:"phone" db:"phone"`
	Email            string     `json:"email" db:"email"`
	Address          string     `json:"address" db:"address"`
	ContactPerson    string     `json:"contact_person" db:"contact_person"`
	Website          string     `json:"website" db:"website"`
	Notes            string     `json:"notes" db:"notes"`
	BankName         string     `json:"bank_name" db:"bank_name"`
	BankAccount      string     `json:"bank_account" db:"bank_account"`
	BIK              string     `json:"bik" db:"bik"`
	CreditLimit      float64    `json:"credit_limit" db:"credit_limit"`
	CurrentDebt      float64    `json:"current_debt" db:"current_debt"`
	Rating           float64    `json:"rating" db:"rating"`
	TotalOrders      int        `json:"total_orders" db:"total_orders"`
	SuccessRate      float64    `json:"success_rate" db:"success_rate"`
	AvgDeliveryDays  int        `json:"avg_delivery_days" db:"avg_delivery_days"`
	Active           bool       `json:"active" db:"active"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

type PurchaseOrder struct {
	ID           uuid.UUID   `json:"id" db:"id"`
	TenantID     uuid.UUID   `json:"tenant_id" db:"tenant_id"`
	UserID       uuid.UUID   `json:"user_id" db:"user_id"`
	OrderNumber  string      `json:"order_number" db:"order_number"`
	SupplierID   uuid.UUID   `json:"supplier_id" db:"supplier_id"`
	SupplierName string      `json:"supplier_name" db:"supplier_name"`
	Status       string      `json:"status" db:"status"`
	TotalAmount  float64     `json:"total_amount" db:"total_amount"`
	OrderDate    time.Time   `json:"order_date" db:"order_date"`
	ExpectedDate *time.Time  `json:"expected_date" db:"expected_date"`
	DeliveryDate *time.Time  `json:"delivery_date" db:"delivery_date"`
	Notes        string      `json:"notes" db:"notes"`
	CreatedAt    time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at" db:"updated_at"`
}

type SupplierHandler struct {
	db    *sqlx.DB
	redis *redis.Client
}

func NewSupplierHandler(db *sqlx.DB, redis *redis.Client) *SupplierHandler {
	return &SupplierHandler{
		db:    db,
		redis: redis,
	}
}

// ============================================
// 2. ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ (ПЕРЕИМЕНОВАНЫ)
// ============================================

func getSupplierTenantID(c *gin.Context) string {
	tenantID, exists := c.Get("tenant_id")
	if !exists {
		return ""
	}
	if id, ok := tenantID.(string); ok {
		return id
	}
	if id, ok := tenantID.(uuid.UUID); ok {
		return id.String()
	}
	return ""
}

func getSupplierUserID(c *gin.Context) uuid.UUID {
	userID, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil
	}
	if id, ok := userID.(uuid.UUID); ok {
		return id
	}
	if id, ok := userID.(string); ok {
		if parsed, err := uuid.Parse(id); err == nil {
			return parsed
		}
	}
	return uuid.Nil
}

func isAdminUser(c *gin.Context) bool {
	role, exists := c.Get("role")
	if !exists {
		return false
	}
	roleStr, ok := role.(string)
	if !ok {
		return false
	}
	return roleStr == "admin" || roleStr == "owner"
}

func (h *SupplierHandler) getCacheKey(prefix string, tenantID string, params ...string) string {
	key := fmt.Sprintf("supplier:%s:%s", prefix, tenantID)
	for _, p := range params {
		if p != "" {
			key += ":" + p
		}
	}
	return key
}

func (h *SupplierHandler) setCache(ctx context.Context, key string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return h.redis.Set(ctx, key, jsonData, 5*time.Minute).Err()
}

func (h *SupplierHandler) getCache(ctx context.Context, key string, dest interface{}) error {
	data, err := h.redis.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (h *SupplierHandler) invalidateCache(ctx context.Context, tenantID string, prefixes ...string) {
	for _, prefix := range prefixes {
		pattern := h.getCacheKey(prefix, tenantID, "*")
		iter := h.redis.Scan(ctx, 0, pattern, 0).Iterator()
		for iter.Next(ctx) {
			h.redis.Del(ctx, iter.Val())
		}
	}
}

// ============================================
// 3. API ПОСТАВЩИКОВ (ВСЕ getTenantID заменены на getSupplierTenantID)
// ============================================

func (h *SupplierHandler) GetSuppliers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	userID := getSupplierUserID(c)
	isAdmin := isAdminUser(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	search := strings.TrimSpace(c.Query("search"))
	activeOnly := c.DefaultQuery("active", "true") == "true"
	limit, _ := c.GetQuery("limit")

	cacheKey := h.getCacheKey("list", tenantID,
		fmt.Sprintf("search_%s", search),
		fmt.Sprintf("active_%t", activeOnly),
		limit,
	)

	var cachedResponse map[string]interface{}
	if err := h.getCache(ctx, cacheKey, &cachedResponse); err == nil {
		c.JSON(http.StatusOK, cachedResponse)
		return
	}

	query := `
		SELECT 
			s.id, s.tenant_id, s.user_id, s.name, s.inn, s.kpp, s.ogrn,
			s.phone, s.email, s.address, s.contact_person, s.website,
			s.notes, s.bank_name, s.bank_account, s.bik,
			s.credit_limit, s.current_debt,
			s.rating, s.total_orders, s.success_rate, s.avg_delivery_days,
			s.active, s.created_at, s.updated_at
		FROM suppliers s
		WHERE s.tenant_id = $1
	`

	args := []interface{}{tenantID}
	argIndex := 2

	if !isAdmin && userID != uuid.Nil {
		query += fmt.Sprintf(" AND s.user_id = $%d", argIndex)
		args = append(args, userID)
		argIndex++
	}

	if activeOnly {
		query += fmt.Sprintf(" AND s.active = true")
	}

	if search != "" {
		query += fmt.Sprintf(" AND (s.name ILIKE $%d OR s.inn ILIKE $%d OR s.contact_person ILIKE $%d)",
			argIndex, argIndex, argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}

	query += " ORDER BY s.name"

	if limit != "" {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, limit)
	}

	var suppliers []Supplier
	err := h.db.SelectContext(ctx, &suppliers, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"suppliers": suppliers,
		"count":     len(suppliers),
	}

	h.setCache(ctx, cacheKey, response)
	c.JSON(http.StatusOK, response)
}

func (h *SupplierHandler) GetSupplier(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	cacheKey := h.getCacheKey("single", tenantID, id)
	var supplier Supplier
	if err := h.getCache(ctx, cacheKey, &supplier); err == nil {
		c.JSON(http.StatusOK, gin.H{"supplier": supplier})
		return
	}

	err := h.db.GetContext(ctx, &supplier, `
		SELECT 
			s.id, s.tenant_id, s.user_id, s.name, s.inn, s.kpp, s.ogrn,
			s.phone, s.email, s.address, s.contact_person, s.website,
			s.notes, s.bank_name, s.bank_account, s.bik,
			s.credit_limit, s.current_debt,
			s.rating, s.total_orders, s.success_rate, s.avg_delivery_days,
			s.active, s.created_at, s.updated_at
		FROM suppliers s
		WHERE s.id = $1 AND s.tenant_id = $2 AND s.active = true
	`, id, tenantID)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Supplier not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	h.setCache(ctx, cacheKey, supplier)
	c.JSON(http.StatusOK, gin.H{"supplier": supplier})
}

func (h *SupplierHandler) CreateSupplier(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	userID := getSupplierUserID(c)

	if tenantID == "" || userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id and user_id required"})
		return
	}

	var req Supplier
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	var exists bool
	err := h.db.GetContext(ctx, &exists, `
		SELECT EXISTS(SELECT 1 FROM suppliers 
		WHERE name = $1 AND tenant_id = $2 AND active = true)
	`, req.Name, tenantID)
	if err == nil && exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "поставщик с таким названием уже существует"})
		return
	}

	if req.Inn != "" {
		err = h.db.GetContext(ctx, &exists, `
			SELECT EXISTS(SELECT 1 FROM suppliers 
			WHERE inn = $1 AND tenant_id = $2 AND active = true)
		`, req.Inn, tenantID)
		if err == nil && exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "поставщик с таким ИНН уже существует"})
			return
		}
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	var id uuid.UUID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO suppliers (
			tenant_id, user_id, name, inn, kpp, ogrn, phone, email, 
			address, contact_person, website, notes,
			bank_name, bank_account, bik,
			credit_limit, current_debt,
			rating, total_orders, success_rate, avg_delivery_days,
			active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
				  $13, $14, $15, $16, $17, $18, $19, $20, $21, true, NOW(), NOW())
		RETURNING id
	`, tenantID, userID, req.Name, req.Inn, req.Kpp, req.Ogrn, req.Phone, req.Email,
		req.Address, req.ContactPerson, req.Website, req.Notes,
		req.BankName, req.BankAccount, req.BIK,
		req.CreditLimit, req.CurrentDebt,
		req.Rating, 0, 0, 0).Scan(&id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_, _ = tx.ExecContext(ctx, `
		INSERT INTO supplier_logs (supplier_id, action, user_id, tenant_id, created_at)
		VALUES ($1, 'create', $2, $3, NOW())
	`, id, userID, tenantID)

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "list", "stats")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"id":      id,
		"message": "поставщик успешно создан",
	})
}

func (h *SupplierHandler) UpdateSupplier(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var req Supplier
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var exists bool
	err := h.db.GetContext(ctx, &exists, `
		SELECT EXISTS(SELECT 1 FROM suppliers 
		WHERE id = $1 AND tenant_id = $2 AND active = true)
	`, id, tenantID)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "поставщик не найден"})
		return
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE suppliers 
		SET name = $1, inn = $2, kpp = $3, ogrn = $4, 
			phone = $5, email = $6, address = $7, 
			contact_person = $8, website = $9, notes = $10,
			bank_name = $11, bank_account = $12, bik = $13,
			credit_limit = $14, current_debt = $15,
			rating = $16, active = $17,
			updated_at = NOW()
		WHERE id = $18 AND tenant_id = $19
	`, req.Name, req.Inn, req.Kpp, req.Ogrn,
		req.Phone, req.Email, req.Address,
		req.ContactPerson, req.Website, req.Notes,
		req.BankName, req.BankAccount, req.BIK,
		req.CreditLimit, req.CurrentDebt,
		req.Rating, req.Active,
		id, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "list", "single", "stats")
	h.invalidateCache(ctx, tenantID, "single", id)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "поставщик успешно обновлен",
	})
}

func (h *SupplierHandler) DeleteSupplier(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var orderCount int
	err := h.db.GetContext(ctx, &orderCount, `
		SELECT COUNT(*) FROM purchase_orders 
		WHERE supplier_id = $1 AND tenant_id = $2 
		AND status NOT IN ('delivered', 'cancelled')
	`, id, tenantID)

	if err == nil && orderCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       "невозможно удалить поставщика, есть активные заказы",
			"order_count": orderCount,
		})
		return
	}

	_, err = h.db.ExecContext(ctx, `
		UPDATE suppliers 
		SET active = false, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "list", "single", "stats")
	h.invalidateCache(ctx, tenantID, "single", id)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "поставщик удален",
	})
}

func (h *SupplierHandler) GetSupplierStats(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	cacheKey := h.getCacheKey("stats", tenantID)

	var stats map[string]interface{}
	if err := h.getCache(ctx, cacheKey, &stats); err == nil {
		c.JSON(http.StatusOK, gin.H{"stats": stats})
		return
	}

	var result struct {
		TotalSuppliers  int     `db:"total_suppliers"`
		ActiveSuppliers int     `db:"active_suppliers"`
		AvgRating       float64 `db:"avg_rating"`
		TotalOrders     int     `db:"total_orders"`
		TotalSpent      float64 `db:"total_spent"`
	}

	err := h.db.GetContext(ctx, &result, `
		SELECT 
			COUNT(*) as total_suppliers,
			COUNT(CASE WHEN active = true THEN 1 END) as active_suppliers,
			COALESCE(AVG(rating), 0) as avg_rating,
			COALESCE(SUM(total_orders), 0) as total_orders,
			COALESCE(SUM(current_debt), 0) as total_spent
		FROM suppliers
		WHERE tenant_id = $1
	`, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	stats = map[string]interface{}{
		"total_suppliers":  result.TotalSuppliers,
		"active_suppliers": result.ActiveSuppliers,
		"avg_rating":       result.AvgRating,
		"total_orders":     result.TotalOrders,
		"total_spent":      result.TotalSpent,
	}

	h.setCache(ctx, cacheKey, stats)
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// ============================================
// 4. API ЗАКУПОК
// ============================================

func (h *SupplierHandler) GetPurchaseOrders(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	userID := getSupplierUserID(c)
	isAdmin := isAdminUser(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	status := c.Query("status")
	supplierID := c.Query("supplier_id")
	limit, _ := c.GetQuery("limit")

	query := `
		SELECT 
			po.id, po.tenant_id, po.user_id, po.order_number, 
			po.supplier_id, s.name as supplier_name,
			po.status, po.total_amount, po.order_date, 
			po.expected_date, po.delivery_date, po.notes,
			po.created_at, po.updated_at
		FROM purchase_orders po
		LEFT JOIN suppliers s ON po.supplier_id = s.id AND s.tenant_id = po.tenant_id
		WHERE po.tenant_id = $1
	`

	args := []interface{}{tenantID}
	argIndex := 2

	if !isAdmin && userID != uuid.Nil {
		query += fmt.Sprintf(" AND po.user_id = $%d", argIndex)
		args = append(args, userID)
		argIndex++
	}

	if status != "" {
		query += fmt.Sprintf(" AND po.status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}

	if supplierID != "" {
		query += fmt.Sprintf(" AND po.supplier_id = $%d", argIndex)
		args = append(args, supplierID)
		argIndex++
	}

	query += " ORDER BY po.created_at DESC"

	if limit != "" {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, limit)
	}

	var orders []PurchaseOrder
	err := h.db.SelectContext(ctx, &orders, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"purchase_orders": orders,
		"count":           len(orders),
	})
}

func (h *SupplierHandler) GetPurchaseOrder(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var order PurchaseOrder
	err := h.db.GetContext(ctx, &order, `
		SELECT 
			po.id, po.tenant_id, po.user_id, po.order_number, 
			po.supplier_id, s.name as supplier_name,
			po.status, po.total_amount, po.order_date, 
			po.expected_date, po.delivery_date, po.notes,
			po.created_at, po.updated_at
		FROM purchase_orders po
		LEFT JOIN suppliers s ON po.supplier_id = s.id AND s.tenant_id = po.tenant_id
		WHERE po.id = $1 AND po.tenant_id = $2
	`, id, tenantID)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	var items []map[string]interface{}
	err = h.db.SelectContext(ctx, &items, `
		SELECT 
			product_id, product_name, quantity, price, total
		FROM purchase_order_items
		WHERE purchase_order_id = $1
	`, id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"order": order,
		"items": items,
	})
}

func (h *SupplierHandler) CreatePurchaseOrder(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	userID := getSupplierUserID(c)

	if tenantID == "" || userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id and user_id required"})
		return
	}

	var req struct {
		SupplierID   uuid.UUID   `json:"supplier_id" binding:"required"`
		ExpectedDate *time.Time  `json:"expected_date"`
		DeliveryDate *time.Time  `json:"delivery_date"`
		Notes        string      `json:"notes"`
		Items        []struct {
			ProductID uuid.UUID `json:"product_id" binding:"required"`
			Quantity  int       `json:"quantity" binding:"required,min=1"`
			Price     float64   `json:"price" binding:"required,min=0"`
		} `json:"items" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var supplierExists bool
	err := h.db.GetContext(ctx, &supplierExists, `
		SELECT EXISTS(SELECT 1 FROM suppliers 
		WHERE id = $1 AND tenant_id = $2 AND active = true)
	`, req.SupplierID, tenantID)
	if err != nil || !supplierExists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "поставщик не найден"})
		return
	}

	orderNumber := fmt.Sprintf("PO-%d-%d", time.Now().Year(), time.Now().UnixNano()%1000000)

	totalAmount := 0.0
	for _, item := range req.Items {
		totalAmount += item.Price * float64(item.Quantity)
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	var orderID uuid.UUID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO purchase_orders (
			tenant_id, user_id, supplier_id, order_number, total_amount,
			expected_date, delivery_date, notes, status, order_date, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'draft', NOW(), NOW(), NOW())
		RETURNING id
	`, tenantID, userID, req.SupplierID, orderNumber, totalAmount,
		req.ExpectedDate, req.DeliveryDate, req.Notes).Scan(&orderID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, item := range req.Items {
		var productName string
		h.db.GetContext(ctx, &productName, `
			SELECT name FROM inventory_products WHERE id = $1 AND tenant_id = $2
		`, item.ProductID, tenantID)

		if productName == "" {
			productName = "Товар #" + item.ProductID.String()[:8]
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO purchase_order_items (
				purchase_order_id, product_id, product_name, quantity, price, total
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, orderID, item.ProductID, productName, item.Quantity, item.Price,
			item.Price*float64(item.Quantity))

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "orders", "stats")

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"order_id":     orderID,
		"order_number": orderNumber,
		"message":      "заказ успешно создан",
	})
}

func (h *SupplierHandler) UpdatePurchaseOrderStatus(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var req struct {
		Status       string     `json:"status" binding:"required"`
		DeliveryDate *time.Time `json:"delivery_date"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	validStatuses := map[string]bool{
		"draft": true, "ordered": true, "shipped": true,
		"delivered": true, "cancelled": true,
	}
	if !validStatuses[req.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "неверный статус"})
		return
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	updateQuery := `
		UPDATE purchase_orders 
		SET status = $1, updated_at = NOW()
	`
	args := []interface{}{req.Status}
	argIndex := 2

	if req.DeliveryDate != nil && req.Status == "delivered" {
		updateQuery += fmt.Sprintf(", delivery_date = $%d", argIndex)
		args = append(args, req.DeliveryDate)
		argIndex++
	}

	updateQuery += fmt.Sprintf(" WHERE id = $%d AND tenant_id = $%d", argIndex, argIndex+1)
	args = append(args, id, tenantID)

	_, err = tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.Status == "delivered" {
		var items []struct {
			ProductID uuid.UUID `db:"product_id"`
			Quantity  int       `db:"quantity"`
		}
		err = tx.SelectContext(ctx, &items, `
			SELECT product_id, quantity FROM purchase_order_items 
			WHERE purchase_order_id = $1
		`, id)

		if err == nil {
			for _, item := range items {
				_, _ = tx.ExecContext(ctx, `
					UPDATE inventory_products 
					SET quantity = quantity + $1, updated_at = NOW()
					WHERE id = $2 AND tenant_id = $3
				`, item.Quantity, item.ProductID, tenantID)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "orders", "stats")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "статус заказа обновлен",
	})
}

func (h *SupplierHandler) DeletePurchaseOrder(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var status string
	err := h.db.GetContext(ctx, &status, `
		SELECT status FROM purchase_orders 
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "заказ не найден"})
		return
	}

	if status != "draft" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "можно удалить только черновики"})
		return
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `DELETE FROM purchase_order_items WHERE purchase_order_id = $1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_, err = tx.ExecContext(ctx, `
		DELETE FROM purchase_orders 
		WHERE id = $1 AND tenant_id = $2 AND status = 'draft'
	`, id, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "orders")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "заказ удален",
	})
}

