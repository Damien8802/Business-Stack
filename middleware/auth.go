package middleware

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "strings"
    "subscription-system/config"
    "subscription-system/database"
    "subscription-system/utils"

    "github.com/gin-gonic/gin"
)
// GetTenantByUserID - получает tenant_id из БД по user_id
func GetTenantByUserID(userID string) (string, error) {
    var tenantID string
    err := database.Pool.QueryRow(context.Background(), `
        SELECT tenant_id FROM users WHERE id = $1
    `, userID).Scan(&tenantID)
    if err != nil {
        return "", err
    }
    return tenantID, nil
}

// ========== ТВОИ ЛИЧНЫЕ ПОМОЩНИКИ (ДОСТУП К ТВОЕЙ ПЛАТФОРМЕ) ==========
// Сюда добавляешь email тех, кому доверяешь администрировать твою платформу
var platformAdmins = map[string]bool{
    // "admin@example.com": true,   ← добавляй сюда своих админов
    // "helper@businesstack.ru": true,
}

// Твои личные разработчики (кто помогает с кодом)
var platformDevelopers = map[string]bool{
    // "dev@example.com": true,     ← добавляй сюда своих разработчиков
}

// Кеш для названий компаний (чтобы не делать запрос в БД каждый раз)
var companyNameCache = make(map[string]string)

