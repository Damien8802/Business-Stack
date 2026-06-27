package handlers

import (
    "log"
    "net/http"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    
    "subscription-system/database"  // ⚠️ ЭТОТ ИМПОРТ ДОЛЖЕН БЫТЬ!
    "subscription-system/services"
)

type IndividualOrdersHandler struct {
    priceSearch *services.PriceSearchService
    telegramBot *services.TelegramNotifier
}

func NewIndividualOrdersHandler(yandexAPIKey, yandexFolderID, telegramBotToken, telegramChatID, adminChatID string) *IndividualOrdersHandler {
    return &IndividualOrdersHandler{
        priceSearch: services.NewPriceSearchService(yandexAPIKey, yandexFolderID),
        telegramBot: services.NewTelegramNotifier(telegramBotToken, telegramChatID, adminChatID),
    }
}

func (h *IndividualOrdersHandler) OrderPage(c *gin.Context) {
    c.HTML(http.StatusOK, "individual_order.html", gin.H{
        "title": "Заказать разработку - Business Stack",
    })
}

func (h *IndividualOrdersHandler) AdminOrdersPage(c *gin.Context) {
    c.HTML(http.StatusOK, "admin_orders.html", gin.H{
        "title": "Управление заказами - Business Stack",
    })
}

func (h *IndividualOrdersHandler) GetPrice(c *gin.Context) {
    serviceType := c.Query("service")
    if serviceType == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "service parameter required"})
        return
    }
    
    result, _ := h.priceSearch.SearchPrice(serviceType)
    c.JSON(http.StatusOK, gin.H{
        "service":   serviceType,
        "avg_price": result.AvgPrice,
        "min_price": result.MinPrice,
        "max_price": result.MaxPrice,
        "sources":   result.SourcesCount,
        "message":   result.Message,
    })
}

func (h *IndividualOrdersHandler) GetServices(c *gin.Context) {
    services := []gin.H{
        {"id": "1", "name": "Телеграм бот", "category_id": "1"},
        {"id": "2", "name": "Чат-бот с ИИ", "category_id": "1"},
        {"id": "3", "name": "Интернет-магазин", "category_id": "2"},
    }
    c.JSON(http.StatusOK, services)
}

func (h *IndividualOrdersHandler) GetCategories(c *gin.Context) {
    categories := []gin.H{
        {"id": "1", "name": "Чат-боты", "icon": "🤖"},
        {"id": "2", "name": "Интернет-магазины", "icon": "🛒"},
        {"id": "3", "name": "CRM системы", "icon": "📊"},
    }
    c.JSON(http.StatusOK, categories)
}

