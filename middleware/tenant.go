package middleware

import (
    "log"
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
)

// TenantMiddleware - определяет тенанта по subdomain
func TenantMiddleware(db *pgxpool.Pool) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Определяем тенанта по subdomain из URL или заголовка
        host := c.Request.Host
        subdomain := extractSubdomain(host)
        log.Printf("🔍 TenantMiddleware: host=%s, subdomain=%s", host, subdomain)

        // Проверяем, есть ли tenant в query параметре (для разработки)
        if tenantParam := c.Query("tenant"); tenantParam != "" {
            subdomain = tenantParam
        }

        var tenantID uuid.UUID
        var tenantName string
        var tenantSettings []byte

        // Ищем тенанта по subdomain
        err := db.QueryRow(c.Request.Context(), `
            SELECT id, name, settings FROM tenants 
            WHERE subdomain = $1 AND status = 'active'
        `, subdomain).Scan(&tenantID, &tenantName, &tenantSettings)

        if err != nil {
            // Если не нашли, используем дефолтного
            err = db.QueryRow(c.Request.Context(), `
                SELECT id, name, settings FROM tenants 
                WHERE subdomain = 'default'
            `).Scan(&tenantID, &tenantName, &tenantSettings)

            if err != nil {
                // ✅ Если даже дефолтного нет - используем тестовый tenant (не падаем)
                tenantID, _ = uuid.Parse("11111111-1111-1111-1111-111111111111")
                tenantName = "Default Company"
                log.Printf("⚠️ Tenant не найден, используем тестовый: %s", tenantID)
            }
        }

        // ✅ Сохраняем в контекст ОБЕ версии - UUID и STRING
        c.Set("tenant_id", tenantID)
        c.Set("tenant_id_string", tenantID.String()) // ← ДОБАВЛЕНО
        c.Set("tenant_name", tenantName)
        c.Set("tenant_subdomain", subdomain)

        // Добавляем tenant_id в заголовки для API
        c.Header("X-Tenant-ID", tenantID.String())
        c.Header("X-Tenant-Name", tenantName)

        c.Next()
    }
}

// extractSubdomain - извлекает subdomain из host
func extractSubdomain(host string) string {
    // Убираем порт
    if idx := strings.Index(host, ":"); idx != -1 {
        host = host[:idx]
    }

    parts := strings.Split(host, ".")
    if len(parts) >= 2 {
        // Если это localhost или IP, возвращаем default
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

// ✅ НОВАЯ ФУНКЦИЯ - получить tenant_id как строку
func GetTenantIDString(c *gin.Context) string {
    // Пробуем получить строковую версию
    if tenantIDStr, exists := c.Get("tenant_id_string"); exists {
        if str, ok := tenantIDStr.(string); ok {
            return str
        }
    }

    // Если нет, пробуем получить UUID и преобразовать
    if tenantID, exists := c.Get("tenant_id"); exists {
        if id, ok := tenantID.(uuid.UUID); ok {
            return id.String()
        }
    }

    // Пробуем получить из заголовка
    if headerID := c.GetHeader("X-Tenant-ID"); headerID != "" {
        return headerID
    }

    // Tenant по умолчанию (из вашей БД)
    return "11111111-1111-1111-1111-111111111111"
}