package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

// ============================================
// WEBSOCKET ДЛЯ ИНВЕНТАРИЗАЦИИ
// ============================================

var inventoryHub *Hub

// Hub - WebSocket хаб для инвентаризации
type Hub struct {
	clients    map[*WSClient]bool
	broadcast  chan []byte
	register   chan *WSClient
	unregister chan *WSClient
	rooms      map[string]map[*WSClient]bool
}

// WSClient - WebSocket клиент
type WSClient struct {
	Conn     *websocket.Conn
	Send     chan []byte
	TenantID string
	UserID   string
	Hub      *Hub
}

// upgrader УДАЛЯЕМ - используем из qr_auth.go

// NewHub - создает новый хаб
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
		rooms:      make(map[string]map[*WSClient]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			if client.TenantID != "" {
				if h.rooms[client.TenantID] == nil {
					h.rooms[client.TenantID] = make(map[*WSClient]bool)
				}
				h.rooms[client.TenantID][client] = true
			}
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				if client.TenantID != "" {
					if h.rooms[client.TenantID] != nil {
						delete(h.rooms[client.TenantID], client)
					}
				}
				close(client.Send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
		}
	}
}

// SendToRoom - отправляет сообщение всем в комнате
func (h *Hub) SendToRoom(room string, event string, data interface{}) {
	msg, _ := json.Marshal(gin.H{
		"event": event,
		"data":  data,
	})
	if h.rooms[room] != nil {
		for client := range h.rooms[room] {
			select {
			case client.Send <- msg:
			default:
				h.unregister <- client
			}
		}
	}
}

func (c *WSClient) writePump() {
	defer c.Conn.Close()
	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}
}

func (c *WSClient) readPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()
	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// InitInventoryWS - инициализирует WebSocket для инвентаризации
func InitInventoryWS() {
	if inventoryHub == nil {
		inventoryHub = NewHub()
		go inventoryHub.Run()
	}
}

