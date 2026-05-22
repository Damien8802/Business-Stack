package handlers

import (
    "net/http"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    
    "subscription-system/database"
)

// PushSubscription структура подписки на push-уведомления
type PushSubscription struct {
    Endpoint string `json:"endpoint"`
    Keys     struct {
        P256dh string `json:"p256dh"`
        Auth   string `json:"auth"`
    } `json:"keys"`
}

// SavePushSubscription - сохранить подписку на push-уведомления
func SavePushSubscription(c *gin.Context) {
    userID := getUserID(c)
    
    var sub PushSubscription
    if err := c.BindJSON(&sub); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Сохраняем в базу
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, created_at)
        VALUES ($1, $2, $3, $4, NOW())
        ON CONFLICT (user_id, endpoint) DO UPDATE SET
            p256dh = EXCLUDED.p256dh,
            auth = EXCLUDED.auth,
            updated_at = NOW()
    `, userID, sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save subscription"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Push subscription saved",
    })
}

// GetPushSubscriptions - получить подписки пользователя
func GetPushSubscriptions(c *gin.Context) {
    userID := getUserID(c)
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, endpoint, p256dh, auth, created_at
        FROM push_subscriptions
        WHERE user_id = $1
    `, userID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var subscriptions []PushSubscription
    for rows.Next() {
        var id uuid.UUID
        var sub PushSubscription
        var createdAt string
        rows.Scan(&id, &sub.Endpoint, &sub.Keys.P256dh, &sub.Keys.Auth, &createdAt)
        subscriptions = append(subscriptions, sub)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "subscriptions": subscriptions,
    })
}

// GetPWAInfo - информация о PWA
func GetPWAInfo(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "name":        "Business Stack ERP",
        "short_name":  "Business Stack",
        "version":     "3.6.0",
        "description": "Управление складом, финансами и закупками",
        "installable": true,
        "offline":     true,
        "push":        true,
    })
}