package middleware

import (
    "log"
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
)

// TenantMiddleware - определяет тенанта по subdomain или из пользователя
func TenantMiddleware(db *pgxpool.Pool) gin.HandlerFunc {
    return func(c *gin.Context) {
        var tenantID uuid.UUID
        
        // 1. Публичные маршруты - пропускаем (не требуют tenant)
        publicPaths := []string{
            "/login", "/register", "/forgot-password", 
            "/api/auth/login", "/api/auth/register", "/api/auth/refresh",
            "/", "/about", "/contact", "/pricing", "/docs",
        }
        for _, path := range publicPaths {
            if c.Request.URL.Path == path {
                c.Next()
                return
            }
        }
        
        // 2. Пробуем получить tenant из авторизованного пользователя
        if userVal, exists := c.Get("user"); exists {
            switch u := userVal.(type) {
            case map[string]interface{}:
                if tid, ok := u["tenant_id"].(string); ok && tid != "" {
                    tenantID, _ = uuid.Parse(tid)
                    log.Printf("🔍 TenantMiddleware: tenant from user map = %s", tenantID)
                }
            default:
                log.Printf("🔍 TenantMiddleware: user type = %T", u)
            }
        }
        
        // 3. Если не нашли в пользователе - берем из subdomain
        if tenantID == uuid.Nil {
            host := c.Request.Host
            subdomain := extractSubdomain(host)
            
            // Для localhost - НЕ УСТАНАВЛИВАЕМ tenant_id, он должен быть из пользователя
            if subdomain == "default" || subdomain == "localhost" {
                log.Printf("🔍 TenantMiddleware: localhost, tenant_id будет из пользователя")
                c.Next()
                return
            }
            
            // Ищем tenant по subdomain
            err := db.QueryRow(c.Request.Context(), `
                SELECT id FROM tenants 
                WHERE subdomain = $1 AND status = 'active'
            `, subdomain).Scan(&tenantID)
            
            if err != nil {
                log.Printf("❌ TenantMiddleware: tenant not found for subdomain: %s", subdomain)
                c.AbortWithStatusJSON(404, gin.H{"error": "Company not found"})
                return
            }
            log.Printf("🔍 TenantMiddleware: tenant from subdomain %s = %s", subdomain, tenantID)
        }
        
        // 4. Проверяем, что tenant существует в БД
        if tenantID != uuid.Nil {
            var exists bool
            err := db.QueryRow(c.Request.Context(), 
                "SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)", tenantID).Scan(&exists)
            if err != nil || !exists {
                log.Printf("❌ TenantMiddleware: tenant %s does not exist", tenantID)
                c.AbortWithStatusJSON(401, gin.H{"error": "Invalid tenant"})
                return
            }
        }
        
        // 5. Сохраняем tenant в контекст
        c.Set("tenant_id", tenantID)
        c.Set("tenant_id_string", tenantID.String())
        c.Header("X-Tenant-ID", tenantID.String())
        
        log.Printf("✅ TenantMiddleware: final tenant_id = %s for path %s", tenantID, c.Request.URL.Path)
        
        c.Next()
    }
}

// extractSubdomain - извлекает subdomain из host
func extractSubdomain(host string) string {
    if idx := strings.Index(host, ":"); idx != -1 {
        host = host[:idx]
    }

    parts := strings.Split(host, ".")
    if len(parts) >= 2 {
        if host == "localhost" || strings.Contains(host, "127.0.0.1") {
            return "default"
        }
        return parts[0]
    }
    return "default"
}

// GetTenantIDFromContext - получить tenant_id из контекста как UUID
func GetTenantIDFromContext(c *gin.Context) uuid.UUID {
    if tenantID, exists := c.Get("tenant_id"); exists {
        if id, ok := tenantID.(uuid.UUID); ok {
            return id
        }
    }
    return uuid.Nil
}

// GetTenantIDString - получить tenant_id как строку
func GetTenantIDString(c *gin.Context) string {
    if tenantIDStr, exists := c.Get("tenant_id_string"); exists {
        if str, ok := tenantIDStr.(string); ok {
            return str
        }
    }
    if tenantID, exists := c.Get("tenant_id"); exists {
        if id, ok := tenantID.(uuid.UUID); ok {
            return id.String()
        }
    }
    if headerID := c.GetHeader("X-Tenant-ID"); headerID != "" {
        return headerID
    }
    return ""
}