// InventoryWebSocket - WebSocket хендлер для инвентаризации
func InventoryWebSocket(c *gin.Context) {
	tenantID := getSupplierTenantID(c)
	userID := getSupplierUserID(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	// Используем upgrader из qr_auth.go
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	if inventoryHub == nil {
		InitInventoryWS()
	}

	client := &WSClient{
		Conn:     conn,
		Send:     make(chan []byte, 256),
		TenantID: tenantID,
		UserID:   userID.String(),
		Hub:      inventoryHub,
	}

	inventoryHub.register <- client

	go client.writePump()
	go client.readPump()
}

// BroadcastInventoryUpdate - отправка обновлений инвентаризации
func BroadcastInventoryUpdate(tenantID string, eventType string, data interface{}) {
	if inventoryHub != nil {
		inventoryHub.SendToRoom(tenantID, eventType, data)
	}
}

// ============================================
// 1. СТРУКТУРЫ ДЛЯ ИНВЕНТАРИЗАЦИИ
// ============================================

// InventoryProduct - товар на складе
type InventoryProduct struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	CategoryID  *uuid.UUID `json:"category_id" db:"category_id"`
	SKU         string     `json:"sku" db:"sku"`
	Name        string     `json:"name" db:"name" binding:"required"`
	Description string     `json:"description" db:"description"`
	Quantity    int        `json:"quantity" db:"quantity"`
	MinQuantity int        `json:"min_quantity" db:"min_quantity"`
	MaxQuantity int        `json:"max_quantity" db:"max_quantity"`
	Unit        string     `json:"unit" db:"unit"`
	Price       float64    `json:"price" db:"price"`
	Cost        float64    `json:"cost" db:"cost"`
	Location    string     `json:"location" db:"location"`
	Barcode     string     `json:"barcode" db:"barcode"`
	ImageURL    string     `json:"image_url" db:"image_url"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// InventoryCategory - категория товаров
type InventoryCategory struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ParentID    *uuid.UUID `json:"parent_id" db:"parent_id"`
	Name        string     `json:"name" db:"name" binding:"required"`
	Description string     `json:"description" db:"description"`
	SortOrder   int        `json:"sort_order" db:"sort_order"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// InventoryTransaction - транзакция движения товара
type InventoryTransaction struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	TenantID      uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ProductID     uuid.UUID  `json:"product_id" db:"product_id"`
	Type          string     `json:"type" db:"type"` // receipt, sale, return, adjustment
	Quantity      int        `json:"quantity" db:"quantity"`
	BeforeQty     int        `json:"before_qty" db:"before_qty"`
	AfterQty      int        `json:"after_qty" db:"after_qty"`
	ReferenceID   *uuid.UUID `json:"reference_id" db:"reference_id"`
	ReferenceType string     `json:"reference_type" db:"reference_type"`
	Notes         string     `json:"notes" db:"notes"`
	UserID        uuid.UUID  `json:"user_id" db:"user_id"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}

// InventoryHandler - обработчик инвентаризации
type InventoryHandler struct {
	db    *sqlx.DB
	redis *redis.Client
}

func NewInventoryHandler(db *sqlx.DB, redis *redis.Client) *InventoryHandler {
	return &InventoryHandler{
		db:    db,
		redis: redis,
	}
}

// ============================================
// 2. ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ДЛЯ КЕША
// ============================================

func (h *InventoryHandler) getCacheKey(prefix string, tenantID string, params ...string) string {
	key := fmt.Sprintf("inventory:%s:%s", prefix, tenantID)
	for _, p := range params {
		if p != "" {
			key += ":" + p
		}
	}
	return key
}

func (h *InventoryHandler) setCache(ctx context.Context, key string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return h.redis.Set(ctx, key, jsonData, 5*time.Minute).Err()
}

func (h *InventoryHandler) getCache(ctx context.Context, key string, dest interface{}) error {
	data, err := h.redis.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (h *InventoryHandler) invalidateCache(ctx context.Context, tenantID string, prefixes ...string) {
	for _, prefix := range prefixes {
		pattern := h.getCacheKey(prefix, tenantID, "*")
		iter := h.redis.Scan(ctx, 0, pattern, 0).Iterator()
		for iter.Next(ctx) {
			h.redis.Del(ctx, iter.Val())
		}
	}
}

// ============================================
// 3. API ТОВАРОВ
// ============================================

func (h *InventoryHandler) GetProducts(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	search := strings.TrimSpace(c.Query("search"))
	categoryID := c.Query("category_id")
	limit, _ := c.GetQuery("limit")

	cacheKey := h.getCacheKey("products", tenantID,
		fmt.Sprintf("search_%s", search),
		fmt.Sprintf("cat_%s", categoryID),
		limit,
	)

	var cachedResponse map[string]interface{}
	if err := h.getCache(ctx, cacheKey, &cachedResponse); err == nil {
		c.JSON(http.StatusOK, cachedResponse)
		return
	}

	query := `
		SELECT 
			id, tenant_id, category_id, sku, name, description,
			quantity, min_quantity, max_quantity, unit, price, cost,
			location, barcode, image_url, is_active, created_at, updated_at
		FROM inventory_products
		WHERE tenant_id = $1 AND is_active = true
	`

	args := []interface{}{tenantID}
	argIndex := 2

	if search != "" {
		query += fmt.Sprintf(" AND (name ILIKE $%d OR sku ILIKE $%d OR barcode ILIKE $%d)", argIndex, argIndex, argIndex)
		args = append(args, "%"+search+"%")
		argIndex++
	}

	if categoryID != "" {
		query += fmt.Sprintf(" AND category_id = $%d", argIndex)
		args = append(args, categoryID)
		argIndex++
	}

	query += " ORDER BY name"

	if limit != "" {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, limit)
	}

	var products []InventoryProduct
	err := h.db.SelectContext(ctx, &products, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"products": products,
		"count":    len(products),
	}

	h.setCache(ctx, cacheKey, response)
	c.JSON(http.StatusOK, response)
}

func (h *InventoryHandler) GetProduct(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	cacheKey := h.getCacheKey("product", tenantID, id)
	var product InventoryProduct
	if err := h.getCache(ctx, cacheKey, &product); err == nil {
		c.JSON(http.StatusOK, gin.H{"product": product})
		return
	}

	err := h.db.GetContext(ctx, &product, `
		SELECT 
			id, tenant_id, category_id, sku, name, description,
			quantity, min_quantity, max_quantity, unit, price, cost,
			location, barcode, image_url, is_active, created_at, updated_at
		FROM inventory_products
		WHERE id = $1 AND tenant_id = $2 AND is_active = true
	`, id, tenantID)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Товар не найден"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	h.setCache(ctx, cacheKey, product)
	c.JSON(http.StatusOK, gin.H{"product": product})
}

func (h *InventoryHandler) CreateProduct(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	userID := getSupplierUserID(c)

	if tenantID == "" || userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id and user_id required"})
		return
	}

	var req InventoryProduct
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	var id uuid.UUID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO inventory_products (
			tenant_id, category_id, sku, name, description,
			quantity, min_quantity, max_quantity, unit, price, cost,
			location, barcode, image_url, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, true, NOW(), NOW())
		RETURNING id
	`, tenantID, req.CategoryID, req.SKU, req.Name, req.Description,
		req.Quantity, req.MinQuantity, req.MaxQuantity, req.Unit,
		req.Price, req.Cost, req.Location, req.Barcode, req.ImageURL).Scan(&id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "products", "stats")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"id":      id,
		"message": "Товар успешно создан",
	})
}

