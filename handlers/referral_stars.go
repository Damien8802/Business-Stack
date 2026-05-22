package handlers

import (
    "fmt"
    "net/http"
    "subscription-system/database"
    "github.com/gin-gonic/gin"
)

// ReferralPageHandler - страница реферальной программы
func ReferralPageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "referral.html", gin.H{
        "Title": "Referral - Business Stack",
    })
}

// ProcessReferral - обработка реферального редиректа
func ProcessReferral(c *gin.Context) {
    code := c.Param("code")
    if code == "" {
        code = c.Query("ref")
    }

    if code == "" {
        c.Redirect(http.StatusFound, "/")
        return
    }

    c.SetCookie("ref_code", code, 86400*30, "/", "", false, true)
    c.Redirect(http.StatusFound, "/register")
}

// GetReferralStatsHandler - статистика рефералов
func GetReferralStatsHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        userID = c.Query("user_id")
    }
    tenantID := c.GetString("tenant_id")

    var invited, earned, available int

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM referrals WHERE user_id = $1 AND tenant_id = $2
    `, userID, tenantID).Scan(&invited)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(commission), 0) FROM referrals WHERE user_id = $1 AND tenant_id = $2
    `, userID, tenantID).Scan(&earned)

    available = earned

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "stats": gin.H{
            "invited":   invited,
            "earned":    earned,
            "available": available,
        },
    })
}

// GetReferralFriendsHandler - список приглашенных друзей
func GetReferralFriendsHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        userID = c.Query("user_id")
    }
    tenantID := c.GetString("tenant_id")

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT referred_email, created_at, status, COALESCE(commission, 0)
        FROM referrals
        WHERE user_id = $1 AND tenant_id = $2
        ORDER BY created_at DESC
    `, userID, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"success": true, "friends": []interface{}{}})
        return
    }
    defer rows.Close()

    var friends []gin.H
    for rows.Next() {
        var email, status, createdAt string
        var bonus int

        rows.Scan(&email, &createdAt, &status, &bonus)

        dateStr := ""
        if len(createdAt) >= 10 {
            dateStr = createdAt[:10]
        } else if len(createdAt) > 0 {
            dateStr = createdAt
        }

        friends = append(friends, gin.H{
            "email":  email,
            "date":   dateStr,
            "status": status,
            "bonus":  bonus,
        })
    }

    if friends == nil {
        friends = []gin.H{}
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "friends": friends})
}

// GetReferralLinkHandler - получение реферальной ссылки
func GetReferralLinkHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusOK, gin.H{"success": true, "link": ""})
        return
    }

    link := "http://localhost:8080/ref/" + userID
    c.JSON(http.StatusOK, gin.H{"success": true, "link": link})
}

// GetReferralPayoutsHandler - история выплат
func GetReferralPayoutsHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    tenantID := c.GetString("tenant_id")

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, amount, method, status, created_at
        FROM referral_payouts
        WHERE user_id = $1 AND tenant_id = $2
        ORDER BY created_at DESC
    `, userID, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"success": true, "payouts": []interface{}{}})
        return
    }
    defer rows.Close()

    var payouts []gin.H
    for rows.Next() {
        var id int
        var amount float64
        var method, status, createdAt string

        rows.Scan(&id, &amount, &method, &status, &createdAt)

        dateStr := ""
        if len(createdAt) >= 19 {
            dateStr = createdAt[:19]
        } else if len(createdAt) > 0 {
            dateStr = createdAt
        }

        payouts = append(payouts, gin.H{
            "amount":     amount,
            "method":     method,
            "status":     status,
            "created_at": dateStr,
        })
    }

    if payouts == nil {
        payouts = []gin.H{}
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "payouts": payouts})
}

// RequestReferralPayoutHandler - запрос на вывод средств
func RequestReferralPayoutHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
        return
    }

    var req struct {
        Amount float64 `json:"amount"`
        Method string  `json:"method"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
        return
    }

    if req.Amount < 500 {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Minimum payout amount is 500 RUB"})
        return
    }

    tenantID := c.GetString("tenant_id")

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO referral_payouts (user_id, amount, method, status, tenant_id, created_at)
        VALUES ($1, $2, $3, 'pending', $4, NOW())
    `, userID, req.Amount, req.Method, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Payout request created"})
}

