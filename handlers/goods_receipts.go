package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

// ============================================
// 1. СТРУКТУРЫ ДЛЯ ПРИЕМКИ ТОВАРОВ
// ============================================

type GoodsReceipt struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	TenantID        uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	UserID          uuid.UUID  `json:"user_id" db:"user_id"`
	ReceiptNumber   string     `json:"receipt_number" db:"receipt_number"`
	PurchaseOrderID *uuid.UUID `json:"purchase_order_id" db:"purchase_order_id"`
	SupplierID      *uuid.UUID `json:"supplier_id" db:"supplier_id"`
	SupplierName    string     `json:"supplier_name" db:"supplier_name"`
	ReceiptDate     time.Time  `json:"receipt_date" db:"receipt_date"`
	Status          string     `json:"status" db:"status"`
	TotalAmount     float64    `json:"total_amount" db:"total_amount"`
	ReceivedBy      string     `json:"received_by" db:"received_by"`
	Notes           string     `json:"notes" db:"notes"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

type GoodsReceiptItem struct {
	ID               uuid.UUID   `json:"id" db:"id"`
	ReceiptID        uuid.UUID   `json:"receipt_id" db:"receipt_id"`
	ProductID        uuid.UUID   `json:"product_id" db:"product_id"`
	ProductName      string      `json:"product_name" db:"product_name"`
	SKU              string      `json:"sku" db:"sku"`
	OrderItemID      *uuid.UUID  `json:"order_item_id" db:"order_item_id"`
	Quantity         int         `json:"quantity" db:"quantity"`
	AcceptedQuantity int         `json:"accepted_quantity" db:"accepted_quantity"`
	RejectedQuantity int         `json:"rejected_quantity" db:"rejected_quantity"`
	Price            float64     `json:"price" db:"price"`
	Total            float64     `json:"total" db:"total"`
	RejectionReason  string      `json:"rejection_reason" db:"rejection_reason"`
	BatchNumber      string      `json:"batch_number" db:"batch_number"`
	ExpirationDate   *time.Time  `json:"expiration_date" db:"expiration_date"`
	StorageLocation  string      `json:"storage_location" db:"storage_location"`
	Notes            string      `json:"notes" db:"notes"`
	CreatedAt        time.Time   `json:"created_at" db:"created_at"`
}

// GoodsReceiptHandler - обработчик приемки товаров
type GoodsReceiptHandler struct {
	db    *sqlx.DB
	redis *redis.Client
}

func NewGoodsReceiptHandler(db *sqlx.DB, redis *redis.Client) *GoodsReceiptHandler {
	return &GoodsReceiptHandler{
		db:    db,
		redis: redis,
	}
}

// ============================================
// 2. ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ДЛЯ КЕША
// ============================================

func (h *GoodsReceiptHandler) getCacheKey(prefix string, tenantID string, params ...string) string {
	key := fmt.Sprintf("receipt:%s:%s", prefix, tenantID)
	for _, p := range params {
		if p != "" {
			key += ":" + p
		}
	}
	return key
}

func (h *GoodsReceiptHandler) setCache(ctx context.Context, key string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return h.redis.Set(ctx, key, jsonData, 5*time.Minute).Err()
}

func (h *GoodsReceiptHandler) getCache(ctx context.Context, key string, dest interface{}) error {
	data, err := h.redis.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (h *GoodsReceiptHandler) invalidateCache(ctx context.Context, tenantID string, prefixes ...string) {
	for _, prefix := range prefixes {
		pattern := h.getCacheKey(prefix, tenantID, "*")
		iter := h.redis.Scan(ctx, 0, pattern, 0).Iterator()
		for iter.Next(ctx) {
			h.redis.Del(ctx, iter.Val())
		}
	}
}

// ============================================
// 3. GET GOODS RECEIPTS
// ============================================

func (h *GoodsReceiptHandler) GetGoodsReceipts(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	userID := getUserID(c)
	isAdmin := isAdminUser(c)

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	purchaseOrderID := strings.TrimSpace(c.Query("purchase_order_id"))
	supplierID := strings.TrimSpace(c.Query("supplier_id"))
	dateFrom := strings.TrimSpace(c.Query("date_from"))
	dateTo := strings.TrimSpace(c.Query("date_to"))
	// limit, _ := c.GetQuery("limit") // Удаляем неиспользуемую переменную
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	offset := (page - 1) * pageSize

	cacheKey := h.getCacheKey("list", tenantID,
		fmt.Sprintf("status_%s", status),
		fmt.Sprintf("po_%s", purchaseOrderID),
		fmt.Sprintf("supp_%s", supplierID),
		fmt.Sprintf("from_%s", dateFrom),
		fmt.Sprintf("to_%s", dateTo),
		fmt.Sprintf("p_%d", page),
		fmt.Sprintf("ps_%d", pageSize),
	)

	var cachedResponse map[string]interface{}
	if err := h.getCache(ctx, cacheKey, &cachedResponse); err == nil {
		c.JSON(http.StatusOK, cachedResponse)
		return
	}

	query := `
		SELECT 
			gr.id, gr.tenant_id, gr.user_id, gr.receipt_number, 
			gr.purchase_order_id, gr.supplier_id, 
			COALESCE(s.name, '') as supplier_name,
			gr.receipt_date, gr.status, gr.total_amount, 
			gr.received_by, gr.notes, gr.created_at, gr.updated_at,
			COUNT(gri.id) as items_count,
			COALESCE(SUM(gri.accepted_quantity), 0) as total_quantity
		FROM goods_receipts gr
		LEFT JOIN suppliers s ON gr.supplier_id = s.id AND s.tenant_id = gr.tenant_id
		LEFT JOIN goods_receipt_items gri ON gr.id = gri.receipt_id
		WHERE gr.tenant_id = $1
	`

	args := []interface{}{tenantID}
	argIndex := 2

	if !isAdmin && userID != uuid.Nil {
		query += fmt.Sprintf(" AND gr.user_id = $%d", argIndex)
		args = append(args, userID)
		argIndex++
	}

	if status != "" {
		query += fmt.Sprintf(" AND gr.status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}

	if purchaseOrderID != "" {
		query += fmt.Sprintf(" AND gr.purchase_order_id = $%d", argIndex)
		args = append(args, purchaseOrderID)
		argIndex++
	}

	if supplierID != "" {
		query += fmt.Sprintf(" AND gr.supplier_id = $%d", argIndex)
		args = append(args, supplierID)
		argIndex++
	}

	if dateFrom != "" {
		query += fmt.Sprintf(" AND gr.receipt_date >= $%d", argIndex)
		args = append(args, dateFrom)
		argIndex++
	}
	if dateTo != "" {
		query += fmt.Sprintf(" AND gr.receipt_date <= $%d", argIndex)
		args = append(args, dateTo+" 23:59:59")
		argIndex++
	}

	query += `
		GROUP BY gr.id, s.name
		ORDER BY gr.receipt_date DESC
	`

	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIndex, argIndex+1)
	args = append(args, pageSize, offset)

	type ReceiptWithStats struct {
		GoodsReceipt
		ItemsCount    int `db:"items_count"`
		TotalQuantity int `db:"total_quantity"`
	}

	var receipts []ReceiptWithStats
	err := h.db.SelectContext(ctx, &receipts, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var total int
	countQuery := strings.Replace(query,
		`SELECT gr.id, gr.tenant_id, gr.user_id, gr.receipt_number, gr.purchase_order_id, gr.supplier_id, COALESCE(s.name, '') as supplier_name, gr.receipt_date, gr.status, gr.total_amount, gr.received_by, gr.notes, gr.created_at, gr.updated_at, COUNT(gri.id) as items_count, COALESCE(SUM(gri.accepted_quantity), 0) as total_quantity`,
		"SELECT COUNT(DISTINCT gr.id)",
		1,
	)
	countQuery = strings.Replace(countQuery, `GROUP BY gr.id, s.name`, "", 1)
	countQuery = strings.Replace(countQuery, `LIMIT $%d OFFSET $%d`, "", 1)

	err = h.db.GetContext(ctx, &total, countQuery, args[:len(args)-2]...)
	if err != nil {
		total = len(receipts)
	}

	response := gin.H{
		"receipts":    receipts,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + pageSize - 1) / pageSize,
	}

	h.setCache(ctx, cacheKey, response)
	c.JSON(http.StatusOK, response)
}

// ============================================
// 4. GET GOODS RECEIPT BY ID
// ============================================

func (h *GoodsReceiptHandler) GetGoodsReceipt(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
	id := c.Param("id")                    // ← получаем ID из URL
	receiptID := id                        // ← объявляем receiptID

	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id required"})
		return
	}

	cacheKey := h.getCacheKey("single", tenantID, receiptID)

	var cachedData map[string]interface{}
	if err := h.getCache(ctx, cacheKey, &cachedData); err == nil {
		c.JSON(http.StatusOK, cachedData)
		return
	}

	var receipt GoodsReceipt
	err := h.db.GetContext(ctx, &receipt, `
		SELECT 
			gr.id, gr.tenant_id, gr.user_id, gr.receipt_number, 
			gr.purchase_order_id, gr.supplier_id, 
			COALESCE(s.name, '') as supplier_name,
			gr.receipt_date, gr.status, gr.total_amount, 
			gr.received_by, gr.notes, gr.created_at, gr.updated_at
		FROM goods_receipts gr
		LEFT JOIN suppliers s ON gr.supplier_id = s.id AND s.tenant_id = gr.tenant_id
		WHERE gr.id = $1 AND gr.tenant_id = $2
	`, receiptID, tenantID)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Приемка не найдена"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	var items []GoodsReceiptItem
	err = h.db.SelectContext(ctx, &items, `
		SELECT 
			id, receipt_id, product_id, product_name, sku, 
			order_item_id, quantity, accepted_quantity, rejected_quantity,
			price, total, rejection_reason, batch_number, 
			expiration_date, storage_location, notes, created_at
		FROM goods_receipt_items
		WHERE receipt_id = $1
		ORDER BY created_at ASC
	`, receiptID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"receipt": receipt,
		"items":   items,
	}

	h.setCache(ctx, cacheKey, response)
	c.JSON(http.StatusOK, response)
}

// ============================================
// 5. CREATE GOODS RECEIPT
// ============================================

func (h *GoodsReceiptHandler) CreateGoodsReceipt(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := getSupplierTenantID(c)
userID := getSupplierUserID(c)

	if tenantID == "" || userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tenant_id and user_id required"})
		return
	}

	var req struct {
		PurchaseOrderID *uuid.UUID `json:"purchase_order_id"`
		SupplierID      *uuid.UUID `json:"supplier_id"`
		ReceiptDate     string     `json:"receipt_date"`
		ReceivedBy      string     `json:"received_by"`
		Notes           string     `json:"notes"`
		Items           []struct {
			ProductID        uuid.UUID  `json:"product_id" binding:"required"`
			OrderItemID      *uuid.UUID `json:"order_item_id"`
			Quantity         int        `json:"quantity" binding:"required,min=1"`
			AcceptedQuantity int        `json:"accepted_quantity"`
			RejectedQuantity int        `json:"rejected_quantity"`
			Price            float64    `json:"price" binding:"required,min=0"`
			RejectionReason  string     `json:"rejection_reason"`
			BatchNumber      string     `json:"batch_number"`
			ExpirationDate   string     `json:"expiration_date"`
			StorageLocation  string     `json:"storage_location"`
			Notes            string     `json:"notes"`
		} `json:"items" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for i, item := range req.Items {
		if item.AcceptedQuantity == 0 {
			req.Items[i].AcceptedQuantity = item.Quantity
		}
		if item.RejectedQuantity > 0 && item.RejectionReason == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Укажите причину брака для товара #" + item.ProductID.String(),
			})
			return
		}
	}

	if req.PurchaseOrderID != nil {
		var exists bool
		err := h.db.GetContext(ctx, &exists, `
			SELECT EXISTS(SELECT 1 FROM purchase_orders 
			WHERE id = $1 AND tenant_id = $2)
		`, req.PurchaseOrderID, tenantID)
		if err != nil || !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Заказ поставщику не найден"})
			return
		}
	}

	receiptNumber := fmt.Sprintf("RC-%d-%d", time.Now().Year(), time.Now().UnixNano()%1000000)

	tx, err := h.db.BeginTxx(ctx, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	receiptDate := time.Now()
	if req.ReceiptDate != "" {
		parsed, err := time.Parse("2006-01-02", req.ReceiptDate)
		if err == nil {
			receiptDate = parsed
		}
	}

	totalAmount := 0.0
	for _, item := range req.Items {
		totalAmount += item.Price * float64(item.AcceptedQuantity)
	}

	var receiptID uuid.UUID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO goods_receipts (
			tenant_id, user_id, receipt_number, purchase_order_id, supplier_id,
			receipt_date, status, total_amount, received_by, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'completed', $7, $8, $9, NOW(), NOW())
		RETURNING id
	`, tenantID, userID, receiptNumber, req.PurchaseOrderID, req.SupplierID,
		receiptDate, totalAmount, req.ReceivedBy, req.Notes).Scan(&receiptID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, item := range req.Items {
		var productName, sku string
		err = h.db.GetContext(ctx, &productName, `
			SELECT name FROM inventory_products 
			WHERE id = $1 AND tenant_id = $2 AND is_active = true
		`, item.ProductID, tenantID)

		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Товар не найден: " + item.ProductID.String(),
			})
			return
		}

		h.db.GetContext(ctx, &sku, `
			SELECT COALESCE(sku, '') FROM inventory_products 
			WHERE id = $1 AND tenant_id = $2
		`, item.ProductID, tenantID)

		var expirationDate *time.Time
		if item.ExpirationDate != "" {
			ed, err := time.Parse("2006-01-02", item.ExpirationDate)
			if err == nil {
				expirationDate = &ed
			}
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO goods_receipt_items (
				receipt_id, product_id, product_name, sku, order_item_id,
				quantity, accepted_quantity, rejected_quantity,
				price, total, rejection_reason, batch_number,
				expiration_date, storage_location, notes, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
		`, receiptID, item.ProductID, productName, sku, item.OrderItemID,
			item.Quantity, item.AcceptedQuantity, item.RejectedQuantity,
			item.Price, item.Price*float64(item.AcceptedQuantity),
			item.RejectionReason, item.BatchNumber,
			expirationDate, item.StorageLocation, item.Notes)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE inventory_products 
			SET quantity = quantity + $1, 
				updated_at = NOW()
			WHERE id = $2 AND tenant_id = $3
		`, item.AcceptedQuantity, item.ProductID, tenantID)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if item.OrderItemID != nil {
			_, _ = tx.ExecContext(ctx, `
				UPDATE purchase_order_items 
				SET received_quantity = COALESCE(received_quantity, 0) + $1,
					updated_at = NOW()
				WHERE id = $2
			`, item.AcceptedQuantity, item.OrderItemID)
		}
	}

	if req.PurchaseOrderID != nil {
		var pendingItems int
		err = tx.GetContext(ctx, &pendingItems, `
			SELECT COUNT(*) FROM purchase_order_items poi
			LEFT JOIN goods_receipt_items gri ON poi.id = gri.order_item_id
			WHERE poi.purchase_order_id = $1 
			AND (gri.id IS NULL OR poi.quantity > COALESCE(gri.accepted_quantity, 0))
		`, req.PurchaseOrderID)

		if err == nil && pendingItems == 0 {
			_, _ = tx.ExecContext(ctx, `
				UPDATE purchase_orders 
				SET status = 'delivered', updated_at = NOW()
				WHERE id = $1 AND tenant_id = $2
			`, req.PurchaseOrderID, tenantID)
		}
	}

	_, _ = tx.ExecContext(ctx, `
		INSERT INTO receipt_logs (receipt_id, action, user_id, tenant_id, created_at)
		VALUES ($1, 'create', $2, $3, NOW())
	`, receiptID, userID, tenantID)

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.invalidateCache(ctx, tenantID, "list", "stats")

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"receipt_id":     receiptID,
		"receipt_number": receiptNumber,
		"total_amount":   totalAmount,
		"message":        "Товары успешно оприходованы",
	})
}

// ============================================
// 6. GET RECEIPT STATS
// ============================================

func (h *GoodsReceiptHandler) GetReceiptStats(c *gin.Context) {
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
		TotalReceipts      int     `db:"total_receipts"`
		TotalItems         int     `db:"total_items"`
		TotalQuantity      int     `db:"total_quantity"`
		TotalAmount        float64 `db:"total_amount"`
		ReceiptsToday      int     `db:"receipts_today"`
		ReceiptsMonth      int     `db:"receipts_month"`
		AvgItemsPerReceipt float64 `db:"avg_items_per_receipt"`
	}

	err := h.db.GetContext(ctx, &result, `
		SELECT 
			COUNT(DISTINCT gr.id) as total_receipts,
			COUNT(gri.id) as total_items,
			COALESCE(SUM(gri.accepted_quantity), 0) as total_quantity,
			COALESCE(SUM(gr.total_amount), 0) as total_amount,
			COUNT(DISTINCT CASE WHEN gr.receipt_date::date = CURRENT_DATE THEN gr.id END) as receipts_today,
			COUNT(DISTINCT CASE WHEN gr.receipt_date::date >= DATE_TRUNC('month', CURRENT_DATE) THEN gr.id END) as receipts_month,
			COALESCE(AVG(item_count.count), 0) as avg_items_per_receipt
		FROM goods_receipts gr
		LEFT JOIN goods_receipt_items gri ON gr.id = gri.receipt_id
		LEFT JOIN (
			SELECT receipt_id, COUNT(*) as count
			FROM goods_receipt_items
			GROUP BY receipt_id
		) item_count ON gr.id = item_count.receipt_id
		WHERE gr.tenant_id = $1
		AND gr.status = 'completed'
	`, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	stats = map[string]interface{}{
		"total_receipts":        result.TotalReceipts,
		"total_items":           result.TotalItems,
		"total_quantity":        result.TotalQuantity,
		"total_amount":          result.TotalAmount,
		"receipts_today":        result.ReceiptsToday,
		"receipts_month":        result.ReceiptsMonth,
		"avg_items_per_receipt": result.AvgItemsPerReceipt,
	}

	h.setCache(ctx, cacheKey, stats)
	c.JSON(http.StatusOK, gin.H{"stats": stats})
}