func (h *InventoryHandler) UpdateProduct(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var req InventoryProduct
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.db.ExecContext(ctx, `
		UPDATE inventory_products 
		SET category_id = $1, sku = $2, name = $3, description = $4,
			quantity = $5, min_quantity = $6, max_quantity = $7,
			unit = $8, price = $9, cost = $10,
			location = $11, barcode = $12, image_url = $13,
			is_active = $14, updated_at = NOW()
		WHERE id = $15 AND tenant_id = $16
	`, req.CategoryID, req.SKU, req.Name, req.Description,
		req.Quantity, req.MinQuantity, req.MaxQuantity,
		req.Unit, req.Price, req.Cost,
		req.Location, req.Barcode, req.ImageURL,
		req.IsActive, id, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "products", "product")
	h.invalidateCache(ctx, tenantID, "product", id)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Товар успешно обновлен",
	})
}

func (h *InventoryHandler) DeleteProduct(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	_, err := h.db.ExecContext(ctx, `
		UPDATE inventory_products 
		SET is_active = false, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "products", "product")
	h.invalidateCache(ctx, tenantID, "product", id)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Товар удален",
	})
}

// ============================================
// 4. API КАТЕГОРИЙ
// ============================================

func (h *InventoryHandler) GetCategories(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	cacheKey := h.getCacheKey("categories", tenantID)

	var categories []InventoryCategory
	if err := h.getCache(ctx, cacheKey, &categories); err == nil {
		c.JSON(http.StatusOK, gin.H{"categories": categories})
		return
	}

	err := h.db.SelectContext(ctx, &categories, `
		SELECT 
			id, tenant_id, parent_id, name, description, sort_order, is_active, created_at, updated_at
		FROM inventory_categories
		WHERE tenant_id = $1 AND is_active = true
		ORDER BY sort_order, name
	`, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.setCache(ctx, cacheKey, categories)
	c.JSON(http.StatusOK, gin.H{"categories": categories})
}

func (h *InventoryHandler) CreateCategory(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var req InventoryCategory
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	var id uuid.UUID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO inventory_categories (
			tenant_id, parent_id, name, description, sort_order, is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, true, NOW(), NOW())
		RETURNING id
	`, tenantID, req.ParentID, req.Name, req.Description, req.SortOrder).Scan(&id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "categories")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"id":      id,
		"message": "Категория создана",
	})
}

func (h *InventoryHandler) UpdateCategory(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var req InventoryCategory
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.db.ExecContext(ctx, `
		UPDATE inventory_categories 
		SET parent_id = $1, name = $2, description = $3, 
			sort_order = $4, is_active = $5, updated_at = NOW()
		WHERE id = $6 AND tenant_id = $7
	`, req.ParentID, req.Name, req.Description, req.SortOrder, req.IsActive, id, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "categories")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Категория обновлена",
	})
}

