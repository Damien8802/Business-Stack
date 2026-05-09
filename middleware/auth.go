package middleware

import (
    "log"
    "net/http"
    "strings"
    "subscription-system/config"
    "subscription-system/utils"

    "github.com/gin-gonic/gin"
)

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
            c.JSON(http.StatusUnauthorized, gin.H{
                "error": "authorization header required",
                "code":  "UNAUTHORIZED",
            })
            c.Abort()
            return
        }

        // Верифицируем JWT токен
        claims, err := utils.ValidateToken(tokenString)
        if err != nil {
            c.JSON(http.StatusUnauthorized, gin.H{
                "error": "invalid or expired token",
                "code":  "INVALID_TOKEN",
            })
            c.Abort()
            return
        }

        // Устанавливаем базовые данные из JWT
        c.Set("user_id", claims.UserID)
        c.Set("user_name", claims.UserName)
        c.Set("user_email", claims.Email)
        c.Set("role", claims.Role)
        c.Set("tenant_id", claims.TenantID)

        // ========== ПРОВЕРКА ДЛЯ КОНКРЕТНЫХ EMAIL (ВРЕМЕННОЕ РЕШЕНИЕ) ==========
        // Устанавливаем права на основе email
        if claims.Email == "dev@businesstack.ru" {
            c.Set("role", "owner")
            c.Set("is_owner", true)
            c.Set("is_admin", true)
            c.Set("is_developer", true)
            c.Set("developer_level", 999)
            c.Set("super_admin", true)
            c.Set("can_manage_users", true)
            c.Set("can_manage_system", true)
            c.Set("can_view_all_data", true)
            c.Set("can_modify_schema", true)
            c.Set("can_deploy", true)
            c.Set("can_access_admin_panel", true)
            c.Set("can_manage_api_keys", true)
            c.Set("can_view_logs", true)
            c.Set("can_manage_backups", true)
            log.Printf("[AUTH] 👑 ВЛАДЕЛЕЦ %s авторизован с полными правами", claims.Email)
            c.Next()
            return
        }

        // Для остальных пользователей - роль из JWT
        if claims.Role == "admin" {
            c.Set("is_admin", true)
            log.Printf("[AUTH] ✅ АДМИН %s авторизован", claims.Email)
        } else if claims.Role == "developer" {
            c.Set("is_developer", true)
            c.Set("is_admin", true)
            log.Printf("[AUTH] ✅ РАЗРАБОТЧИК %s авторизован", claims.Email)
        } else {
            log.Printf("[AUTH] ✅ Авторизован: %s (%s), роль=%s", claims.UserName, claims.Email, claims.Role)
        }

        c.Next()
    }
}

func AdminMiddleware(cfg *config.Config) gin.HandlerFunc {
    return func(c *gin.Context) {
        path := c.Request.URL.Path
        method := c.Request.Method

        // Проверяем роль из контекста
        role, roleExists := c.Get("role")
        isAdmin, adminExists := c.Get("is_admin")
        isOwner, ownerExists := c.Get("is_owner")
        isDeveloper, devExists := c.Get("is_developer")

        // Если нет роли - запрещаем
        if !roleExists {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "unauthorized - role not found",
                "code":  "ROLE_NOT_FOUND",
            })
            return
        }

        // Разрешаем доступ владельцам, админам и разработчикам
        hasAccess := false
        
        if ownerExists && isOwner == true {
            hasAccess = true
            log.Printf("[ADMIN] 👑 ВЛАДЕЛЕЦ имеет полный доступ к %s %s", method, path)
        } else if adminExists && isAdmin == true {
            hasAccess = true
            log.Printf("[ADMIN] 🟢 АДМИН имеет доступ к %s %s", method, path)
        } else if devExists && isDeveloper == true {
            hasAccess = true
            log.Printf("[ADMIN] 🔧 РАЗРАБОТЧИК имеет доступ к %s %s", method, path)
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