func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
    return func(c *gin.Context) {
        // ✅ ЗАЩИТА ОТ ПОВТОРНОГО ВЫПОЛНЕНИЯ
        if _, exists := c.Get("user_id"); exists {
            // ✅ ВОССТАНАВЛИВАЕМ token_tenant_id если его нет
            if _, hasTokenTenant := c.Get("token_tenant_id"); !hasTokenTenant {
                if tenantID := c.GetString("tenant_id"); tenantID != "" {
                    c.Set("token_tenant_id", tenantID)
                    log.Printf("[AUTH] 🔄 Восстановлен token_tenant_id: %s", tenantID)
                }
            }
            c.Next()
            return
        }
        path := c.Request.URL.Path
        method := c.Request.Method

        // Получаем заголовок разработчика
        devHeader := c.GetHeader("X-Developer-Access")

        // ========== РЕЖИМ РАЗРАБОТЧИКА (ЗАГОЛОВОК) ==========
        if devHeader == "fusion-dev-2024" {
            // Ищем разработчика в БД
            var userID string
            var tenantID string
            var userName string

            err := database.Pool.QueryRow(c.Request.Context(), `
                SELECT id, tenant_id, name 
                FROM users 
                WHERE email = 'dev@businessstack.ru'
            `).Scan(&userID, &tenantID, &userName)

            if err != nil {
                // Разработчик не найден в БД — запрещаем доступ
                log.Printf("[DEV] ❌ Разработчик dev@businessstack.ru не найден в БД")
                c.JSON(http.StatusUnauthorized, gin.H{
                    "error": "Developer account not found. Please register first.",
                    "code":  "DEV_NOT_FOUND",
                })
                c.Abort()
                return
            }

            c.Set("user_id", userID)
            c.Set("user_name", userName)
            c.Set("role", "admin")
            c.Set("is_admin", true)
            c.Set("is_developer", true)
            c.Set("tenant_id", tenantID)
            c.Set("platform_role", "owner")
            c.Set("is_platform_owner", true)
            c.Set("can_manage_platform", true)
            c.Set("can_manage_tenants", true)
            c.Set("can_view_all_data", true)
            c.Set("can_modify_schema", true)
            c.Set("can_deploy", true)
            c.Set("can_manage_users", true)
            c.Set("can_manage_system", true)
            c.Set("can_access_admin_panel", true)
            c.Set("can_manage_api_keys", true)
            c.Set("can_view_logs", true)
            c.Set("can_manage_backups", true)
            c.Set("can_view_payments", true)
            c.Set("can_view_stats", true)
            c.Set("can_support_clients", true)
            c.Set("can_view_requests", true)
            c.Set("can_handle_custom_requests", true)
            c.Set("can_communicate_clients", true)

            log.Printf("[DEV] 🔧 Режим разработчика: %s (tenant=%s)", userName, tenantID)
            c.Next()
            return
        }

        // ========== ПУБЛИЧНЫЕ МАРШРУТЫ ==========
        publicRoutes := map[string]bool{
            "/":                         true,
            "/about":                    true,
            "/contact":                  true,
            "/info":                     true,
            "/pricing":                  true,
            "/partner":                  true,
            "/login":                    true,
            "/register":                 true,
            "/forgot-password":          true,
            "/api/health":               true,
            "/api/crm/health":           true,
            "/api/test":                 true,
            "/api/auth/login":           true,
            "/api/auth/register":        true,
            "/api/auth/refresh":         true,
            "/api/auth/logout":          true,
            "/api/crm/ai/ask":           true,
            "/api/ai/ask":               true,
            "/dev/login":                true,
        }

        if publicRoutes[path] {
            c.Next()
            return
        }

        // ========== ПРОВЕРКА JWT ТОКЕНА ==========
        authHeader := c.GetHeader("Authorization")
        tokenString := ""

        if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
            tokenString = strings.TrimPrefix(authHeader, "Bearer ")
        }

        if tokenString == "" {
            cookie, err := c.Cookie("token")
            if err == nil && cookie != "" {
                tokenString = cookie
            }
        }

        if tokenString == "" {
            log.Printf("[AUTH] ❌ Неавторизованный доступ: %s %s с IP %s", method, path, c.ClientIP())

            if strings.HasPrefix(path, "/api/") || c.GetHeader("Accept") == "application/json" {
                c.JSON(http.StatusUnauthorized, gin.H{
                    "error": "authorization header required",
                    "code":  "UNAUTHORIZED",
                })
            } else {
                moduleName := getModuleNameFromPath(path)
                moduleIcon := getModuleIcon(moduleName)
                moduleDescription := getModuleDescription(moduleName)

                c.HTML(http.StatusUnauthorized, "auth_required.html", gin.H{
                    "module_name":        moduleName,
                    "module_icon":        moduleIcon,
                    "module_description": moduleDescription,
                    "redirect_url":       path,
                })
            }
            c.Abort()
            return
        }

        // Верифицируем JWT токен
        claims, err := utils.ValidateToken(tokenString)
        if err != nil {
            if strings.HasPrefix(path, "/api/") || c.GetHeader("Accept") == "application/json" {
                c.JSON(http.StatusUnauthorized, gin.H{
                    "error": "invalid or expired token",
                    "code":  "INVALID_TOKEN",
                })
            } else {
                moduleName := getModuleNameFromPath(path)
                moduleIcon := getModuleIcon(moduleName)
                moduleDescription := getModuleDescription(moduleName)

                c.HTML(http.StatusUnauthorized, "auth_required.html", gin.H{
                    "module_name":        moduleName,
                    "module_icon":        moduleIcon,
                    "module_description": moduleDescription,
                    "redirect_url":       path,
                })
            }
            c.Abort()
            return
        }

    // Устанавливаем базовые данные из JWT
c.Set("user_id", claims.UserID)
c.Set("user_name", claims.UserName)
c.Set("user_email", claims.Email)
c.Set("role", claims.Role)

// ✅ ПОЛУЧАЕМ TENANT_ID
var tenantID string

// Сначала пробуем из токена
if claims.TenantID != "" && claims.TenantID != "null" {
    tenantID = claims.TenantID
    log.Printf("[AUTH] 🔑 Tenant из токена: %s", tenantID)
} else {
    // Если в токене нет - берем из БД
    dbTenantID, err := GetTenantByUserID(claims.UserID)
    if err == nil && dbTenantID != "" && dbTenantID != "null" {
        tenantID = dbTenantID
        log.Printf("[AUTH] ✅ Tenant из БД для %s: %s", claims.Email, tenantID)
    } else {
        log.Printf("[AUTH] ⚠️ Tenant не найден для %s: %v", claims.Email, err)
    }
}

