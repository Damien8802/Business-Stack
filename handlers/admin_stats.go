package handlers

import (
    "log"
    "net/http"
    "time" 

    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5" 

    "subscription-system/database"
    "subscription-system/middleware"
)

type StatsResponse struct {
    TotalUsers          int     `json:"total_users"`
    ActiveSubscriptions int     `json:"active_subscriptions"`
    TotalAIRequests     int     `json:"total_ai_requests"`
    TotalAPIKeys        int     `json:"total_api_keys"`
    TotalPayments       int     `json:"total_payments"`
    TotalRevenue        float64 `json:"total_revenue"`
}

func AdminStatsHandler(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userEmail := c.GetString("user_email")
    userRole := c.GetString("role")
    
    // Для владельца показываем все данные
    var stats StatsResponse
    
    // Если владелец - показываем данные по всем тенантам
    if userEmail == "dev@businesstack.ru" || userRole == "owner" {
        // Все пользователи (без tenant_id)
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM users`).Scan(&stats.TotalUsers)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM user_subscriptions WHERE status = 'active'`).Scan(&stats.ActiveSubscriptions)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM ai_requests`).Scan(&stats.TotalAIRequests)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM api_keys`).Scan(&stats.TotalAPIKeys)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM payments`).Scan(&stats.TotalPayments)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COALESCE(SUM(amount), 0) FROM payments`).Scan(&stats.TotalRevenue)
    } else {
        // Обычные пользователи - только по своему тенанту
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM users WHERE tenant_id = $1`, tenantID).Scan(&stats.TotalUsers)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM user_subscriptions WHERE status = 'active' AND tenant_id = $1`, tenantID).Scan(&stats.ActiveSubscriptions)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM ai_requests WHERE tenant_id = $1`, tenantID).Scan(&stats.TotalAIRequests)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM api_keys WHERE tenant_id = $1`, tenantID).Scan(&stats.TotalAPIKeys)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COUNT(*) FROM payments WHERE tenant_id = $1`, tenantID).Scan(&stats.TotalPayments)
        
        database.Pool.QueryRow(c.Request.Context(),
            `SELECT COALESCE(SUM(amount), 0) FROM payments WHERE tenant_id = $1`, tenantID).Scan(&stats.TotalRevenue)
    }
    
    log.Printf("[ADMIN STATS] Для владельца: Users=%d, Revenue=%.2f", stats.TotalUsers, stats.TotalRevenue)
    
    c.JSON(http.StatusOK, stats)
}

func AdminUsersHandler(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userEmail := c.GetString("user_email")
    userRole := c.GetString("role")
    
    var rows pgx.Rows
    var err error
    
    // Владелец видит всех пользователей
    if userEmail == "dev@businesstack.ru" || userRole == "owner" {
        rows, err = database.Pool.Query(c.Request.Context(),
            `SELECT id::text, email, COALESCE(name, '') as name, COALESCE(role, 'user') as role,
                    COALESCE(telegram_id::text, '') as telegram_id,
                    COALESCE(is_developer, false) as is_developer,
                    created_at
             FROM users
             ORDER BY created_at DESC
             LIMIT 50`)
    } else {
        rows, err = database.Pool.Query(c.Request.Context(),
            `SELECT id::text, email, COALESCE(name, '') as name, COALESCE(role, 'user') as role,
                    COALESCE(telegram_id::text, '') as telegram_id,
                    COALESCE(is_developer, false) as is_developer,
                    created_at
             FROM users
             WHERE tenant_id = $1
             ORDER BY created_at DESC
             LIMIT 50`, tenantID)
    }
    
    if err != nil {
        log.Printf("AdminUsersHandler query error: %v", err)
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var users []gin.H
    for rows.Next() {
        var id string          // ← ИСПРАВЛЕНО: был int, стал string
        var email, name, role, telegramID string
        var isDeveloper bool
        var createdAt time.Time
        
        err := rows.Scan(&id, &email, &name, &role, &telegramID, &isDeveloper, &createdAt)
        if err != nil {
            log.Printf("AdminUsersHandler scan error: %v", err)
            continue
        }
        
        users = append(users, gin.H{
            "id":           id,
            "email":        email,
            "name":         name,
            "role":         role,
            "telegram_id":  telegramID,
            "is_developer": isDeveloper,
            "created_at":   createdAt,
        })
    }
    
    c.JSON(200, users)
}
func AdminToggleUserBlockHandler(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.Param("id")
    userEmail := c.GetString("user_email")
    userRole := c.GetString("role")

    var req struct {
        IsActive bool `json:"is_active"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var err error
    // Владелец может блокировать любого
    if userEmail == "dev@businesstack.ru" || userRole == "owner" {
        _, err = database.Pool.Exec(c.Request.Context(),
            `UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2`,
            req.IsActive, userID)
    } else {
        _, err = database.Pool.Exec(c.Request.Context(),
            `UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`,
            req.IsActive, userID, tenantID)
    }
    
    if err != nil {
        log.Printf("AdminToggleUserBlockHandler exec error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}

func AdminBroadcastHandler(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userEmail := c.GetString("user_email")
    userRole := c.GetString("role")

    var req struct {
        Message string `json:"message" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var rows interface{}
    var err error
    
    // Владелец видит все telegram_id
    if userEmail == "dev@businesstack.ru" || userRole == "owner" {
        rows, err = database.Pool.Query(c.Request.Context(),
            `SELECT telegram_id FROM users WHERE telegram_id IS NOT NULL`)
    } else {
        rows, err = database.Pool.Query(c.Request.Context(),
            `SELECT telegram_id FROM users WHERE telegram_id IS NOT NULL AND tenant_id = $1`, tenantID)
    }
    
    if err != nil {
        log.Printf("AdminBroadcastHandler query error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
        return
    }
    defer rows.(interface{ Close() error }).Close()

    var telegramIDs []int64
    // Обработка rows...
    
    c.JSON(http.StatusOK, gin.H{
        "message":    "Broadcast prepared",
        "recipients": telegramIDs,
    })
}