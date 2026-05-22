package middleware

import (
    "context"
    "log"  
    "fmt"
    "net/http"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

// Список email'ов владельцев системы (безлимитный доступ)
var ownerEmails = map[string]bool{
    "dev@businessstack.ru":     true,
    "Skorpion_88-88@mail.ru": true,
}

// isOwner - проверяет, является ли пользователь владельцем системы
func isOwner(email string) bool {
    return ownerEmails[email]
}

// ========== МОДУЛИ (VPN, CRM) ==========

// RequireModuleAccess - проверяет доступ к модулю
func RequireModuleAccess(moduleName string) gin.HandlerFunc {
    return func(c *gin.Context) {
        role := c.GetString("role")
        email := c.GetString("user_email")

        // ВЛАДЕЛЕЦ СИСТЕМЫ - БЕЗЛИМИТНЫЙ ДОСТУП
        if isOwner(email) {
            c.Next()
            return
        }

        // РАЗРАБОТЧИК И АДМИН - БЕЗЛИМИТНЫЙ ДОСТУП
        if role == "developer" || role == "admin" {
            c.Next()
            return
        }

        userID := c.GetString("user_id")
        tenantID := c.GetString("tenant_id")

        if tenantID == "" {
            c.JSON(http.StatusForbidden, gin.H{
                "error":   "Доступ запрещён",
                "message": "Tenant ID не найден",
            })
            c.Abort()
            return
        }

        // 1. Проверяем активную подписку
        var expiresAt time.Time
        var subStatus string
        err := database.Pool.QueryRow(context.Background(), `
            SELECT status, expires_at FROM user_subscriptions 
            WHERE user_id = $1 AND module_name = $2 AND status = 'active'
            ORDER BY expires_at DESC LIMIT 1
        `, userID, moduleName).Scan(&subStatus, &expiresAt)

        if err == nil && expiresAt.After(time.Now()) {
            c.Next()
            return
        }

        // 1.5 ПРОВЕРКА РОДИТЕЛЬСКОГО МОДУЛЯ FinCore
        if parentModule, ok := fincoreSubModules[moduleName]; ok {
            var parentExpiresAt time.Time
            var parentStatus string
            err := database.Pool.QueryRow(context.Background(), `
                SELECT status, expires_at FROM user_subscriptions 
                WHERE user_id = $1 AND module_name = $2 AND status = 'active' AND expires_at > NOW()
            `, userID, parentModule).Scan(&parentStatus, &parentExpiresAt)
            
            if err == nil && parentExpiresAt.After(time.Now()) {
                c.Next()
                return
            }
            
            var parentTrialEnd time.Time
            err = database.Pool.QueryRow(context.Background(), `
                SELECT trial_end FROM user_trials 
                WHERE user_id = $1 AND module_name = $2 AND trial_end > NOW()
            `, userID, parentModule).Scan(&parentTrialEnd)
            
            if err == nil && parentTrialEnd.After(time.Now()) {
                c.Next()
                return
            }
        }

        // 2. Проверяем триальный период
        var trialEnd time.Time
        err = database.Pool.QueryRow(context.Background(), `
            SELECT trial_end FROM user_trials 
            WHERE user_id = $1 AND module_name = $2 AND trial_end > NOW()
        `, userID, moduleName).Scan(&trialEnd)

        if err == nil && trialEnd.After(time.Now()) {
            daysLeft := int(time.Until(trialEnd).Hours() / 24)
            if daysLeft == 3 || daysLeft == 1 {
                var notified bool
                database.Pool.QueryRow(context.Background(), `
                    SELECT notified FROM user_trials 
                    WHERE user_id = $1 AND module_name = $2
                `, userID, moduleName).Scan(&notified)

                if !notified {
                    fmt.Printf("🔔 Уведомление: У пользователя %s заканчивается триал модуля %s через %d дней\n", userID, moduleName, daysLeft)
                    database.Pool.Exec(context.Background(), `
                        UPDATE user_trials SET notified = true
                        WHERE user_id = $1 AND module_name = $2
                    `, userID, moduleName)
                }
            }
            c.Next()
            return
        }

        // 3. Нет доступа - проверяем тип запроса
        if strings.HasPrefix(c.Request.URL.Path, "/api/") || 
           c.GetHeader("X-Requested-With") == "XMLHttpRequest" || 
           c.GetHeader("Accept") == "application/json" {
            // API запрос - возвращаем JSON
            c.JSON(http.StatusPaymentRequired, gin.H{
                "error":            "Модуль не оплачен",
                "message":          "Для доступа к этому модулю необходимо оплатить подписку или начать пробный период",
                "module":           moduleName,
                "trial_available":  true,
                "trial_days":       GetModuleTrialDays(moduleName),
                "upgrade_url":      "/pricing",
                "start_trial_url":  "/api/trial/start?module=" + moduleName,
            })
        } else {
            // Обычный переход по ссылке - показываем красивую HTML страницу
            displayName := GetModuleDisplayName(moduleName)
            c.HTML(http.StatusPaymentRequired, "module_locked.html", gin.H{
                "title":        fmt.Sprintf("🔒 %s — требуется подписка", displayName),
                "message":      fmt.Sprintf("Для доступа к модулю «%s» необходимо оформить подписку", displayName),
                "module_name":  displayName,
                "module_code":  moduleName,
                "price":        GetModulePrice(moduleName),
                "trial_days":   GetModuleTrialDays(moduleName),
                "icon":         GetModuleIcon(moduleName),
            })
        }
        c.Abort()
    }
}

// StartModuleTrial - начать триальный период для пользователя (с учётом дней для конкретного модуля)
func StartModuleTrial(userID, moduleName string) error {
    trialDays := GetModuleTrialDays(moduleName)
    log.Printf("🔍 StartModuleTrial called: userID=%s, moduleName=%s, days=%d", userID, moduleName, trialDays)
    
    // Если модуль является подмодулем FinCore, активируем и родительский
    if parentModule, ok := fincoreSubModules[moduleName]; ok {
        parentDays := GetModuleTrialDays(parentModule)
        _, err := database.Pool.Exec(context.Background(), `
            INSERT INTO user_trials (user_id, module_name, trial_start, trial_end, notified)
            VALUES ($1, $2, NOW(), NOW() + INTERVAL '1 day' * $3, false)
            ON CONFLICT (user_id, module_name) DO UPDATE SET
                trial_start = NOW(),
                trial_end = NOW() + INTERVAL '1 day' * $3,
                notified = false
        `, userID, parentModule, parentDays)
        if err != nil {
            log.Printf("⚠️ Ошибка активации родительского модуля %s: %v", parentModule, err)
        }
    }
    
    _, err := database.Pool.Exec(context.Background(), `
        INSERT INTO user_trials (user_id, module_name, trial_start, trial_end, notified)
        VALUES ($1, $2, NOW(), NOW() + INTERVAL '1 day' * $3, false)
        ON CONFLICT (user_id, module_name) DO UPDATE SET
            trial_start = NOW(),
            trial_end = NOW() + INTERVAL '1 day' * $3,
            notified = false
    `, userID, moduleName, trialDays)
    
    if err != nil {
        log.Printf("❌ Exec error: %v", err)
        return err
    }
    
    log.Printf("✅ Trial activated successfully for user %s, module %s, %d days", userID, moduleName, trialDays)
    return nil
}

// GetAvailableModules - возвращает список доступных модулей
func GetAvailableModules(tenantID string) []string {
    if tenantID == "" {
        return []string{}
    }

    rows, err := database.Pool.Query(context.Background(), `
        SELECT module_name FROM user_subscriptions
        WHERE tenant_id = $1 AND status = 'active'
    `, tenantID)
    if err != nil {
        return []string{}
    }
    defer rows.Close()

    var modules []string
    for rows.Next() {
        var module string
        rows.Scan(&module)
        modules = append(modules, module)
    }
    return modules
}

// ========== РАЗРАБОТЧИКИ (DEVELOPER PORTAL) ==========

// RequireDeveloperAccess - проверяет доступ к Developer Portal
func RequireDeveloperAccess() gin.HandlerFunc {
    return func(c *gin.Context) {
        role := c.GetString("role")
        userID := c.GetString("user_id")
        email := c.GetString("user_email")

        if isOwner(email) {
            c.Next()
            return
        }

        if role == "admin" {
            c.Next()
            return
        }

        if role == "developer" {
            c.Next()
            return
        }

        var plan string
        var trialEnd time.Time
        var subscriptionEnd time.Time
        var status string

        err := database.Pool.QueryRow(context.Background(), `
            SELECT plan, trial_end, subscription_end, status
            FROM developer_subscriptions
            WHERE user_id = $1
        `, userID).Scan(&plan, &trialEnd, &subscriptionEnd, &status)

        if err == nil && status == "active" {
            if plan == "trial" && trialEnd.After(time.Now()) {
                c.Next()
                return
            }
            if (plan == "pro" || plan == "enterprise") && subscriptionEnd.After(time.Now()) {
                c.Next()
                return
            }
        }

        c.JSON(http.StatusPaymentRequired, gin.H{
            "error":       "Доступ к Developer Portal требует подписки",
            "message":     "Станьте разработчиком, чтобы создавать OAuth-приложения",
            "trial_days":  14,
            "plans": []gin.H{
                {"name": "Пробный", "price": 0, "days": 14, "apps": 1, "users": 100},
                {"name": "Pro", "price": 2990, "apps": 10, "users": 10000},
                {"name": "Enterprise", "price": 14990, "apps": -1, "users": -1},
            },
            "upgrade_url": "/developer/pricing",
        })
        c.Abort()
    }
}

// StartDeveloperTrial - начать триальный период для разработчика
func StartDeveloperTrial(userID string) error {
    var existingEnd time.Time
    err := database.Pool.QueryRow(context.Background(), `
        SELECT trial_end FROM developer_trials 
        WHERE user_id = $1 AND trial_end > NOW()
    `, userID).Scan(&existingEnd)
    
    if err == nil {
        _, _ = database.Pool.Exec(context.Background(), `
            UPDATE users SET role = 'developer' WHERE id = $1 AND role = 'user'
        `, userID)
        return nil
    }
    
    trialEnd := time.Now().Add(14 * 24 * time.Hour)
    _, err = database.Pool.Exec(context.Background(), `
        INSERT INTO developer_trials (user_id, trial_end, created_at)
        VALUES ($1, $2, NOW())
        ON CONFLICT (user_id) DO UPDATE SET trial_end = $2
    `, userID, trialEnd)
    
    if err != nil {
        return err
    }
    
    _, err = database.Pool.Exec(context.Background(), `
        UPDATE users SET role = 'developer' WHERE id = $1 AND role = 'user'
    `, userID)
    
    return err
}

// GetDeveloperSubscription - получить информацию о подписке разработчика
func GetDeveloperSubscription(userID string) (map[string]interface{}, error) {
    var plan string
    var trialEnd, subscriptionEnd time.Time
    var maxApps, maxUsers int
    var status string

    err := database.Pool.QueryRow(context.Background(), `
        SELECT plan, trial_end, subscription_end, max_apps, max_users, status
        FROM developer_subscriptions
        WHERE user_id = $1
    `, userID).Scan(&plan, &trialEnd, &subscriptionEnd, &maxApps, &maxUsers, &status)

    if err != nil {
        return nil, err
    }

    return map[string]interface{}{
        "plan":                       plan,
        "trial_end":                  trialEnd,
        "subscription_end":           subscriptionEnd,
        "max_apps":                   maxApps,
        "max_users":                  maxUsers,
        "status":                     status,
        "is_trial_active":            plan == "trial" && trialEnd.After(time.Now()),
        "is_subscription_active":     (plan == "pro" || plan == "enterprise") && subscriptionEnd.After(time.Now()),
    }, nil
}

// ========== НОВЫЕ МОДУЛИ С РАЗНЫМИ ТРИАЛ-ПЕРИОДАМИ ==========

var moduleTrialDays = map[string]int{
    "fincore":           14,
    "finance":           14,
    "reports-analytics": 14,
    "reconciliation":    14,
    "journal":           14,
    "bank-client":       14,
    "payroll":           14,
    "tax-reporting":     14,
    "import-excel":      14,
    "month-closing":     14,
    "crm":               14,
    "hr":                7,
    "vpn":               3,
    "cloud":             3,
    "archive":           3,
    "marketplace":       7,
    "inventory":         7,
    "teamsphere":        14,
    "logistics":         7,
    "analytics":         14,
    "migration":         7,
    "security":          14,
    "referral":          14,
    "profile":           14,
    "fusion-api":        3,
    "whatsapp":          7,
    "backup":            3,
    "projects":          7,
    "ai-agents":         7,
    "advanced-analytics": 14,
}

var fincoreSubModules = map[string]string{
    "finance":           "fincore",
    "reports-analytics": "fincore",
    "reconciliation":    "fincore",
    "journal":           "fincore",
    "bank-client":       "fincore",
    "payroll":           "fincore",
    "tax-reporting":     "fincore",
    "import-excel":      "fincore",
    "month-closing":     "fincore",
}

var moduleDisplayNames = map[string]string{
    "fincore":           "💰 FinCore",
    "finance":           "📊 Финансовый учёт",
    "reports-analytics": "📈 Отчёты и аналитика",
    "reconciliation":    "📋 Акты сверки",
    "journal":           "📝 Журнал проводок",
    "bank-client":       "🏦 Банк-клиент",
    "payroll":           "💳 Расчёт зарплаты",
    "tax-reporting":     "📑 Налоговая отчётность",
    "import-excel":      "📎 Импорт Excel",
    "month-closing":     "🔒 Закрытие месяца",
    "crm":               "🤝 CRM",
    "hr":                "👥 HR модуль",
    "vpn":               "🔒 VPN",
    "cloud":             "☁️ Cloud Storage",
    "archive":           "🗄️ Архив",
    "marketplace":       "🛒 Маркетплейс",
    "inventory":         "📦 Складской учёт",
    "teamsphere":        "👥 TeamSphere",
    "logistics":         "🚚 Логистика",
    "analytics":         "📊 Аналитика",
    "migration":         "🔄 Миграция данных (3 фазы)",
    "security":          "🛡️ Безопасность",
    "referral":          "👥 Рефералы",
    "profile":           "👤 Профиль",
    "fusion-api":        "⚡ FusionAPI",
    "whatsapp":          "💬 WhatsApp",
    "backup":            "💾 Бэкапы",
    "projects":          "📁 Проекты",
    "ai-agents":         "🤖 AI Агенты",
    "advanced-analytics": "🎯 Продвинутая аналитика",
}

var modulePrices = map[string]int{
    "fincore":           19900,
    "finance":           0,
    "reports-analytics": 0,
    "reconciliation":    0,
    "journal":           0,
    "bank-client":       0,
    "payroll":           0,
    "tax-reporting":     0,
    "import-excel":      0,
    "month-closing":     0,
    "crm":               14900,
    "hr":                4900,
    "vpn":               490,
    "cloud":             150,
    "inventory":         5900,
    "teamsphere":        9900,
    "logistics":         3900,
    "whatsapp":          2900,
    "analytics":         7900,
    "security":          5900,
    "migration":         4900,
    "archive":           990,
    "projects":          4900,
    "ai-agents":         9900,
    "advanced-analytics": 14900,
}

func GetModuleTrialDays(moduleName string) int {
    if days, ok := moduleTrialDays[moduleName]; ok {
        return days
    }
    return 14
}

func GetModuleDisplayName(moduleName string) string {
    if name, ok := moduleDisplayNames[moduleName]; ok {
        return name
    }
    return moduleName
}

func GetModulePrice(moduleName string) int {
    if price, ok := modulePrices[moduleName]; ok {
        return price
    }
    return 0
}

func GetModuleIcon(moduleName string) string {
    return getModuleIcon(moduleName)
}

func CheckFincoreAccess(userID, moduleName string) bool {
    if parentModule, ok := fincoreSubModules[moduleName]; ok {
        return CheckModuleAccessWithTrial(userID, parentModule)
    }
    return CheckModuleAccessWithTrial(userID, moduleName)
}

func CheckModuleAccessWithTrial(userID, moduleName string) bool {
    var expiresAt time.Time
    var subStatus string
    err := database.Pool.QueryRow(context.Background(), `
        SELECT status, expires_at FROM user_subscriptions 
        WHERE user_id = $1 AND module_name = $2 AND status = 'active' AND expires_at > NOW()
        ORDER BY expires_at DESC LIMIT 1
    `, userID, moduleName).Scan(&subStatus, &expiresAt)

    if err == nil && expiresAt.After(time.Now()) {
        return true
    }

    var trialEnd time.Time
    err = database.Pool.QueryRow(context.Background(), `
        SELECT trial_end FROM user_trials 
        WHERE user_id = $1 AND module_name = $2 AND trial_end > NOW()
    `, userID, moduleName).Scan(&trialEnd)

    return err == nil && trialEnd.After(time.Now())
}

func StartModuleTrialWithDays(userID, moduleName string, days int) error {
    log.Printf("🔍 StartModuleTrialWithDays: userID=%s, moduleName=%s, days=%d", userID, moduleName, days)
    
    _, err := database.Pool.Exec(context.Background(), `
        INSERT INTO user_trials (user_id, module_name, trial_start, trial_end, notified)
        VALUES ($1, $2, NOW(), NOW() + INTERVAL '1 day' * $3, false)
        ON CONFLICT (user_id, module_name) DO UPDATE SET
            trial_start = NOW(),
            trial_end = NOW() + INTERVAL '1 day' * $3,
            notified = false
    `, userID, moduleName, days)
    
    if err != nil {
        log.Printf("❌ Exec error: %v", err)
        return err
    }
    
    log.Printf("✅ Trial activated for user %s, module %s, %d days", userID, moduleName, days)
    return nil
}

func StartFincoreTrial(userID string) error {
    if err := StartModuleTrialWithDays(userID, "fincore", 14); err != nil {
        return err
    }
    
    for subModule := range fincoreSubModules {
        if err := StartModuleTrialWithDays(userID, subModule, 14); err != nil {
            log.Printf("⚠️ Не удалось активировать триал для %s: %v", subModule, err)
        }
    }
    
    log.Printf("✅ FinCore trial activated for user %s with all submodules", userID)
    return nil
}