func (h *InventoryHandler) DeleteCategory(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	_, err := h.db.ExecContext(ctx, `
		UPDATE inventory_categories 
		SET is_active = false, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "categories")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Категория удалена",
	})
}

// ============================================
// 5. API СТАТИСТИКИ
// ============================================

func (h *InventoryHandler) GetInventoryStats(c *gin.Context) {
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
		TotalProducts   int     `db:"total_products"`
		TotalQuantity   int     `db:"total_quantity"`
		TotalValue      float64 `db:"total_value"`
		LowStockItems   int     `db:"low_stock_items"`
		CategoriesCount int     `db:"categories_count"`
	}

	err := h.db.GetContext(ctx, &result, `
		SELECT 
			COUNT(*) as total_products,
			COALESCE(SUM(quantity), 0) as total_quantity,
			COALESCE(SUM(quantity * cost), 0) as total_value,
			COUNT(CASE WHEN quantity <= min_quantity THEN 1 END) as low_stock_items,
			(SELECT COUNT(*) FROM inventory_categories WHERE tenant_id = $1 AND is_active = true) as categories_count
		FROM inventory_products
		WHERE tenant_id = $1 AND is_active = true
	`, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	stats = map[string]interface{}{
		"total_products":   result.TotalProducts,
		"total_quantity":   result.TotalQuantity,
		"total_value":      result.TotalValue,
		"low_stock_items":  result.LowStockItems,
		"categories_count": result.CategoriesCount,
	}

	h.setCache(ctx, cacheKey, stats)
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// ============================================
// 6. ДОПОЛНИТЕЛЬНЫЕ МЕТОДЫ ДЛЯ ИНВЕНТАРИЗАЦИИ
// ============================================

// GetLowStock - возвращает товары с низким остатком
func (h *InventoryHandler) GetLowStock(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	threshold := 5
	if t := c.Query("threshold"); t != "" {
		if val, err := strconv.Atoi(t); err == nil && val > 0 {
			threshold = val
		}
	}

	var products []InventoryProduct
	err := h.db.SelectContext(ctx, &products, `
		SELECT 
			id, tenant_id, category_id, sku, name, description,
			quantity, min_quantity, max_quantity, unit, price, cost,
			location, barcode, image_url, is_active, created_at, updated_at
		FROM inventory_products
		WHERE tenant_id = $1 AND is_active = true AND quantity <= $2
		ORDER BY quantity ASC
	`, tenantID, threshold)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"products":  products,
		"count":     len(products),
		"threshold": threshold,
	})
}

// ExportProductsCSV - экспорт товаров в CSV
func (h *InventoryHandler) ExportProductsCSV(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var products []InventoryProduct
	err := h.db.SelectContext(ctx, &products, `
		SELECT 
			id, tenant_id, category_id, sku, name, description,
			quantity, min_quantity, max_quantity, unit, price, cost,
			location, barcode, image_url, is_active, created_at, updated_at
		FROM inventory_products
		WHERE tenant_id = $1 AND is_active = true
		ORDER BY name
	`, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Формируем CSV
	var csv strings.Builder
	csv.WriteString("ID,SKU,Название,Количество,Цена,Себестоимость,Местоположение,Категория\n")

	for _, p := range products {
		categoryName := ""
		if p.CategoryID != nil {
			h.db.GetContext(ctx, &categoryName, `
				SELECT name FROM inventory_categories WHERE id = $1 AND tenant_id = $2
			`, p.CategoryID, tenantID)
		}
		csv.WriteString(fmt.Sprintf("%s,%s,%s,%d,%.2f,%.2f,%s,%s\n",
			p.ID.String(), p.SKU, p.Name, p.Quantity,
			p.Price, p.Cost, p.Location, categoryName))
	}

	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=inventory_products.csv")
	c.String(http.StatusOK, csv.String())
}

// BulkUpdateInventory - массовое обновление товаров
func (h *InventoryHandler) BulkUpdateInventory(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	userID := getSupplierUserID(c)

	if tenantID == "" || userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id and user_id required"})
		return
	}

	var req struct {
		Products []struct {
			ID       uuid.UUID `json:"id" binding:"required"`
			Quantity int       `json:"quantity"`
			Price    float64   `json:"price"`
			Cost     float64   `json:"cost"`
			Location string    `json:"location"`
		} `json:"products" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	updated := 0
	for _, item := range req.Products {
		result, err := tx.ExecContext(ctx, `
			UPDATE inventory_products 
			SET quantity = $1, price = $2, cost = $3, location = $4, updated_at = NOW()
			WHERE id = $5 AND tenant_id = $6 AND is_active = true
		`, item.Quantity, item.Price, item.Cost, item.Location, item.ID, tenantID)

		if err != nil {
			continue
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			updated++
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "products", "stats")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"updated": updated,
		"total":   len(req.Products),
		"message": fmt.Sprintf("Обновлено %d из %d товаров", updated, len(req.Products)),
	})
}

