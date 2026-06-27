package handlers

import (
    "net/http"
    "subscription-system/database"
    "subscription-system/models"
    "github.com/gin-gonic/gin"
)

func GetUserProfile(c *gin.Context) {
    // ===== ИСПРАВЛЕНО: используем c.GetString("user_id") =====
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
        return
    }

    // Получаем пользователя из БД через модель
    user, err := models.GetUserByID(userID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
        return
    }

    // Количество AI-запросов
    var aiCount int
    _ = database.Pool.QueryRow(c.Request.Context(),
        "SELECT COUNT(*) FROM ai_requests WHERE user_id = $1", userID).Scan(&aiCount)

    // Получаем активную подписку пользователя
    type subscriptionInfo struct {
        ID                 int     `json:"id"`
        PlanID             int     `json:"plan_id"`
        Status             string  `json:"status"`
        CurrentPeriodStart *string `json:"current_period_start"`
        CurrentPeriodEnd   *string `json:"current_period_end"`
        CancelAtPeriodEnd  bool    `json:"cancel_at_period_end"`
        TrialEnd           *string `json:"trial_end"`
        PaymentMethod      *string `json:"payment_method"`
        AITokensUsed       *int64  `json:"ai_tokens_used"`
        CreatedAt          string  `json:"created_at"`
        UpdatedAt          string  `json:"updated_at"`
    }

    var sub subscriptionInfo
    var hasSubscription bool
    err = database.Pool.QueryRow(c.Request.Context(),
        `SELECT id, plan_id, status, current_period_start, current_period_end, 
                cancel_at_period_end, trial_end, payment_method, ai_tokens_used,
                created_at, updated_at
         FROM subscriptions 
         WHERE user_id = $1 AND status = 'active' 
         ORDER BY created_at DESC LIMIT 1`,
        userID).Scan(
        &sub.ID, &sub.PlanID, &sub.Status,
        &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
        &sub.CancelAtPeriodEnd, &sub.TrialEnd, &sub.PaymentMethod,
        &sub.AITokensUsed, &sub.CreatedAt, &sub.UpdatedAt,
    )
    if err == nil {
        hasSubscription = true
    }

    // Получаем название тарифа, если есть подписка
    var planName *string
    if hasSubscription {
        var name string
        err = database.Pool.QueryRow(c.Request.Context(),
            "SELECT name FROM plans WHERE id = $1", sub.PlanID).Scan(&name)
        if err == nil {
            planName = &name
        }
    }

    // ===== ДОБАВЛЯЕМ ВСЕ ПОЛЯ ПОЛЬЗОВАТЕЛЯ =====
    response := gin.H{
        "user": gin.H{
            "id":                 user.ID,
            "email":              user.Email,
            "name":               user.Name,
            "role":               user.Role,
            "phone":              user.Phone,              // ← ДОБАВЛЕНО
            "organization_name":  user.OrganizationName,   // ← ДОБАВЛЕНО
            "organization_inn":   user.OrganizationInn,    // ← ДОБАВЛЕНО
            "created_at":         user.CreatedAt,
            "updated_at":         user.UpdatedAt,
            "email_verified":     user.EmailVerified,      // ← ДОБАВЛЕНО
            "telegram_id":        user.TelegramID,         // ← ДОБАВЛЕНО
        },
        "ai_requests_count": aiCount,
    }

    if hasSubscription {
        response["subscription"] = gin.H{
            "id":                    sub.ID,
            "plan_id":               sub.PlanID,
            "plan_name":             planName,
            "status":                sub.Status,
            "current_period_start":  sub.CurrentPeriodStart,
            "current_period_end":    sub.CurrentPeriodEnd,
            "cancel_at_period_end":  sub.CancelAtPeriodEnd,
            "trial_end":             sub.TrialEnd,
            "payment_method":        sub.PaymentMethod,
            "ai_tokens_used":        sub.AITokensUsed,
            "created_at":            sub.CreatedAt,
            "updated_at":            sub.UpdatedAt,
        }
    } else {
        response["subscription"] = nil
    }

    c.JSON(http.StatusOK, response)
}