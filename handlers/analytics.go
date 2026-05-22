package handlers

import (
    
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

type AnalyticsData struct {
    TotalUsers       int     `json:"total_users"`
    TotalSubscriptions int   `json:"total_subscriptions"`
    TotalAPIKeys     int     `json:"total_api_keys"`
    TotalReferrals   int     `json:"total_referrals"`
    Revenue          float64 `json:"revenue"`
    PageLoadTime     string  `json:"page_load_time"`
}

func AnalyticsHandler(c *gin.Context) {
    // // tenantID := middleware.GetTenantIDFromContext(c)
    start := time.Now()
    
    var data AnalyticsData

    // Считаем пользователей
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT COUNT(*) FROM users").Scan(&data.TotalUsers)

    // Считаем активные подписки
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT COUNT(*) FROM user_subscriptions WHERE status = 'active'").Scan(&data.TotalSubscriptions)

    // Считаем API ключи
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT COUNT(*) FROM api_keys WHERE is_active = true").Scan(&data.TotalAPIKeys)

    // Считаем рефералы
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT COUNT(*) FROM referrals WHERE status = 'active'").Scan(&data.TotalReferrals)

    // Считаем доход (примерно)
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT COALESCE(SUM(price_monthly), 0) FROM subscription_plans p JOIN user_subscriptions us ON us.plan_id = p.id WHERE us.status = 'active'").Scan(&data.Revenue)

    data.PageLoadTime = time.Since(start).String()

    c.HTML(http.StatusOK, "analytics.html", gin.H{
        "Title":   "Аналитика - Business Stack",
        "Data":    data,
        "Version": "3.0",
    })
}



