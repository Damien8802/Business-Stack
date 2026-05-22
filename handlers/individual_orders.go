package handlers

import (
    "net/http"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    
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

func (h *IndividualOrdersHandler) CreateOrder(c *gin.Context) {
    var req struct {
        ServiceName    string  `json:"service_name"`
        Requirements   string  `json:"requirements"`
        EstimatedPrice *float64 `json:"estimated_price"`
        ClientName     string  `json:"client_name"`
        ClientPhone    string  `json:"client_phone"`
        ClientEmail    string  `json:"client_email"`
        ClientTelegram string  `json:"client_telegram"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    if req.ServiceName == "" || req.Requirements == "" || req.ClientName == "" || req.ClientPhone == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Все обязательные поля должны быть заполнены"})
        return
    }
    
    // Отправляем уведомление
    go h.telegramBot.SendOrderNotification(nil)
    
    c.JSON(http.StatusCreated, gin.H{
        "message":  "✅ Заявка отправлена! Мы свяжемся с вами в ближайшее время.",
        "order_id": uuid.New().String(),
    })
}

func (h *IndividualOrdersHandler) GetOrders(c *gin.Context) {
    c.JSON(http.StatusOK, []gin.H{})
}

func (h *IndividualOrdersHandler) GetOrder(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
}

func (h *IndividualOrdersHandler) UpdateOrderStatus(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *IndividualOrdersHandler) DeleteOrder(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *IndividualOrdersHandler) GetOrderStats(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "total": 0, "pending": 0, "approved": 0, "in_progress": 0, "completed": 0, "cancelled": 0,
    })
}