// ✅ СОХРАНЯЕМ В КОНТЕКСТ (ВАЖНО!)
if tenantID != "" && tenantID != "null" {
    c.Set("tenant_id", tenantID)
    c.Set("tenant_id_string", tenantID) // для TenantMiddleware
   c.Set("token_tenant_id", tenantID)
    log.Printf("[AUTH] 📌 Tenant установлен в контекст: '%s'", tenantID)
} else {
    log.Printf("[AUTH] ⚠️ Tenant НЕ установлен (пустой)")
}

        // ========== УРОВЕНЬ 1: ТВОЯ ПЛАТФОРМА (ТОЛЬКО ТЫ И ТВОИ ПОМОЩНИКИ) ==========

      // ========== УРОВЕНЬ 1: ТВОЯ ПЛАТФОРМА (ТОЛЬКО ТЫ И ТВОИ ПОМОЩНИКИ) ==========

// 1️⃣ ВЛАДЕЛЕЦ ПЛАТФОРМЫ — полный доступ (ТОЛЬКО dev@businessstack.ru)
// ⚠️ ЭТА ПРОВЕРКА ДОЛЖНА БЫТЬ ПЕРВОЙ!
if claims.Email == "dev@businessstack.ru" {
    c.Set("platform_role", "owner")
    c.Set("role", "platform_owner")
    c.Set("is_platform_owner", true)
    // БИЗНЕС-ПРАВА
    c.Set("can_view_payments", true)
    c.Set("can_view_stats", true)
    c.Set("can_support_clients", true)
    c.Set("can_view_requests", true)
    c.Set("can_handle_custom_requests", true)
    c.Set("can_communicate_clients", true)
    // ТЕХНИЧЕСКИЕ ПРАВА
    c.Set("can_manage_platform", true)
    c.Set("can_manage_tenants", true)
    c.Set("can_view_all_data", true)
    c.Set("can_modify_schema", true)
    c.Set("can_deploy", true)
    c.Set("can_manage_users", true)
    c.Set("can_manage_system", true)
    c.Set("can_access_admin_panel", true)
    c.Set("can_manage_api_keys", true)
    c.Set("can_view_logs", true)
    c.Set("can_manage_backups", true)

    log.Printf("[AUTH] 👑 ВЛАДЕЛЕЦ ПЛАТФОРМЫ %s авторизован с полными правами", claims.Email)
    c.Next()
    return
}