// GetProductMovements - история движения товара
func (h *InventoryHandler) GetProductMovements(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	productID := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	var movements []map[string]interface{}
	err := h.db.SelectContext(ctx, &movements, `
		SELECT 
			id, type, quantity, before_qty, after_qty,
			reference_id, reference_type, notes, user_id, created_at
		FROM inventory_transactions
		WHERE tenant_id = $1 AND product_id = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, tenantID, productID, limit)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"movements":  movements,
		"count":      len(movements),
		"product_id": productID,
	})
}

// ============================================
// 7. УПРАВЛЕНИЕ СКЛАДАМИ (ТЕРМИНАЛАМИ)
// ============================================

// GetWarehouses - список складов клиента
func (h *InventoryHandler) GetWarehouses(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	cacheKey := h.getCacheKey("warehouses", tenantID)
	var warehouses []map[string]interface{}
	if err := h.getCache(ctx, cacheKey, &warehouses); err == nil {
		c.JSON(http.StatusOK, gin.H{"warehouses": warehouses})
		return
	}

	err := h.db.SelectContext(ctx, &warehouses, `
		SELECT 
			id, name, address, phone, email, manager,
			is_active, is_default, 
			COALESCE(terminal_id, '') as terminal_id,
			COALESCE(settings::text, '{}') as settings,
			created_at, updated_at,
			(SELECT COUNT(*) FROM inventory_products WHERE warehouse_id = w.id AND is_active = true) as products_count
		FROM inventory_warehouses w
		WHERE tenant_id = $1 AND is_active = true
		ORDER BY is_default DESC, name
	`, tenantID)

	if err != nil {
		// Если таблицы нет - возвращаем пустой список
		c.JSON(http.StatusOK, gin.H{"warehouses": []interface{}{}, "message": "Таблица складов не создана"})
		return
	}

	h.setCache(ctx, cacheKey, warehouses)
	c.JSON(http.StatusOK, gin.H{"warehouses": warehouses})
}

// CreateWarehouse - создать склад (терминал)
func (h *InventoryHandler) CreateWarehouse(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	userID := getSupplierUserID(c)

	if tenantID == "" || userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id and user_id required"})
		return
	}

	var req struct {
		Name       string `json:"name" binding:"required"`
		Address    string `json:"address"`
		Phone      string `json:"phone"`
		Email      string `json:"email"`
		Manager    string `json:"manager"`
		IsDefault  bool   `json:"is_default"`
		TerminalID string `json:"terminal_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Генерируем Terminal ID
	if req.TerminalID == "" {
		req.TerminalID = fmt.Sprintf("WH-%s-%d", tenantID[:8], time.Now().UnixNano()%10000)
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	// Если это первый склад - делаем его default
	var count int
	h.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM inventory_warehouses WHERE tenant_id = $1`, tenantID)
	if count == 0 {
		req.IsDefault = true
	}

	if req.IsDefault {
		_, _ = tx.ExecContext(ctx, `
			UPDATE inventory_warehouses SET is_default = false WHERE tenant_id = $1
		`, tenantID)
	}

	var id uuid.UUID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO inventory_warehouses (
			tenant_id, name, address, phone, email, manager,
			is_active, is_default, terminal_id, settings, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, true, $7, $8, '{}', NOW(), NOW())
		RETURNING id
	`, tenantID, req.Name, req.Address, req.Phone, req.Email, req.Manager,
		req.IsDefault, req.TerminalID).Scan(&id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "warehouses", "stats")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"id":      id,
		"message": "Склад создан",
	})
}

// UpdateWarehouse - обновить склад
func (h *InventoryHandler) UpdateWarehouse(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var req struct {
		Name      string `json:"name"`
		Address   string `json:"address"`
		Phone     string `json:"phone"`
		Email     string `json:"email"`
		Manager   string `json:"manager"`
		IsDefault bool   `json:"is_default"`
		IsActive  bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	if req.IsDefault {
		_, _ = tx.ExecContext(ctx, `
			UPDATE inventory_warehouses SET is_default = false WHERE tenant_id = $1
		`, tenantID)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE inventory_warehouses 
		SET name = $1, address = $2, phone = $3, email = $4, manager = $5,
			is_default = $6, is_active = $7, updated_at = NOW()
		WHERE id = $8 AND tenant_id = $9
	`, req.Name, req.Address, req.Phone, req.Email, req.Manager,
		req.IsDefault, req.IsActive, id, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "warehouses", "single")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Склад обновлен",
	})
}

// DeleteWarehouse - удалить склад
func (h *InventoryHandler) DeleteWarehouse(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	// Проверяем есть ли товары на складе
	var productsCount int
	h.db.GetContext(ctx, &productsCount, `
		SELECT COUNT(*) FROM inventory_products 
		WHERE warehouse_id = $1 AND tenant_id = $2 AND is_active = true
	`, id, tenantID)

	if productsCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Невозможно удалить склад, на нем есть товары",
			"products_count": productsCount,
		})
		return
	}

	_, err := h.db.ExecContext(ctx, `
		UPDATE inventory_warehouses 
		SET is_active = false, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "warehouses", "single")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Склад удален",
	})
}