// ✅ ГЛАВНАЯ ФУНКЦИЯ - СОХРАНЯЕТ В БАЗУ
func (h *IndividualOrdersHandler) CreateOrder(c *gin.Context) {
    log.Println("📥 [CreateOrder] Получен запрос")
    
    var req struct {
        ServiceName    string  `json:"service_name"`
        Requirements   string  `json:"requirements"`
        EstimatedPrice *float64 `json:"estimated_price"`
        Budget         float64  `json:"budget"`         // ← ДОБАВЛЕНО!
        ClientName     string  `json:"client_name"`
        ClientPhone    string  `json:"client_phone"`
        ClientEmail    string  `json:"client_email"`
        ClientTelegram string  `json:"client_telegram"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
        return
    }
    
    log.Printf("📥 Данные: Name=%s, Phone=%s, Service=%s, Budget=%v", 
        req.ClientName, req.ClientPhone, req.ServiceName, req.Budget)
    
    // Проверка обязательных полей
    if req.ServiceName == "" {
        log.Printf("❌ Пустое поле: service_name")
        c.JSON(http.StatusBadRequest, gin.H{"error": "Укажите название услуги"})
        return
    }
    if req.Requirements == "" {
        log.Printf("❌ Пустое поле: requirements")
        c.JSON(http.StatusBadRequest, gin.H{"error": "Опишите требования"})
        return
    }
    if req.ClientName == "" {
        log.Printf("❌ Пустое поле: client_name")
        c.JSON(http.StatusBadRequest, gin.H{"error": "Укажите ваше имя"})
        return
    }
    if req.ClientPhone == "" {
        log.Printf("❌ Пустое поле: client_phone")
        c.JSON(http.StatusBadRequest, gin.H{"error": "Укажите телефон для связи"})
        return
    }
    
    // ✅ СОХРАНЯЕМ В БАЗУ С BUDGET
    log.Printf("✅ Сохраняем в БД: name=%s, phone=%s, budget=%v", 
        req.ClientName, req.ClientPhone, req.Budget)
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO individual_orders (name, phone, description, budget, status, created_at)
        VALUES ($1, $2, $3, $4, 'new', NOW())
    `, req.ClientName, req.ClientPhone, req.Requirements, req.Budget)
    
    if err != nil {
        log.Printf("❌ Ошибка сохранения в БД: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Ошибка сохранения заявки: " + err.Error(),
        })
        return
    }
    
    log.Printf("✅ Индивидуальный заказ сохранён: %s (%s) с бюджетом %v", 
        req.ClientName, req.ClientPhone, req.Budget)
    
    // Отправляем уведомление в Telegram
    go h.telegramBot.SendOrderNotification(nil)
    
    c.JSON(http.StatusCreated, gin.H{
        "success": true,
        "message": "✅ Заявка отправлена! Мы свяжемся с вами в ближайшее время.",
        "order_id": uuid.New().String(),
    })
}
func (h *IndividualOrdersHandler) GetOrders(c *gin.Context) {
    log.Println("📋 [GetOrders] Запрос списка заказов")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, phone, description, budget, status, created_at
        FROM individual_orders
        ORDER BY created_at DESC
    `)
    if err != nil {
        log.Printf("❌ Ошибка получения заказов: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var orders []gin.H
    for rows.Next() {
        var id int
        var name, phone, description, status string
        var budget float64
        var createdAt time.Time
        
        err := rows.Scan(&id, &name, &phone, &description, &budget, &status, &createdAt)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
            continue
        }
        
        orders = append(orders, gin.H{
            "id":            id,
            "client_name":   name,
            "client_phone":  phone,
            "requirements":  description,
            "budget":        budget,        // ← ДОБАВЛЕНО!
            "status":        status,
            "created_at":    createdAt,
        })
    }
    
    log.Printf("📋 Найдено заказов: %d", len(orders))
    c.JSON(http.StatusOK, orders)
}
func (h *IndividualOrdersHandler) GetOrder(c *gin.Context) {
    id := c.Param("id")
    
    var name, phone, description, status string
    var createdAt time.Time
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT name, phone, description, status, created_at
        FROM individual_orders
        WHERE id = $1
    `, id).Scan(&name, &phone, &description, &status, &createdAt)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Заказ не найден"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "id":           id,
        "client_name":  name,
        "client_phone": phone,
        "requirements": description,
        "status":       status,
        "created_at":   createdAt,
    })
}

func (h *IndividualOrdersHandler) UpdateOrderStatus(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        Status string `json:"status"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE individual_orders SET status = $1 WHERE id = $2
    `, req.Status, id)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *IndividualOrdersHandler) DeleteOrder(c *gin.Context) {
    id := c.Param("id")
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM individual_orders WHERE id = $1
    `, id)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *IndividualOrdersHandler) GetOrderStats(c *gin.Context) {
    var total, pending, approved, inProgress, completed, cancelled int
    
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM individual_orders").Scan(&total)
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM individual_orders WHERE status = 'new'").Scan(&pending)
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM individual_orders WHERE status = 'approved'").Scan(&approved)
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM individual_orders WHERE status = 'in_progress'").Scan(&inProgress)
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM individual_orders WHERE status = 'completed'").Scan(&completed)
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM individual_orders WHERE status = 'cancelled'").Scan(&cancelled)
    
    c.JSON(http.StatusOK, gin.H{
        "total": total,
        "pending": pending,
        "approved": approved,
        "in_progress": inProgress,
        "completed": completed,
        "cancelled": cancelled,
    })
}
// ========== ПОЛУЧИТЬ ВСЕХ ПОЛЬЗОВАТЕЛЕЙ (ВКЛЮЧАЯ КЛИЕНТОВ) ==========
// ========== ПОЛУЧИТЬ ВСЕХ ПОЛЬЗОВАТЕЛЕЙ (ВКЛЮЧАЯ КЛИЕНТОВ) ==========
func GetAllUsersHandler(c *gin.Context) {
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, email, name, role, created_at 
        FROM users 
        ORDER BY created_at DESC
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var users []gin.H
    for rows.Next() {
        var id, email, name, role string
        var createdAt time.Time
        
        err := rows.Scan(&id, &email, &name, &role, &createdAt)
        if err != nil {
            continue
        }
        
        users = append(users, gin.H{
            "id":         id,
            "email":      email,
            "name":       name,
            "role":       role,
            "created_at": createdAt,
        })
    }
    
    c.JSON(http.StatusOK, users)
}