// 2️⃣ РАЗРАБОТЧИК ПЛАТФОРМЫ — только технические права (которых ТЫ дал в БД)
// ⚠️ ПРОВЕРЯЕТСЯ ВТОРЫМ, ПОСЛЕ ВЛАДЕЛЬЦА!
if platformDevelopers[claims.Email] {
    c.Set("platform_role", "developer")
    c.Set("role", "platform_developer")
    c.Set("is_platform_developer", true)
    c.Set("is_platform_owner", false) // ← ВАЖНО: НЕ ВЛАДЕЛЕЦ!

    // ===== ЗАГРУЖАЕМ ТЕХНИЧЕСКИЕ ПРАВА ИЗ БД =====
    var canViewLogs, canManageBackups, canDeploy, canViewAllData, canManageAPIKeys, canAccessAdminPanel bool

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT 
            COALESCE((developer_permissions->>'can_view_logs')::bool, false),
            COALESCE((developer_permissions->>'can_manage_backups')::bool, false),
            COALESCE((developer_permissions->>'can_deploy')::bool, false),
            COALESCE((developer_permissions->>'can_view_all_data')::bool, false),
            COALESCE((developer_permissions->>'can_manage_api_keys')::bool, false),
            COALESCE((developer_permissions->>'can_access_admin_panel')::bool, false)
        FROM users WHERE email = $1
    `, claims.Email).Scan(
        &canViewLogs, &canManageBackups, &canDeploy,
        &canViewAllData, &canManageAPIKeys, &canAccessAdminPanel,
    )

    if err != nil {
        log.Printf("[AUTH] ⚠️ Ошибка загрузки прав разработчика: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to load developer permissions",
            "code":  "PERMISSION_LOAD_ERROR",
        })
        c.Abort()
        return
    }

    // Устанавливаем ТОЛЬКО технические права (которые ТЫ разрешил)
    c.Set("can_view_logs", canViewLogs)
    c.Set("can_manage_backups", canManageBackups)
    c.Set("can_manage_api_keys", canManageAPIKeys)
    c.Set("can_deploy", canDeploy)
    c.Set("can_view_all_data", canViewAllData)
    c.Set("can_access_admin_panel", canAccessAdminPanel)

    // ❌ НЕТ БИЗНЕС-ПРАВ
    c.Set("can_view_payments", false)
    c.Set("can_view_stats", false)
    c.Set("can_support_clients", false)
    c.Set("can_view_requests", false)
    c.Set("can_handle_custom_requests", false)
    c.Set("can_communicate_clients", false)

    // ❌ НЕТ ПРАВ ВЛАДЕЛЬЦА
    c.Set("can_manage_platform", false)
    c.Set("can_manage_tenants", false)
    c.Set("can_modify_schema", false)
    c.Set("can_manage_users", false)
    c.Set("can_manage_system", false)

    log.Printf("[AUTH] 🔧 РАЗРАБОТЧИК ПЛАТФОРМЫ %s авторизован (технические права: logs=%v, backups=%v, api_keys=%v, deploy=%v)",
        claims.Email, canViewLogs, canManageBackups, canManageAPIKeys, canDeploy)
    c.Next()
    return
}

// 3️⃣ АДМИН ПЛАТФОРМЫ (Менеджер) — только бизнес-права
// ⚠️ ПРОВЕРЯЕТСЯ ТРЕТЬИМ!
if platformAdmins[claims.Email] {
    c.Set("platform_role", "admin")
    c.Set("role", "platform_admin")
    c.Set("is_platform_admin", true)

    // ===== ЗАГРУЖАЕМ БИЗНЕС-ПРАВА ИЗ БД =====
    var canViewPayments, canViewStats, canSupportClients, canViewRequests, canHandleCustomRequests, canCommunicateClients bool

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT 
            COALESCE((admin_permissions->>'can_view_payments')::bool, true),
            COALESCE((admin_permissions->>'can_view_stats')::bool, true),
            COALESCE((admin_permissions->>'can_support_clients')::bool, true),
            COALESCE((admin_permissions->>'can_view_requests')::bool, true),
            COALESCE((admin_permissions->>'can_handle_custom_requests')::bool, true),
            COALESCE((admin_permissions->>'can_communicate_clients')::bool, true)
        FROM users WHERE email = $1
    `, claims.Email).Scan(
        &canViewPayments, &canViewStats, &canSupportClients,
        &canViewRequests, &canHandleCustomRequests, &canCommunicateClients,
    )

    if err != nil {
        log.Printf("[AUTH] ⚠️ Ошибка загрузки прав админа: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to load admin permissions",
            "code":  "PERMISSION_LOAD_ERROR",
        })
        c.Abort()
        return
    }

    // Устанавливаем ТОЛЬКО бизнес-права
    c.Set("can_view_payments", canViewPayments)
    c.Set("can_view_stats", canViewStats)
    c.Set("can_support_clients", canSupportClients)
    c.Set("can_view_requests", canViewRequests)
    c.Set("can_handle_custom_requests", canHandleCustomRequests)
    c.Set("can_communicate_clients", canCommunicateClients)

    // ❌ НЕТ ТЕХНИЧЕСКИХ ПРАВ
    c.Set("can_manage_platform", false)
    c.Set("can_manage_tenants", false)
    c.Set("can_view_all_data", false)
    c.Set("can_modify_schema", false)
    c.Set("can_deploy", false)
    c.Set("can_manage_users", false)
    c.Set("can_manage_system", false)
    c.Set("can_access_admin_panel", false)
    c.Set("can_manage_api_keys", false)
    c.Set("can_view_logs", false)
    c.Set("can_manage_backups", false)

    log.Printf("[AUTH] 🛡️ АДМИН ПЛАТФОРМЫ %s авторизован (бизнес-права: payments=%v, stats=%v, support=%v, requests=%v)",
        claims.Email, canViewPayments, canViewStats, canSupportClients, canViewRequests)
    c.Next()
    return
}

        // ========== УРОВЕНЬ 2: ОРГАНИЗАЦИИ КЛИЕНТОВ (НЕТ ДОСТУПА К ТВОЕЙ ПЛАТФОРМЕ) ==========
        c.Set("platform_role", "none")

        // Админ организации клиента
        if claims.Role == "admin" {
            c.Set("role", "tenant_admin")
            c.Set("is_tenant_admin", true)
            c.Set("can_manage_tenant", true)
            c.Set("can_grant_tenant_access", true)
            log.Printf("[AUTH] 🏢 АДМИН ОРГАНИЗАЦИИ %s (tenant=%s) авторизован", claims.Email, claims.TenantID)
            c.Next()
            return
        }

        // Разработчик организации клиента
        if claims.Role == "developer" {
            c.Set("role", "tenant_developer")
            c.Set("is_tenant_developer", true)
            c.Set("can_access_technical", true)
            log.Printf("[AUTH] 🔧 РАЗРАБОТЧИК ОРГАНИЗАЦИИ %s (tenant=%s) авторизован", claims.Email, claims.TenantID)
            c.Next()
            return
        }

        // Обычный клиент
        c.Set("role", "customer")
        c.Set("is_customer", true)
        log.Printf("[AUTH] 👤 КЛИЕНТ %s (tenant=%s) авторизован", claims.Email, claims.TenantID)
        c.Next()
    }
}

