package middleware

import (
    "log"
    "net/http"
    "strings"
    "subscription-system/config"
    "subscription-system/utils"

    "github.com/gin-gonic/gin"
)

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

func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
    return func(c *gin.Context) {
        path := c.Request.URL.Path
        method := c.Request.Method

        // Получаем заголовок разработчика
        devHeader := c.GetHeader("X-Developer-Access")

        // ========== РЕЖИМ РАЗРАБОТЧИКА (ЗАГОЛОВОК) ==========
        if devHeader == "fusion-dev-2024" {
            userID := "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
            c.Set("user_id", userID)
            c.Set("user_name", "Разработчик")
            c.Set("role", "admin")
            c.Set("is_admin", true)
            c.Set("is_developer", true)
            c.Set("tenant_id", "11111111-1111-1111-1111-111111111111")
            log.Printf("[DEV] 🔧 Режим разработчика: %s %s (заголовок принят)", method, path)
            c.Next()
            return
        }

        // Пропускаем маршруты архива
        if strings.HasPrefix(path, "/archive/") {
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
            "/fusion-portal":            true,
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
            
            // Проверяем, ожидается ли JSON ответ
            if strings.HasPrefix(path, "/api/") || c.GetHeader("Accept") == "application/json" {
                c.JSON(http.StatusUnauthorized, gin.H{
                    "error": "authorization header required",
                    "code":  "UNAUTHORIZED",
                })
            } else {
                // Показываем красивую HTML страницу
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
            // Токен невалидный, показываем страницу входа
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
        c.Set("tenant_id", claims.TenantID)

        // ========== УРОВЕНЬ 1: ТВОЯ ПЛАТФОРМА (ТОЛЬКО ТЫ И ТВОИ ПОМОЩНИКИ) ==========
        // Это ДОСТУП К УПРАВЛЕНИЮ ПЛАТФОРМОЙ (глобальная админка)
        if claims.Email == "dev@businessstack.ru" {
            c.Set("platform_role", "owner")
            c.Set("role", "platform_owner")
            c.Set("is_platform_owner", true)
            c.Set("is_platform_admin", true)
            c.Set("can_manage_platform", true)
            c.Set("can_manage_all_tenants", true)
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

        // Твои личные админы (помощники, которых ТЫ назначил)
        if platformAdmins[claims.Email] {
            c.Set("platform_role", "admin")
            c.Set("role", "platform_admin")
            c.Set("is_platform_admin", true)
            c.Set("can_manage_platform", true)
            c.Set("can_manage_all_tenants", true)
            log.Printf("[AUTH] 🛡️ АДМИН ПЛАТФОРМЫ %s авторизован", claims.Email)
            c.Next()
            return
        }

        // Твои личные разработчики (помощники, которых ТЫ нанял)
        if platformDevelopers[claims.Email] {
            c.Set("platform_role", "developer")
            c.Set("role", "platform_developer")
            c.Set("is_platform_developer", true)
            c.Set("can_access_technical", true)
            log.Printf("[AUTH] 🔧 РАЗРАБОТЧИК ПЛАТФОРМЫ %s авторизован", claims.Email)
            c.Next()
            return
        }

        // ========== УРОВЕНЬ 2: ОРГАНИЗАЦИИ КЛИЕНТОВ (НЕТ ДОСТУПА К ТВОЕЙ ПЛАТФОРМЕ) ==========
        // Здесь идут админы и разработчики, которые работают ВНУТРИ своих организаций
        // Они НЕ МОГУТ зайти в твою глобальную админку!
        
        c.Set("platform_role", "none") // Нет прав на управление платформой

        // Админ организации клиента (управляет только своей компанией)
        if claims.Role == "admin" {
            c.Set("role", "tenant_admin")
            c.Set("is_tenant_admin", true)
            c.Set("can_manage_tenant", true)       // Управление своей организацией
            c.Set("can_grant_tenant_access", true) // Может выдавать доступ внутри своей компании
            log.Printf("[AUTH] 🏢 АДМИН ОРГАНИЗАЦИИ %s (tenant=%s) авторизован", claims.Email, claims.TenantID)
            c.Next()
            return
        }

        // Разработчик организации клиента (техническая роль внутри своей компании)
        if claims.Role == "developer" {
            c.Set("role", "tenant_developer")
            c.Set("is_tenant_developer", true)
            c.Set("can_access_technical", true)
            log.Printf("[AUTH] 🔧 РАЗРАБОТЧИК ОРГАНИЗАЦИИ %s (tenant=%s) авторизован", claims.Email, claims.TenantID)
            c.Next()
            return
        }

        // Обычный клиент (пользователь организации)
        c.Set("role", "customer")
        c.Set("is_customer", true)
        log.Printf("[AUTH] 👤 КЛИЕНТ %s (tenant=%s) авторизован", claims.Email, claims.TenantID)
        c.Next()
    }
}

// ========== НОВЫЕ MIDDLEWARE ДЛЯ РАЗГРАНИЧЕНИЯ ДОСТУПА ==========

// RequirePlatformAccess - доступ только для твоих сотрудников (владелец, админ платформы, разработчик платформы)
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

// RequireTenantAdmin - доступ для админа организации (но не для глобального админа платформы в чужой тенант)
func RequireTenantAdmin() gin.HandlerFunc {
    return func(c *gin.Context) {
        isTenantAdmin := c.GetBool("is_tenant_admin")
        platformRole := c.GetString("platform_role")
        
        // Если это глобальный админ платформы - тоже пропускаем (он может всё)
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


// Вспомогательные функции для красивого отображения
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

        // Получаем роль и platform_role
        role, roleExists := c.Get("role")
        platformRole := c.GetString("platform_role")
        userEmail := c.GetString("user_email")

        // Если нет роли - запрещаем
        if !roleExists {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "unauthorized - role not found",
                "code":  "ROLE_NOT_FOUND",
            })
            return
        }

        // Разрешаем доступ владельцам платформы, админам и разработчикам
        hasAccess := false
        
        // Владелец платформы
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


// Дополнительная функция для проверки прав разработчика
func DeveloperMiddleware(cfg *config.Config) gin.HandlerFunc {
    return func(c *gin.Context) {
        isDeveloper, exists := c.Get("is_developer")
        
        if !exists || isDeveloper != true {
            // Проверяем роль
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

// Функция для проверки прав владельца
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