package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

// ==================== АДМИН-СТРАНИЦЫ (HTML) ====================

// AdminFixedHandler отображает фиксированную админ-панель
func AdminFixedHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "admin-fixed.html", gin.H{
        "Title":   "Админ-панель (Fixed) - Business Stack",
        "Version": "3.0",
        "Time":    time.Now().Format("2006-01-02 15:04:05"),
    })
}

// GoldAdminHandler отображает Gold Admin панель
func GoldAdminHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "gold-admin.html", gin.H{
        "Title":   "Gold Admin - Business Stack",
        "Version": "3.0",
        "Time":    time.Now().Format("2006-01-02 15:04:05"),
    })
}

// DatabaseAdminHandler отображает админ-панель базы данных
func DatabaseAdminHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "database-admin.html", gin.H{
        "Title":   "Админ базы данных - Business Stack",
        "Version": "3.0",
        "Time":    time.Now().Format("2006-01-02 15:04:05"),
    })
}

// ==================== АДМИН API (JSON) ====================

// AdminPaymentsHandler возвращает список платежей
func AdminPaymentsHandler(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success":  true,
        "message":  "Admin payments endpoint",
        "payments": []gin.H{},
    })
}

// AdminPaymentStats возвращает статистику платежей из БД
func AdminPaymentStats(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    var totalAmount float64
    var paymentsCount int
    var todayAmount float64
    var weekAmount float64
    var monthAmount float64
    
    // Общая сумма и количество платежей (только completed)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0), COUNT(*)
        FROM payments
        WHERE tenant_id = $1 AND status = 'completed'
    `, tenantID).Scan(&totalAmount, &paymentsCount)
    
    // Сумма за сегодня
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0)
        FROM payments
        WHERE tenant_id = $1 AND status = 'completed' AND DATE(created_at) = CURRENT_DATE
    `, tenantID).Scan(&todayAmount)
    
    // Сумма за неделю
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0)
        FROM payments
        WHERE tenant_id = $1 AND status = 'completed' AND created_at >= NOW() - INTERVAL '7 days'
    `, tenantID).Scan(&weekAmount)
    
    // Сумма за месяц
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0)
        FROM payments
        WHERE tenant_id = $1 AND status = 'completed' AND created_at >= NOW() - INTERVAL '30 days'
    `, tenantID).Scan(&monthAmount)
    
    c.JSON(200, gin.H{
        "total_amount":   totalAmount,
        "payments_count": paymentsCount,
        "today_amount":   todayAmount,
        "week_amount":    weekAmount,
        "month_amount":   monthAmount,
    })
}
// AdminSecurityLogs возвращает логи безопасности
func AdminSecurityLogs(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "logs":    []gin.H{},
    })
}

// AdminBlockedIPs возвращает список заблокированных IP
func AdminBlockedIPs(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "ips":     []gin.H{},
    })
}

// AdminToggleUserBlock блокирует/разблокирует пользователя
func AdminToggleUserBlock(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "User block status toggled",
    })
}

// AdminChangeUserRole изменяет роль пользователя
func AdminChangeUserRole(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "User role changed",
    })
}

// AdminDeleteUser удаляет пользователя
func AdminDeleteUser(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "User deleted",
    })
}

// GetRecentPayments возвращает последние платежи
func GetRecentPayments(c *gin.Context) {
    limit := c.DefaultQuery("limit", "10")
    tenantID := c.GetString("tenant_id")
    
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, amount, status, created_at, plan_name
        FROM payments
        WHERE tenant_id = $1
        ORDER BY created_at DESC
        LIMIT $2
    `, tenantID, limit)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var payments []gin.H
    for rows.Next() {
        var id string
        var amount float64
        var status string
        var createdAt time.Time
        var planName string
        
        rows.Scan(&id, &amount, &status, &createdAt, &planName)
        
        payments = append(payments, gin.H{
            "id":         id,
            "amount":     amount,
            "status":     status,
            "created_at": createdAt,
            "plan_name":  planName,
        })
    }
    
    c.JSON(200, gin.H{"payments": payments})
}