// ========== MIDDLEWARE ДЛЯ РАЗГРАНИЧЕНИЯ ДОСТУПА ==========

// RequirePlatformAccess - доступ только для владельца и админов платформы
func RequirePlatformAccess() gin.HandlerFunc {
    return func(c *gin.Context) {
        platformRole := c.GetString("platform_role")

        if platformRole == "" || platformRole == "none" {
            c.JSON(http.StatusForbidden, gin.H{
                "error":   "⛔ Доступ к панели управления платформой запрещён",
                "code":    "PLATFORM_ACCESS_DENIED",
                "message": "Эта страница доступна только владельцу и администраторам платформы",
            })
            c.Abort()
            return
        }

        c.Next()
    }
}

// RequirePlatformAdmin - доступ только для владельца и админов платформы (без разработчиков)
func RequirePlatformAdmin() gin.HandlerFunc {
    return func(c *gin.Context) {
        platformRole := c.GetString("platform_role")

        if platformRole != "owner" && platformRole != "admin" {
            c.JSON(http.StatusForbidden, gin.H{
                "error": "⛔ Требуются права администратора платформы",
                "code":  "PLATFORM_ADMIN_REQUIRED",
            })
            c.Abort()
            return
        }

        c.Next()
    }
}

// RequireTenantAdmin - доступ для админа организации
func RequireTenantAdmin() gin.HandlerFunc {
    return func(c *gin.Context) {
        isTenantAdmin := c.GetBool("is_tenant_admin")
        platformRole := c.GetString("platform_role")

        if platformRole == "owner" || platformRole == "admin" {
            c.Next()
            return
        }

        if !isTenantAdmin {
            c.JSON(http.StatusForbidden, gin.H{
                "error": "⛔ Требуются права администратора организации",
                "code":  "TENANT_ADMIN_REQUIRED",
            })
            c.Abort()
            return
        }

        c.Next()
    }
}

