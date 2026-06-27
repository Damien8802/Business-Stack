package middleware

import (
    "fmt"
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

func DevModulesMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        requestPath := c.Request.URL.Path

   // ✅ ПРОПУСКАЕМ /profile (без логов и без проверки)
        if requestPath == "/profile" {
            c.Next()
            return
        }
        
        // Убираем trailing slash для сравнения с БД
        if len(requestPath) > 1 && strings.HasSuffix(requestPath, "/") {
            requestPath = requestPath[:len(requestPath)-1]
        }
        
        userEmail := c.GetString("user_email")
        
        // ✅ ИСПРАВЛЕНО: получаем tenant как строку
        tenantID := c.GetString("tenant_id_string") // ← ИЗМЕНЕНО!
        if tenantID == "" {
            // Fallback: пробуем из заголовка
            tenantID = c.GetHeader("X-Tenant-ID")
        }
        if tenantID == "" {
            // Fallback: пробуем из параметра
            tenantID = c.Query("tenant_id")
        }

        fmt.Printf("🔥🔥🔥 [DevModules] START: path=%s, email=%s, tenant=%s\n", requestPath, userEmail, tenantID)

        // Владельцы пропускаются
        if isOwner(userEmail) {
            fmt.Printf("👑 [DevModules] Owner %s - пропускаем\n", userEmail)
            c.Next()
            return
        }

        // Пропускаем API, статику и специфические пути
        if strings.HasPrefix(requestPath, "/api/") ||
           strings.HasPrefix(requestPath, "/static/") ||
           strings.HasPrefix(requestPath, "/frontend/") ||
           strings.HasPrefix(requestPath, "/app/") ||
           requestPath == "/favicon.ico" ||
           requestPath == "/favicon.svg" ||
           requestPath == "/dev-modules" ||
           strings.Contains(requestPath, ".") {
            fmt.Printf("⏭️ [DevModules] Skip path: %s\n", requestPath)
            c.Next()
            return
        }

        // Проверяем в БД
        var moduleName, moduleIcon, moduleDesc, moduleStatus string
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT name, COALESCE(icon, '🔧'), COALESCE(description, ''), COALESCE(status, 'development')
            FROM dev_modules WHERE route = $1
        `, requestPath).Scan(&moduleName, &moduleIcon, &moduleDesc, &moduleStatus)

        if err != nil {
            fmt.Printf("❌ [DevModules] Module NOT found: %v (tenant=%s)\n", err, tenantID)
            c.Next()
            return
        }

        fmt.Printf("✅ [DevModules] Module FOUND: %s, status=%s, tenant=%s\n", moduleName, moduleStatus, tenantID)

        if moduleStatus == "development" {
            fmt.Printf("🏗️ [DevModules] SHOW UNDER CONSTRUCTION for %s\n", moduleName)
            c.HTML(http.StatusOK, "under_construction.html", gin.H{
                "title": "Модуль в разработке | Business Stack",
                "name":  moduleName,
                "icon":  moduleIcon,
                "desc":  moduleDesc,
            })
            c.Abort()
            return
        }

        c.Next()
    }
}