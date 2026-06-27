package handlers

import (
    "fmt"
    "log"    
    "net/http"
    "time"
    
    "subscription-system/database"
    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5"  // ← ИСПОЛЬЗУЕМ pgx!
)

// AdminGetPayouts - получить ВСЕ заявки на вывод (для владельца)
func AdminGetPayouts(c *gin.Context) {
    userEmail := c.GetString("user_email")
    
    log.Printf("👑 AdminGetPayouts: Владелец %s просматривает ВСЕ заявки", userEmail)
    
    // ✅ ЯВНО УКАЗЫВАЕМ ТИП pgx.Rows
    var rows pgx.Rows
    var err error
    
    rows, err = database.Pool.Query(c.Request.Context(), `
        SELECT 
            p.id, 
            p.user_id, 
            p.amount, 
            p.method, 
            p.status, 
            p.created_at, 
            COALESCE(u.email, '') as user_email,
            COALESCE(u.name, '') as user_name
        FROM referral_payouts p
        LEFT JOIN users u ON p.user_id = u.id
        ORDER BY p.created_at DESC
    `)
    
    if err != nil {
        log.Printf("❌ AdminGetPayouts ошибка: %v", err)
        c.JSON(http.StatusOK, gin.H{"payouts": []interface{}{}})
        return
    }
    defer rows.Close()
    
    var payouts []gin.H
    for rows.Next() {
        var id int
        var userID, method, status, userEmail2, userName string
        var amount float64
        var createdAt time.Time
        
        // ✅ ИСПОЛЬЗУЕМ pgx.Scan
        err := rows.Scan(&id, &userID, &amount, &method, &status, &createdAt, &userEmail2, &userName)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
            continue
        }
        
        payouts = append(payouts, gin.H{
            "id":          id,
            "user_id":     userID,
            "user_email":  userEmail2,
            "user_name":   userName,
            "amount":      amount,
            "method":      method,
            "status":      status,
            "created_at":  createdAt.Format("2006-01-02 15:04:05"),
        })
    }
    
    if payouts == nil {
        payouts = []gin.H{}
    }
    
    log.Printf("📊 AdminGetPayouts: Найдено %d заявок (ВСЕХ пользователей)", len(payouts))
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "payouts": payouts,
        "count":   len(payouts),
    })
}

// AdminUpdatePayoutStatus - обновить статус выплаты
func AdminUpdatePayoutStatus(c *gin.Context) {
    var req struct {
        ID     int    `json:"id"`
        Status string `json:"status"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    var userID string
    var amount float64
    var userEmail string
    
    // ✅ ИСПОЛЬЗУЕМ pgx.QueryRow
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT p.user_id, p.amount, COALESCE(u.email, '')
        FROM referral_payouts p
        LEFT JOIN users u ON p.user_id = u.id
        WHERE p.id = $1
    `, req.ID).Scan(&userID, &amount, &userEmail)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Заявка не найдена"})
        return
    }
    
    // ✅ ИСПОЛЬЗУЕМ pgx.Exec
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE referral_payouts 
        SET status = $1, updated_at = NOW()
        WHERE id = $2
    `, req.Status, req.ID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    if req.Status == "completed" {
        _, _ = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO notifications (user_id, title, message, type, is_read, created_at)
            VALUES ($1, $2, $3, 'payout', false, NOW())
        `, userID, "✅ Выплата выполнена", fmt.Sprintf("Сумма %.0f ₽ успешно переведена", amount))
        
        log.Printf("📧 Уведомление отправлено пользователю %s: Выплата %.0f ₽ выполнена", userEmail, amount)
    }
    
    if req.Status == "rejected" {
        _, _ = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO notifications (user_id, title, message, type, is_read, created_at)
            VALUES ($1, $2, $3, 'payout', false, NOW())
        `, userID, "❌ Выплата отклонена", fmt.Sprintf("Заявка на сумму %.0f ₽ отклонена", amount))
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// AdminDeletePayout - удаление заявки на вывод
func AdminDeletePayout(c *gin.Context) {
    id := c.Param("id")
    
    var exists bool
    
    // ✅ ИСПОЛЬЗУЕМ pgx.QueryRow
    err := database.Pool.QueryRow(c.Request.Context(), 
        "SELECT EXISTS(SELECT 1 FROM referral_payouts WHERE id = $1)", id).Scan(&exists)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки заявки"})
        return
    }
    
    if !exists {
        c.JSON(http.StatusNotFound, gin.H{"error": "Заявка не найдена"})
        return
    }
    
    // ✅ ИСПОЛЬЗУЕМ pgx.Exec
    _, err = database.Pool.Exec(c.Request.Context(), 
        "DELETE FROM referral_payouts WHERE id = $1", id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления: " + err.Error()})
        return
    }
    
    log.Printf("🗑️ Заявка на вывод #%s удалена", id)
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Заявка удалена",
    })
}