package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

// ==================== ОСНОВНЫЕ СТРАНИЦЫ ====================
func HomeHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "index.html", gin.H{
        "Title":           "Business Stack - Управление подписками",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}
func DashboardHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "dashboard.html", gin.H{
        "Title":           "Дашборд - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func AdminHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "admin.html", gin.H{
        "Title":           "Админ-панель - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func PaymentHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "payment.html", gin.H{
        "Title":           "Платежи - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func PricingHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "pricing.html", gin.H{
        "Title":           "Тарифы - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

// ==================== АВТОРИЗАЦИЯ (СТРАНИЦЫ) ====================
func LoginPageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "login.html", gin.H{
        "Title":   "Вход - Business Stack",
        "Version": "3.0",
        "Time":    time.Now().Format("2006-01-02 15:04:05"),
    })
}

func RegisterPageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "register.html", gin.H{
        "Title":   "Регистрация - Business Stack",
        "Version": "3.0",
        "Time":    time.Now().Format("2006-01-02 15:04:05"),
    })
}

// ForgotPasswordPageHandler - отображает страницу восстановления пароля
func ForgotPasswordPageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "forgot-password.html", gin.H{
        "Title":   "Восстановление пароля - Business Stack",
        "Version": "3.0",
        "Time":    time.Now().Format("2006-01-02 15:04:05"),
    })
}

// ==================== СИСТЕМНЫЕ СТРАНИЦЫ ====================
func SettingsHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "settings.html", gin.H{
        "Title":           "Настройки - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func UsersHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "users.html", gin.H{
        "Title":           "Пользователи - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func MySubscriptionsHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "my-subscriptions.html", gin.H{
        "Title":           "Мои подписки - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

// ==================== ПАРТНЕРЫ И КОНТАКТЫ ====================
func PartnerHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "partner.html", gin.H{
        "Title":           "Партнерская программа - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func ContactHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "contact.html", gin.H{
        "Title":           "Контакты - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func ReferralHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "referral.html", gin.H{
        "Title":           "Реферальная программа - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

// ==================== БЕЗОПАСНОСТЬ ====================
func SecurityHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "security.html", gin.H{
        "Title":           "Безопасность - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func SecurityHubHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "security-hub.html", gin.H{
        "Title":           "Центр безопасности - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func SecurityPanelHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "security-panel.html", gin.H{
        "Title":           "Панель безопасности - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

// ==================== ДОПОЛНИТЕЛЬНЫЕ СТРАНИЦЫ ====================
func AboutHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "about.html", gin.H{
        "Title":           "О нас - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func InfoHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "info.html", gin.H{
        "Title":           "Информация - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func IntegrationsHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "integrations.html", gin.H{
        "Title":           "Интеграции - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func MonetizationHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "monetization.html", gin.H{
        "Title":           "Монетизация - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func SubscriptionsHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "subscriptions.html", gin.H{
        "Title":           "Подписки - Business Stack",
        "Version":         "3.0",
        "Time":            time.Now().Format("2006-01-02 15:04:05"),
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

// IntegrationsPageHandler - страница интеграций
func IntegrationsPageHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "integrations.html", gin.H{
        "title":            "Integrations - Business Stack ERP",
        "active":           "integrations",
        "IsAuthenticated":  isAuthenticated,
        "UserName":         userName,
        "IsDeveloper":      isDeveloper,
    })
}

// PricingPageHandler отображает страницу тарифов
func PricingPageHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    isAuthenticated := userID != ""
    isDeveloper := false

    if isAuthenticated {
        database.Pool.QueryRow(c.Request.Context(),
            "SELECT is_developer FROM users WHERE id = $1", userID).Scan(&isDeveloper)
    }

    if userName == "" && isAuthenticated {
        userName = "Пользователь"
    }

    c.HTML(http.StatusOK, "pricing.html", gin.H{
        "Title":           "Тарифы - Business Stack",
        "IsAuthenticated": isAuthenticated,
        "UserName":        userName,
        "IsDeveloper":     isDeveloper,
    })
}

func IdentityLoginPageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "identity-login.html", gin.H{
        "title": "Identity Hub | Вход и регистрация",
    })
}


