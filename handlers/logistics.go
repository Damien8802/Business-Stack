package handlers

import (
    "net/http"
    "strconv"
    "time"
    "github.com/gin-gonic/gin"
    "subscription-system/models"
    "subscription-system/services"
)

var logisticsService = services.NewLogisticsService()

// Страницы
func LogisticsDashboardHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "logistics_dashboard.html", gin.H{
        "title": "Логистика | Business Stack",
        "page":  "dashboard",
    })
}

func LogisticsOrdersHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "logistics_orders.html", gin.H{
        "title": "Заказы | Логистика",
        "page":  "orders",
    })
}

func LogisticsHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "logistics_dashboard.html", gin.H{
        "title": "Логистика | Business Stack",
    })
}

func DeliveryHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "delivery.html", gin.H{
        "title": "Доставка | Business Stack",
    })
}

func TrackHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "logistics_track.html", gin.H{
        "title": "Отслеживание | Логистика",
        "page":  "track",
    })
}

// API
func APICreateOrder(c *gin.Context) {
    var order models.LogisticsOrder
    if err := c.ShouldBindJSON(&order); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    result, err := logisticsService.CreateOrder(c.Request.Context(), &order)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, result)
}

func APIGetOrders(c *gin.Context) {
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
    offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
    
    orders, total, err := logisticsService.GetOrders(c.Request.Context(), limit, offset)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{
        "orders": orders,
        "total":  total,
        "limit":  limit,
        "offset": offset,
    })
}

func APIUpdateOrderStatus(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        Status string `json:"status"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    if err := logisticsService.UpdateStatus(c.Request.Context(), id, req.Status); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func APIGetStats(c *gin.Context) {
    stats, err := logisticsService.GetStats(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, stats)
}

func TrackAPIHandler(c *gin.Context) {
    trackingNumber := c.Param("trackingNumber")
    c.JSON(http.StatusOK, gin.H{
        "tracking_number":    trackingNumber,
        "status":             "in_transit",
        "current_location":   "Склад Москва",
        "estimated_delivery": time.Now().Add(48 * time.Hour).Format("2006-01-02"),
    })
}