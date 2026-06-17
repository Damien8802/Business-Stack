package middleware

import (
    "log"
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
    "subscription-system/database"
)

func TenantMiddleware(db *pgxpool.Pool) gin.HandlerFunc {
    return func(c *gin.Context) {
        var tenantID uuid.UUID
        
        // 1. Публичные маршруты - пропускаем
        publicPaths := []string{
            "/login", "/register", "/forgot-password", 
            "/api/auth/login", "/api/auth/register", "/api/auth/refresh",
            "/", "/about", "/contact", "/pricing", "/docs", "/favicon.ico",
        }
        for _, path := range publicPaths {
            if c.Request.URL.Path == path {
                c.Next()
                return
            }
        }
        
        // 2. Получаем данные из контекста (установлены AuthMiddleware)
        userID := c.GetString("user_id")
        userEmail := c.GetString("user_email")
        tokenTenantID := c.GetString("tenant_id") // tenant из токена
        
        log.Printf("🔍 [TenantMiddleware] ========================================")
        log.Printf("🔍 [TenantMiddleware] Path: %s", c.Request.URL.Path)
        log.Printf("🔍 [TenantMiddleware] user_id: '%s'", userID)
        log.Printf("🔍 [TenantMiddleware] user_email: '%s'", userEmail)
        log.Printf("🔍 [TenantMiddleware] token_tenant_id: '%s'", tokenTenantID)
        
        // 3. СНАЧАЛА пробуем tenant из токена (самый быстрый)
       if tokenTenantID != "" {
            parsedTenant, err := uuid.Parse(tokenTenantID)
            if err == nil && parsedTenant != uuid.Nil {
                tenantID = parsedTenant
                log.Printf("✅ [TenantMiddleware] Tenant из ТОКЕНА: %s", tenantID)
            } else {
                log.Printf("⚠️ [TenantMiddleware] Невалидный tenant из токена: '%s'", tokenTenantID)
            }
        }
        
        // 4. Если не нашли - ищем по user_id в БД
        if tenantID == uuid.Nil && userID != "" {
            var dbTenantID uuid.UUID
            err := db.QueryRow(c.Request.Context(), `
                SELECT tenant_id FROM users WHERE id = $1
            `, userID).Scan(&dbTenantID)
            
            if err == nil && dbTenantID != uuid.Nil {
                tenantID = dbTenantID
                log.Printf("✅ [TenantMiddleware] Tenant из БД по user_id %s: %s", userID, tenantID)
            } else {
                log.Printf("⚠️ [TenantMiddleware] user_id %s не найден или tenant = NULL, ошибка: %v", userID, err)
            }
        }
        
        // 5. Если не нашли - ищем по user_email в БД
        if tenantID == uuid.Nil && userEmail != "" {
            var dbTenantID uuid.UUID
            err := db.QueryRow(c.Request.Context(), `
                SELECT tenant_id FROM users WHERE email = $1
            `, userEmail).Scan(&dbTenantID)
            
            if err == nil && dbTenantID != uuid.Nil {
                tenantID = dbTenantID
                log.Printf("✅ [TenantMiddleware] Tenant из БД по email %s: %s", userEmail, tenantID)
            } else {
                log.Printf("⚠️ [TenantMiddleware] email %s не найден или tenant = NULL, ошибка: %v", userEmail, err)
            }
        }
        
        // 6. Если не нашли - ищем по субдомену
        if tenantID == uuid.Nil {
            host := c.Request.Host
            subdomain := extractSubdomain(host)
            
            if subdomain == "default" || subdomain == "localhost" {
                log.Printf("❌ [TenantMiddleware] localhost без tenant, user_id=%s, user_email=%s", userID, userEmail)
                c.AbortWithStatusJSON(401, gin.H{"error": "Authentication required", "details": "No tenant found for user"})
                return
            }
            
            err := db.QueryRow(c.Request.Context(), `
                SELECT id FROM tenants 
                WHERE subdomain = $1 AND status = 'active'
            `, subdomain).Scan(&tenantID)
            
            if err != nil {
                log.Printf("❌ [TenantMiddleware] Субдомен не найден: %s, ошибка: %v", subdomain, err)
                c.AbortWithStatusJSON(404, gin.H{"error": "Company not found"})
                return
            }
            log.Printf("✅ [TenantMiddleware] Tenant из субдомена %s: %s", subdomain, tenantID)
        }
        
        // 7. Финальная проверка
        if tenantID == uuid.Nil {
            log.Printf("❌ [TenantMiddleware] НЕ УДАЛОСЬ НАЙТИ TENANT! user_id=%s, user_email=%s", userID, userEmail)
            c.AbortWithStatusJSON(401, gin.H{"error": "Invalid tenant"})
            return
        }
        
        // 8. Проверяем, что tenant существует в БД
        var exists bool
        err := db.QueryRow(c.Request.Context(), 
            "SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)", tenantID).Scan(&exists)
        if err != nil || !exists {
            log.Printf("❌ [TenantMiddleware] Tenant %s не существует в БД", tenantID)
            c.AbortWithStatusJSON(401, gin.H{"error": "Invalid tenant"})
            return
        }
        
        // 9. Сохраняем в контекст
        c.Set("tenant_id", tenantID)
        c.Set("tenant_id_string", tenantID.String())
        c.Header("X-Tenant-ID", tenantID.String())
        
        log.Printf("✅ [TenantMiddleware] УСПЕХ! final tenant_id = %s для path %s", tenantID, c.Request.URL.Path)
        log.Printf("🔍 [TenantMiddleware] ========================================")
        
        c.Next()
    }
}
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
    // Пробуем из tenant_id (UUID)
    if tenantID, exists := c.Get("tenant_id"); exists {
        if id, ok := tenantID.(uuid.UUID); ok && id != uuid.Nil {
            return id
        }
    }
    
    // Пробуем из tenant_id_string (строка)
    if tenantIDStr := c.GetString("tenant_id_string"); tenantIDStr != "" {
        if id, err := uuid.Parse(tenantIDStr); err == nil && id != uuid.Nil {
            return id
        }
    }
    
    // Пробуем из заголовка
    if headerID := c.GetHeader("X-Tenant-ID"); headerID != "" {
        if id, err := uuid.Parse(headerID); err == nil && id != uuid.Nil {
            c.Set("tenant_id", id)
            c.Set("tenant_id_string", headerID)
            return id
        }
    }
    
    // Пробуем из user_id (берем tenant из БД)
    userID := c.GetString("user_id")
    if userID != "" {
        var dbTenantID uuid.UUID
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT COALESCE(tenant_id, id::text) FROM users WHERE id = $1
        `, userID).Scan(&dbTenantID)
        if err == nil && dbTenantID != uuid.Nil {
            c.Set("tenant_id", dbTenantID)
            c.Set("tenant_id_string", dbTenantID.String())
            return dbTenantID
        }
    }
    
    log.Printf("❌ GetTenantIDFromContext: tenant_id не найден для user_id=%s", userID)
    return uuid.Nil
}