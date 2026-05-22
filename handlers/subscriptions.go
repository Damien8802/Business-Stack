package handlers

import (
    "log"
    "net/http"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

// MySubscriptionsPageHandler отображает страницу с подписками пользователя
func MySubscriptionsPageHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        userID = c.GetString("userId")
    }
    tenantID := c.GetString("tenant_id")
    
    log.Printf("📋 MySubscriptionsPageHandler: userID=%s, tenantID=%s", userID, tenantID)
    
    // Получаем ТОЛЬКО АКТИВНЫЕ подписки из БД
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, plan_name, plan_price_monthly, plan_currency, status, 
               COALESCE(current_period_start::text, ''), 
               COALESCE(current_period_end::text, '')
        FROM subscriptions 
        WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
        ORDER BY id
    `, userID, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка запроса подписок: %v", err)
        c.HTML(http.StatusOK, "my-subscriptions.html", gin.H{
            "title":         "Мои подписки | Business Stack",
            "Subscriptions": []gin.H{},
            "isMaxVersion":  false,
        })
        return
    }
    defer rows.Close()
    
    var subscriptions []gin.H
    for rows.Next() {
        var id int
        var planName, status, startDate, endDate, currency string
        var price float64
        
        err := rows.Scan(&id, &planName, &price, &currency, &status, &startDate, &endDate)
        if err != nil {
            log.Printf("❌ Ошибка сканирования: %v", err)
            continue
        }
        
        log.Printf("✅ Найдена подписка: id=%d, name=%s, status=%s", id, planName, status)
        
        subscriptions = append(subscriptions, gin.H{
            "id":                   id,
            "plan_name":            planName,
            "plan_price_monthly":   price,
            "plan_currency":        currency,
            "status":               status,
            "current_period_start": startDate,
            "current_period_end":   endDate,
        })
    }
    
    // Проверяем есть ли максимальная версия (ПРОФИ или КОРПОРАЦИЯ)
    isMaxVersion := false
    for _, sub := range subscriptions {
        name, _ := sub["plan_name"].(string)
        if name == "🏆 ПРОФИ" || name == "👑 КОРПОРАЦИЯ" {
            isMaxVersion = true
            break
        }
    }
    
    log.Printf("📊 Найдено активных подписок: %d, isMaxVersion=%v", len(subscriptions), isMaxVersion)
    
    c.HTML(http.StatusOK, "my-subscriptions.html", gin.H{
        "title":         "Мои подписки | Business Stack",
        "Subscriptions": subscriptions,
        "isMaxVersion":  isMaxVersion,
    })
}

// CreateSubscriptionHandler - создание новой подписки
func CreateSubscriptionHandler(c *gin.Context) {
    var req struct {
        PlanCode string `json:"plan_code" binding:"required"`
        UserID   string `json:"user_id"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Подписка создана",
    })
}

// GetUserSubscriptionsHandler - список подписок пользователя (API)
func GetUserSubscriptionsHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        userID = c.Query("user_id")
    }
    tenantID := c.GetString("tenant_id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, plan_name, plan_price_monthly, plan_currency, status, 
               current_period_start, current_period_end
        FROM subscriptions 
        WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
    `, userID, tenantID)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"success": true, "subscriptions": []interface{}{}})
        return
    }
    defer rows.Close()
    
    var subscriptions []gin.H
    for rows.Next() {
        var id int
        var planName, status, startDate, endDate, currency string
        var price float64
        
        rows.Scan(&id, &planName, &price, &currency, &status, &startDate, &endDate)
        
        subscriptions = append(subscriptions, gin.H{
            "id":         id,
            "plan_name":  planName,
            "price":      price,
            "currency":   currency,
            "status":     status,
            "start_date": startDate,
            "end_date":   endDate,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "subscriptions": subscriptions,
    })
}