// GetWarehouseStats - статистика по складу
func (h *InventoryHandler) GetWarehouseStats(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var stats struct {
		TotalProducts int     `db:"total_products"`
		TotalQuantity int     `db:"total_quantity"`
		TotalValue    float64 `db:"total_value"`
		LowStock      int     `db:"low_stock"`
	}

	err := h.db.GetContext(ctx, &stats, `
		SELECT 
			COUNT(*) as total_products,
			COALESCE(SUM(quantity), 0) as total_quantity,
			COALESCE(SUM(quantity * cost), 0) as total_value,
			COUNT(CASE WHEN quantity <= min_quantity THEN 1 END) as low_stock
		FROM inventory_products
		WHERE tenant_id = $1 AND warehouse_id = $2 AND is_active = true
	`, tenantID, id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

// ============================================
// 8. ПОДКЛЮЧЕНИЕ ТЕРМИНАЛОВ
// ============================================

// ConnectTerminal - подключить терминал к складу
func (h *InventoryHandler) ConnectTerminal(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	var req struct {
		WarehouseID uuid.UUID `json:"warehouse_id" binding:"required"`
		TerminalID  string    `json:"terminal_id" binding:"required"`
		DeviceName  string    `json:"device_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Проверяем существование склада
	var exists bool
	err := h.db.GetContext(ctx, &exists, `
		SELECT EXISTS(SELECT 1 FROM inventory_warehouses 
		WHERE id = $1 AND tenant_id = $2 AND is_active = true)
	`, req.WarehouseID, tenantID)
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Склад не найден"})
		return
	}

	// Обновляем terminal_id
	_, err = h.db.ExecContext(ctx, `
		UPDATE inventory_warehouses 
		SET terminal_id = $1, settings = jsonb_set(
			COALESCE(settings, '{}'::jsonb), 
			'{device_name}', 
			to_jsonb($2)
		), updated_at = NOW()
		WHERE id = $3 AND tenant_id = $4
	`, req.TerminalID, req.DeviceName, req.WarehouseID, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "warehouses", "single")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Терминал подключен",
		"terminal_id": req.TerminalID,
	})
}

// DisconnectTerminal - отключить терминал
func (h *InventoryHandler) DisconnectTerminal(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	warehouseID := c.Param("id")

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	_, err := h.db.ExecContext(ctx, `
		UPDATE inventory_warehouses 
		SET terminal_id = NULL, updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2
	`, warehouseID, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "warehouses", "single")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Терминал отключен",
	})
}

// ============================================
// 9. МИГРАЦИЯ ТАБЛИЦ
// ============================================

// CreateWarehouseTables - создает таблицы для складов
func (h *InventoryHandler) CreateWarehouseTables(c *gin.Context) {
    ctx := c.Request.Context()
    tenantID := getSupplierTenantID(c)

    if tenantID == "" || !isAdminUser(c) {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
        return
    }
	queries := []string{
		// Таблица складов
		`CREATE TABLE IF NOT EXISTS inventory_warehouses (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL,
			name VARCHAR(255) NOT NULL,
			address TEXT,
			phone VARCHAR(50),
			email VARCHAR(255),
			manager VARCHAR(255),
			is_active BOOLEAN DEFAULT true,
			is_default BOOLEAN DEFAULT false,
			terminal_id VARCHAR(100),
			settings JSONB DEFAULT '{}',
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,

		// Индексы для складов
		`CREATE INDEX IF NOT EXISTS idx_warehouses_tenant ON inventory_warehouses(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_warehouses_terminal ON inventory_warehouses(terminal_id)`,

		// Добавляем warehouse_id в товары
		`ALTER TABLE inventory_products ADD COLUMN IF NOT EXISTS warehouse_id UUID`,

		// Индекс для товаров по складу
		`CREATE INDEX IF NOT EXISTS idx_products_warehouse ON inventory_products(warehouse_id)`,

		// Таблица движений с warehouse_id
		`ALTER TABLE inventory_transactions ADD COLUMN IF NOT EXISTS warehouse_id UUID`,

		// Индекс для движений по складу
		`CREATE INDEX IF NOT EXISTS idx_transactions_warehouse ON inventory_transactions(warehouse_id)`,

		// Добавляем reserved колонку в товары
		`ALTER TABLE inventory_products ADD COLUMN IF NOT EXISTS reserved INT DEFAULT 0`,
	}

	for _, q := range queries {
		_, err := h.db.ExecContext(ctx, q)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "query": q})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Таблицы складов созданы",
	})
}
// ============================================
// 11. API ДЛЯ ТЕРМИНАЛОВ (СКАНЕРЫ, КАССЫ)
// ============================================