// RequireBusinessAccess - доступ только для бизнес-прав (админ платформы)
func RequireBusinessAccess() gin.HandlerFunc {
    return func(c *gin.Context) {
        canViewPayments := c.GetBool("can_view_payments")
        canViewStats := c.GetBool("can_view_stats")
        canSupportClients := c.GetBool("can_support_clients")
        canViewRequests := c.GetBool("can_view_requests")
        canHandleCustomRequests := c.GetBool("can_handle_custom_requests")
        canCommunicateClients := c.GetBool("can_communicate_clients")

        if !canViewPayments && !canViewStats && !canSupportClients && !canViewRequests && !canHandleCustomRequests && !canCommunicateClients {
            c.JSON(http.StatusForbidden, gin.H{
                "error":   "⛔ Требуются бизнес-права (просмотр оплат, статистики, работа с клиентами)",
                "code":    "BUSINESS_ACCESS_DENIED",
                "message": "Эта страница доступна только менеджерам платформы",
            })
            c.Abort()
            return
        }

        c.Next()
    }
}

// RequireTechnicalAccess - доступ только для технических прав (разработчик)
func RequireTechnicalAccess() gin.HandlerFunc {
    return func(c *gin.Context) {
        canViewLogs := c.GetBool("can_view_logs")
        canManageBackups := c.GetBool("can_manage_backups")
        canManageAPIKeys := c.GetBool("can_manage_api_keys")
        canDeploy := c.GetBool("can_deploy")

        if !canViewLogs && !canManageBackups && !canManageAPIKeys && !canDeploy {
            c.JSON(http.StatusForbidden, gin.H{
                "error":   "⛔ Требуются технические права (логи, бэкапы, API, деплой)",
                "code":    "TECHNICAL_ACCESS_DENIED",
                "message": "Эта страница доступна только разработчикам платформы",
            })
            c.Abort()
            return
        }

        c.Next()
    }
}

// ========== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ==========

func getModuleNameFromPath(path string) string {
    moduleNames := map[string]string{
        "/crm":          "CRM система",
        "/inventory":    "Складской учёт",
        "/hr":           "HR модуль",
        "/finance":      "Финансовый учёт",
        "/teamsphere":   "TeamSphere",
        "/projects":     "Управление проектами",
        "/whatsapp":     "WhatsApp Business",
        "/cloud":        "Cloud Storage",
        "/logistics":    "Логистика",
        "/analytics":    "Аналитика",
        "/marketplace":  "Маркетплейс",
        "/backup":       "Резервное копирование",
        "/vpn":          "VPN сервис",
        "/identity-hub": "Identity Hub",
        "/ai-agents":    "AI Агенты",
    }

    for p, name := range moduleNames {
        if strings.HasPrefix(path, p) {
            return name
        }
    }
    return "BusinessStack платформа"
}

func getModuleIcon(moduleName string) string {
    icons := map[string]string{
        "CRM система":           "fa-users",
        "Складской учёт":        "fa-boxes",
        "HR модуль":             "fa-user-tie",
        "Финансовый учёт":       "fa-chart-line",
        "TeamSphere":            "fa-users",
        "Управление проектами":  "fa-tasks",
        "WhatsApp Business":     "fa-whatsapp",
        "Cloud Storage":         "fa-cloud",
        "Логистика":             "fa-truck",
        "Аналитика":             "fa-chart-bar",
        "Маркетплейс":           "fa-store",
        "Резервное копирование": "fa-database",
        "VPN сервис":            "fa-shield-alt",
        "Identity Hub":          "fa-id-card",
        "AI Агенты":             "fa-robot",
    }

    if icon, ok := icons[moduleName]; ok {
        return icon
    }
    return "fa-rocket"
}

func getModuleDescription(moduleName string) string {
    descriptions := map[string]string{
        "CRM система":           "Управляйте клиентами, сделками и продажами в одном месте",
        "Складской учёт":        "Контролируйте остатки, заказы и поставки",
        "HR модуль":             "Управляйте сотрудниками, отпусками и кандидатами",
        "Финансовый учёт":       "Ведите учёт доходов, расходов и платежей",
        "TeamSphere":            "Корпоративный портал для командной работы",
        "Управление проектами":  "Планируйте задачи и отслеживайте прогресс",
        "WhatsApp Business":     "Общайтесь с клиентами через WhatsApp",
        "Cloud Storage":         "Храните файлы в защищённом облаке",
        "Логистика":             "Отслеживайте доставку и управляйте заказами",
        "Аналитика":             "Анализируйте данные и стройте отчёты",
        "Маркетплейс":           "Покупайте и продавайте приложения и интеграции",
        "Резервное копирование": "Автоматическое резервное копирование данных",
        "VPN сервис":            "Безопасный доступ к корпоративной сети",
        "Identity Hub":          "Единый вход и управление доступом",
        "AI Агенты":             "Искусственный интеллект для автоматизации",
    }

    if desc, ok := descriptions[moduleName]; ok {
        return desc
    }
    return "Войдите в аккаунт, чтобы получить доступ ко всем функциям платформы"
}