// GetTopReferralsHandler - топ рефералов
func GetTopReferralsHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT u.name, u.email, 
               COUNT(r.id) as invited_count,
               COALESCE(SUM(r.commission), 0) as earned
        FROM users u
        LEFT JOIN referrals r ON r.user_id = u.id AND r.status = 'completed'
        WHERE u.tenant_id = $1
        GROUP BY u.id
        ORDER BY earned DESC
        LIMIT 10
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"success": true, "referrals": []interface{}{}})
        return
    }
    defer rows.Close()

    var referrals []gin.H
    for rows.Next() {
        var name, email string
        var invitedCount, earned int

        rows.Scan(&name, &email, &invitedCount, &earned)

        if name == "" {
            name = email
        }

        referrals = append(referrals, gin.H{
            "name":          name,
            "email":         email,
            "invited_count": invitedCount,
            "earned":        earned,
        })
    }

    if referrals == nil {
        referrals = []gin.H{}
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "referrals": referrals})
}

// CreateReferralProgram - создание реферальной программы
func CreateReferralProgram(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var req struct {
        CommissionPercent int `json:"commission_percent"`
    }

    if err := c.BindJSON(&req); err != nil {
        req.CommissionPercent = 20
    }

    if req.CommissionPercent < 5 {
        req.CommissionPercent = 5
    }
    if req.CommissionPercent > 50 {
        req.CommissionPercent = 50
    }

    tenantID := c.GetString("tenant_id")
    referralLink := "http://localhost:8080/ref/" + userID

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO referral_programs (user_id, commission_percent, referral_link, tenant_id, created_at, updated_at)
        VALUES ($1, $2, $3, $4, NOW(), NOW())
        ON CONFLICT (user_id, tenant_id) DO UPDATE SET 
            commission_percent = EXCLUDED.commission_percent,
            referral_link = EXCLUDED.referral_link,
            updated_at = NOW()
    `, userID, req.CommissionPercent, referralLink, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Referral program created", "commission_percent": req.CommissionPercent})
}

// GetReferralProgram - получение реферальной программы
func GetReferralProgram(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    tenantID := c.GetString("tenant_id")

    var commissionPercent int
    var totalEarned int
    var totalReferred int
    var referralLink string

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(commission_percent, 20), 
               COALESCE(total_earned, 0),
               COALESCE(total_referred, 0),
               COALESCE(referral_link, '')
        FROM referral_programs
        WHERE user_id = $1 AND tenant_id = $2
    `, userID, tenantID).Scan(&commissionPercent, &totalEarned, &totalReferred, &referralLink)

    if err != nil {
        commissionPercent = 20
        totalEarned = 0
        totalReferred = 0
        referralLink = "http://localhost:8080/ref/" + userID
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "program": gin.H{
            "commission_percent": commissionPercent,
            "total_earned":       totalEarned,
            "total_referred":     totalReferred,
            "referral_link":      referralLink,
        },
    })
}

// GetReferralCommissions - получение комиссий
func GetReferralCommissions(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    tenantID := c.GetString("tenant_id")

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, amount, status, created_at, paid_at
        FROM referral_commissions
        WHERE user_id = $1 AND tenant_id = $2
        ORDER BY created_at DESC
        LIMIT 50
    `, userID, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"success": true, "commissions": []interface{}{}})
        return
    }
    defer rows.Close()

    var commissions []gin.H
    for rows.Next() {
        var id string
        var amount int
        var status, createdAt string
        var paidAt *string

        rows.Scan(&id, &amount, &status, &createdAt, &paidAt)

        dateStr := ""
        if len(createdAt) >= 19 {
            dateStr = createdAt[:19]
        } else if len(createdAt) > 0 {
            dateStr = createdAt
        }

        commissions = append(commissions, gin.H{
            "id":         id,
            "amount":     amount,
            "status":     status,
            "created_at": dateStr,
            "paid_at":    paidAt,
        })
    }

    if commissions == nil {
        commissions = []gin.H{}
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "commissions": commissions})
}

// PayCommission - выплата комиссии
func PayCommission(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var req struct {
        CommissionID string `json:"commission_id"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    tenantID := c.GetString("tenant_id")

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE referral_commissions 
        SET status = 'paid', paid_at = NOW()
        WHERE id = $1 AND user_id = $2 AND tenant_id = $3 AND status = 'pending'
    `, req.CommissionID, userID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Commission paid"})
}

// SavePayoutDetailsHandler - сохранение реквизитов для вывода
func SavePayoutDetailsHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
        return
    }

    var req struct {
        Method  string                 `json:"method"`
        Details map[string]interface{} `json:"details"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
        return
    }

    tenantID := c.GetString("tenant_id")

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO user_payout_details (user_id, method, details, tenant_id, created_at)
        VALUES ($1, $2, $3, $4, NOW())
        ON CONFLICT (user_id, tenant_id) DO UPDATE SET
            method = EXCLUDED.method,
            details = EXCLUDED.details,
            updated_at = NOW()
    `, userID, req.Method, req.Details, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Payout details saved"})
}

func formatMoney(amount float64) string {
    return fmt.Sprintf("%.0f RUB", amount)
}