// RegisterTerminal - регистрация нового терминала
func (h *InventoryHandler) RegisterTerminal(c *gin.Context) {
    ctx := c.Request.Context()
    tenantID := getSupplierTenantID(c)
    userID := getSupplierUserID(c)

    if tenantID == "" || userID == uuid.Nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id and user_id required"})
        return
    }

    var req struct {
        Name        string `json:"name" binding:"required"`
        WarehouseID string `json:"warehouse_id" binding:"required"`
        DeviceType  string `json:"device_type"` // scanner, pos, mobile, tablet
        DeviceID    string `json:"device_id"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Проверяем склад
    var exists bool
    err := h.db.GetContext(ctx, &exists, `
        SELECT EXISTS(SELECT 1 FROM inventory_warehouses 
        WHERE id = $1 AND tenant_id = $2 AND is_active = true)
    `, req.WarehouseID, tenantID)
    if err != nil || !exists {
        c.JSON(http.StatusNotFound, gin.H{"error": "Склад не найден"})
        return
    }

    // Генерируем terminal_id
    terminalID := fmt.Sprintf("TRM-%s-%d", tenantID[:8], time.Now().UnixNano()%10000)

    // Генерируем API ключ для терминала
    apiKey := fmt.Sprintf("term_%s_%d", uuid.New().String()[:8], time.Now().Unix())

    var id uuid.UUID
    err = h.db.QueryRowContext(ctx, `
        INSERT INTO inventory_terminals (
            tenant_id, warehouse_id, name, terminal_id, device_type, 
            device_id, api_key, status, last_active, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, 'active', NOW(), NOW(), NOW())
        RETURNING id
    `, tenantID, req.WarehouseID, req.Name, terminalID, req.DeviceType,
        req.DeviceID, apiKey).Scan(&id)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    h.invalidateCache(ctx, tenantID, "terminals", "warehouses")
    c.JSON(http.StatusOK, gin.H{
        "success":     true,
        "id":          id,
        "terminal_id": terminalID,
        "api_key":     apiKey,
        "message":     "Терминал зарегистрирован",
    })
}

// GetTerminals - список терминалов
func (h *InventoryHandler) GetTerminals(c *gin.Context) {
    ctx := c.Request.Context()
    tenantID := getSupplierTenantID(c)

    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
        return
    }

    var terminals []map[string]interface{}
    err := h.db.SelectContext(ctx, &terminals, `
        SELECT 
            t.id, t.name, t.terminal_id, t.device_type, t.device_id,
            t.status, t.api_key, t.last_active, t.created_at,
            w.name as warehouse_name,
            w.id as warehouse_id
        FROM inventory_terminals t
        LEFT JOIN inventory_warehouses w ON t.warehouse_id = w.id
        WHERE t.tenant_id = $1 AND t.status = 'active'
        ORDER BY t.created_at DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"terminals": terminals})
}