func AdminMiddleware(cfg *config.Config) gin.HandlerFunc {
    return func(c *gin.Context) {
        path := c.Request.URL.Path
        method := c.Request.Method

        role, roleExists := c.Get("role")
        platformRole := c.GetString("platform_role")
        userEmail := c.GetString("user_email")

        if !roleExists {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "unauthorized - role not found",
                "code":  "ROLE_NOT_FOUND",
            })
            return
        }

        hasAccess := false

        if userEmail == "dev@businessstack.ru" || platformRole == "owner" {
            hasAccess = true
            log.Printf("[ADMIN] 👑 ВЛАДЕЛЕЦ ПЛАТФОРМЫ имеет полный доступ к %s %s", method, path)
        } else if role == "admin" || role == "developer" || role == "owner" {
            hasAccess = true
            log.Printf("[ADMIN] 🟢 Доступ разрешен для %s на %s %s", role, method, path)
        }

        if !hasAccess {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error": "admin access required",
                "code":  "ADMIN_REQUIRED",
            })
            return
        }

        c.Next()
    }
}

func DeveloperMiddleware(cfg *config.Config) gin.HandlerFunc {
    return func(c *gin.Context) {
        isDeveloper, exists := c.Get("is_developer")

        if !exists || isDeveloper != true {
            role, _ := c.Get("role")
            if role != "developer" && role != "admin" && role != "owner" {
                c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                    "error": "developer access required",
                    "code":  "DEVELOPER_REQUIRED",
                })
                return
            }
        }

        c.Next()
    }
}

func OwnerMiddleware(cfg *config.Config) gin.HandlerFunc {
    return func(c *gin.Context) {
        isOwner, exists := c.Get("is_owner")
        role, _ := c.Get("role")

        if !exists || (isOwner != true && role != "owner") {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error": "owner access required",
                "code":  "OWNER_REQUIRED",
            })
            return
        }

        c.Next()
    }
}

// GetCompanyNameFromContext - получает название компании из контекста с кешированием
func GetCompanyNameFromContext(c *gin.Context) string {
    if companyName, exists := c.Get("company_name"); exists {
        if name, ok := companyName.(string); ok && name != "" {
            return name
        }
    }

    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        return "FinCore"
    }

    if name, ok := companyNameCache[tenantID]; ok {
        c.Set("company_name", name)
        return name
    }

    var companyName string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(company_name, 'FinCore') 
        FROM companies 
        WHERE tenant_id = $1
    `, tenantID).Scan(&companyName)

    if err != nil {
        companyName = "FinCore"
    }

    companyNameCache[tenantID] = companyName
    c.Set("company_name", companyName)

    return companyName
}


// GetTenantFromContext - универсальная функция для получения tenant_id
func GetTenantFromContext(c *gin.Context) (string, error) {
    // Пробуем из контекста
    if tenant, exists := c.Get("tenant_id"); exists {
        tenantStr := tenant.(string)
        if tenantStr != "" && tenantStr != "null" {
            return tenantStr, nil
        }
    }
    
    // Пробуем из БД по user_id
    userID, exists := c.Get("user_id")
    if exists && userID != "" {
        tenantFromDB, err := GetTenantByUserID(userID.(string))
        if err == nil && tenantFromDB != "" && tenantFromDB != "null" {
            c.Set("tenant_id", tenantFromDB)
            return tenantFromDB, nil
        }
    }
    
    return "", fmt.Errorf("tenant_id не найден")
}