// TerminalScan - сканирование товара терминалом
func (h *InventoryHandler) TerminalScan(c *gin.Context) {
    ctx := c.Request.Context()
    tenantID := getSupplierTenantID(c)

    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
        return
    }

    // Проверяем API ключ терминала
    apiKey := c.GetHeader("X-Terminal-API-Key")
    if apiKey == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "X-Terminal-API-Key required"})
        return
    }

    var terminal struct {
        ID          uuid.UUID `db:"id"`
        WarehouseID uuid.UUID `db:"warehouse_id"`
        Status      string    `db:"status"`
    }
    err := h.db.GetContext(ctx, &terminal, `
        SELECT id, warehouse_id, status FROM inventory_terminals
        WHERE api_key = $1 AND tenant_id = $2 AND status = 'active'
    `, apiKey, tenantID)

    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Недействительный API ключ терминала"})
        return
    }

    var req struct {
        Barcode     string `json:"barcode" binding:"required"`
        Quantity    int    `json:"quantity" binding:"required,min=1"`
        Action      string `json:"action"` // add, remove, set
        StorageLocation string `json:"storage_location"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Находим товар по штрих-коду
    var product struct {
        ID          uuid.UUID `db:"id"`
        Name        string    `db:"name"`
        CurrentQty  int       `db:"quantity"`
        WarehouseID *uuid.UUID `db:"warehouse_id"`
    }
    err = h.db.GetContext(ctx, &product, `
        SELECT id, name, quantity, warehouse_id FROM inventory_products
        WHERE barcode = $1 AND tenant_id = $2 AND is_active = true
    `, req.Barcode, tenantID)

    if err != nil {
        if err == sql.ErrNoRows {
            c.JSON(http.StatusNotFound, gin.H{"error": "Товар с таким штрих-кодом не найден"})
        } else {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        }
        return
    }

    // Обновляем количество
    newQty := product.CurrentQty
    action := "add"
    if req.Action == "remove" {
        newQty = product.CurrentQty - req.Quantity
        action = "remove"
        if newQty < 0 {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Недостаточно товара на складе"})
            return
        }
    } else if req.Action == "set" {
        newQty = req.Quantity
        action = "set"
    } else {
        newQty = product.CurrentQty + req.Quantity
        action = "add"
    }

    // Записываем в транзакцию
    _, err = h.db.ExecContext(ctx, `
        INSERT INTO inventory_transactions (
            tenant_id, product_id, warehouse_id, type, quantity,
            before_qty, after_qty, reference_type, notes, user_id, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, 'terminal_scan', $8, $9, NOW())
    `, tenantID, product.ID, terminal.WarehouseID, action, req.Quantity,
        product.CurrentQty, newQty, "Сканирование терминалом", terminal.ID)

    // Обновляем товар
    _, err = h.db.ExecContext(ctx, `
        UPDATE inventory_products 
        SET quantity = $1, updated_at = NOW()
        WHERE id = $2 AND tenant_id = $3
    `, newQty, product.ID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    h.invalidateCache(ctx, tenantID, "products", "product")

    // Обновляем время активности терминала
    h.db.ExecContext(ctx, `
        UPDATE inventory_terminals 
        SET last_active = NOW()
        WHERE id = $1
    `, terminal.ID)

    // Отправляем обновление через WebSocket
    BroadcastInventoryUpdate(tenantID, "product_updated", gin.H{
        "product_id": product.ID,
        "name":       product.Name,
        "quantity":   newQty,
        "action":     action,
        "terminal":   terminal.ID,
    })

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "product_id": product.ID,
        "name":       product.Name,
        "old_qty":    product.CurrentQty,
        "new_qty":    newQty,
        "action":     action,
        "message":    fmt.Sprintf("Товар обновлен: %d → %d", product.CurrentQty, newQty),
    })
}

// GetTerminalStats - статистика терминала
func (h *InventoryHandler) GetTerminalStats(c *gin.Context) {
    ctx := c.Request.Context()
    tenantID := getSupplierTenantID(c)
    terminalID := c.Param("id")

    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
        return
    }

    var stats struct {
        TotalScans   int     `db:"total_scans"`
        ProductsAdded int    `db:"products_added"`
        ProductsRemoved int `db:"products_removed"`
        LastActive   time.Time `db:"last_active"`
    }

    err := h.db.GetContext(ctx, &stats, `
        SELECT 
            COUNT(*) as total_scans,
            COUNT(CASE WHEN type = 'add' THEN 1 END) as products_added,
            COUNT(CASE WHEN type = 'remove' THEN 1 END) as products_removed,
            COALESCE(MAX(created_at), NOW()) as last_active
        FROM inventory_transactions
        WHERE tenant_id = $1 AND reference_type = 'terminal_scan'
        AND reference_id = $2
    `, tenantID, terminalID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"stats": stats})
}