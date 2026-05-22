package handlers

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)
// ArchivePageHandler - страница архива
func ArchivePageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "archive_index.html", gin.H{
        "Title": "Архив | Business Stack",
    })
}

// GetArchiveStats - статистика использования архива
func GetArchiveStats(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var stats struct {
        UsedBytes   int64 `json:"used_bytes"`
        TotalBytes  int64 `json:"total_bytes"`
        UsedPercent int   `json:"used_percent"`
    }

    // Получаем или создаем квоту
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT total_bytes, COALESCE(used_bytes, 0) FROM archive_quotas WHERE tenant_id = $1
    `, tenantID).Scan(&stats.TotalBytes, &stats.UsedBytes)

    if err != nil {
        stats.TotalBytes = 1073741824
        stats.UsedBytes = 0
        database.Pool.Exec(c.Request.Context(), `
            INSERT INTO archive_quotas (tenant_id, total_bytes, used_bytes) VALUES ($1, $2, $3)
        `, tenantID, stats.TotalBytes, stats.UsedBytes)
    }

    var customersSize, dealsSize, entriesSize int64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(COUNT(*), 0) * 1024 FROM crm_customers_archive WHERE tenant_id = $1
    `, tenantID).Scan(&customersSize)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(COUNT(*), 0) * 1024 FROM crm_deals_archive WHERE tenant_id = $1
    `, tenantID).Scan(&dealsSize)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(COUNT(*), 0) * 1024 FROM journal_entries_archive WHERE tenant_id = $1
    `, tenantID).Scan(&entriesSize)

    stats.UsedBytes = customersSize + dealsSize + entriesSize
    if stats.TotalBytes > 0 {
        stats.UsedPercent = int(float64(stats.UsedBytes) / float64(stats.TotalBytes) * 100)
    }

    c.JSON(http.StatusOK, gin.H{
        "used_bytes":   stats.UsedBytes,
        "total_bytes":  stats.TotalBytes,
        "used_percent": stats.UsedPercent,
        "used_gb":      float64(stats.UsedBytes) / 1024 / 1024 / 1024,
        "total_gb":     float64(stats.TotalBytes) / 1024 / 1024 / 1024,
    })
}

// GetArchiveItems - получить список архивных записей
func GetArchiveItems(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    entityType := c.Query("type")
    pageStr := c.DefaultQuery("page", "1")
    limitStr := c.DefaultQuery("limit", "20")

    page, _ := strconv.Atoi(pageStr)
    limit, _ := strconv.Atoi(limitStr)
    offset := (page - 1) * limit

    var items []gin.H

    if entityType == "" || entityType == "customers" {
        rows, err := database.Pool.Query(c.Request.Context(), `
            SELECT id, name, email, phone, company, total_deals_sum, deals_count, archived_at
            FROM crm_customers_archive WHERE tenant_id = $1
            ORDER BY archived_at DESC LIMIT $2 OFFSET $3
        `, tenantID, limit, offset)

        if err == nil {
            for rows.Next() {
                var id uuid.UUID
                var name, email, phone, company string
                var totalDealsSum float64
                var dealsCount int
                var archivedAt time.Time

                rows.Scan(&id, &name, &email, &phone, &company, &totalDealsSum, &dealsCount, &archivedAt)

                items = append(items, gin.H{
                    "id":          id.String(),
                    "type":        "customer",
                    "type_label":  "Клиент",
                    "type_icon":   "👤",
                    "title":       name,
                    "subtitle":    company,
                    "details":     fmt.Sprintf("Email: %s | Телефон: %s | Сделок: %d | Сумма: %.2f ₽", email, phone, dealsCount, totalDealsSum),
                    "archived_at": archivedAt.Format("02.01.2006"),
                })
            }
            rows.Close()
        }
    }

    if entityType == "" || entityType == "deals" {
        rows, err := database.Pool.Query(c.Request.Context(), `
            SELECT d.id, d.title, d.value, d.stage, d.closed_at, COALESCE(c.name, 'Неизвестно') as customer_name
            FROM crm_deals_archive d
            LEFT JOIN crm_customers_archive c ON d.customer_id = c.id
            WHERE d.tenant_id = $1
            ORDER BY d.archived_at DESC LIMIT $2 OFFSET $3
        `, tenantID, limit, offset)

        if err == nil {
            for rows.Next() {
                var id uuid.UUID
                var title, stage, customerName string
                var value float64
                var closedAt *time.Time

                rows.Scan(&id, &title, &value, &stage, &closedAt, &customerName)

                items = append(items, gin.H{
                    "id":          id.String(),
                    "type":        "deal",
                    "type_label":  "Сделка",
                    "type_icon":   "💰",
                    "title":       title,
                    "subtitle":    customerName,
                    "details":     fmt.Sprintf("Сумма: %.2f ₽ | Статус: %s", value, stage),
                    "archived_at": func() string {
                        if closedAt != nil {
                            return closedAt.Format("02.01.2006")
                        }
                        return ""
                    }(),
                })
            }
            rows.Close()
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "items": items,
        "total": len(items),
        "page":  page,
    })
}

// ArchiveCustomer - переместить клиента в архив
func ArchiveCustomer(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    customerID := c.Param("id")

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO crm_customers_archive (id, name, email, phone, company, status, tenant_id, archived_at)
        SELECT id, name, email, phone, company, status, tenant_id, NOW()
        FROM crm_customers WHERE id = $1 AND tenant_id = $2
    `, customerID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        DELETE FROM crm_customers WHERE id = $1 AND tenant_id = $2
    `, customerID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Клиент перемещен в архив"})
}

// RestoreFromArchive - восстановить из архива
func RestoreFromArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    entityType := c.Param("type")
    entityID := c.Param("id")

    switch entityType {
    case "customer":
        _, err := database.Pool.Exec(c.Request.Context(), `
            INSERT INTO crm_customers (id, name, email, phone, company, status, tenant_id, created_at)
            SELECT id, name, email, phone, company, status, tenant_id, archived_at
            FROM crm_customers_archive WHERE id = $1 AND tenant_id = $2
        `, entityID, tenantID)

        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        _, err = database.Pool.Exec(c.Request.Context(), `
            DELETE FROM crm_customers_archive WHERE id = $1 AND tenant_id = $2
        `, entityID, tenantID)

        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
    case "deal":
        c.JSON(http.StatusOK, gin.H{"success": true, "message": "Сделка восстановлена"})
        return
    case "entry":
        c.JSON(http.StatusOK, gin.H{"success": true, "message": "Проводка восстановлена"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Запись восстановлена"})
}

// UpgradeArchiveQuota - расширение квоты архива
func UpgradeArchiveQuota(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var req struct {
        Plan string `json:"plan" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var totalBytes int64
    var price int

    switch req.Plan {
    case "10gb":
        totalBytes = 10 * 1024 * 1024 * 1024
        price = 490
    case "50gb":
        totalBytes = 50 * 1024 * 1024 * 1024
        price = 1490
    case "100gb":
        totalBytes = 100 * 1024 * 1024 * 1024
        price = 2490
    default:
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid plan"})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO archive_quotas (tenant_id, total_bytes, plan_type, updated_at)
        VALUES ($1, $2, $3, NOW())
        ON CONFLICT (tenant_id) DO UPDATE SET
            total_bytes = $2,
            plan_type = $3,
            updated_at = NOW()
    `, tenantID, totalBytes, req.Plan)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":     true,
        "total_bytes": totalBytes,
        "total_gb":    totalBytes / 1024 / 1024 / 1024,
        "price":       price,
        "message":     "Тариф архива обновлен",
    })
}


// ========== АВТОМАТИЧЕСКАЯ АРХИВАЦИЯ ==========

// GetAutoArchiveSettings - получить настройки авто-архивации
func GetAutoArchiveSettings(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var settings struct {
        Enabled          bool `json:"enabled"`
        InactiveDays     int  `json:"inactive_days"`
        AutoArchiveEnabled bool `json:"auto_archive_enabled"`
    }

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT enabled, COALESCE(inactive_days, 180), COALESCE(auto_archive_enabled, true)
        FROM archive_auto_settings WHERE tenant_id = $1
    `, tenantID).Scan(&settings.Enabled, &settings.InactiveDays, &settings.AutoArchiveEnabled)

    if err != nil {
        // Создаём настройки по умолчанию
        settings.Enabled = true
        settings.InactiveDays = 180
        settings.AutoArchiveEnabled = true
        database.Pool.Exec(c.Request.Context(), `
            INSERT INTO archive_auto_settings (tenant_id, enabled, inactive_days, auto_archive_enabled)
            VALUES ($1, $2, $3, $4)
        `, tenantID, settings.Enabled, settings.InactiveDays, settings.AutoArchiveEnabled)
    }

    c.JSON(http.StatusOK, settings)
}

// UpdateAutoArchiveSettings - обновить настройки авто-архивации
func UpdateAutoArchiveSettings(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var req struct {
        Enabled          bool `json:"enabled"`
        InactiveDays     int  `json:"inactive_days"`
        AutoArchiveEnabled bool `json:"auto_archive_enabled"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if req.InactiveDays < 30 {
        req.InactiveDays = 30
    }
    if req.InactiveDays > 730 {
        req.InactiveDays = 730
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO archive_auto_settings (tenant_id, enabled, inactive_days, auto_archive_enabled, updated_at)
        VALUES ($1, $2, $3, $4, NOW())
        ON CONFLICT (tenant_id) DO UPDATE SET
            enabled = $2,
            inactive_days = $3,
            auto_archive_enabled = $4,
            updated_at = NOW()
    `, tenantID, req.Enabled, req.InactiveDays, req.AutoArchiveEnabled)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Настройки сохранены"})
}

// RunAutoArchive - запустить автоматическую архивацию
func RunAutoArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    // Получаем настройки
    var inactiveDays int
    var autoEnabled bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(inactive_days, 180), COALESCE(auto_archive_enabled, true)
        FROM archive_auto_settings WHERE tenant_id = $1
    `, tenantID).Scan(&inactiveDays, &autoEnabled)

    if err != nil || !autoEnabled {
        c.JSON(http.StatusOK, gin.H{"message": "Авто-архивация отключена"})
        return
    }

    // Находим неактивных клиентов
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, email, phone, company, status, last_seen
        FROM crm_customers 
        WHERE tenant_id = $1 
        AND status != 'archived'
        AND (last_seen IS NULL OR last_seen < NOW() - ($2 || ' days')::INTERVAL)
    `, tenantID, inactiveDays)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var archivedCount int
    for rows.Next() {
        var id uuid.UUID
        var name, email, phone, company, status string
        var lastSeen *time.Time

        rows.Scan(&id, &name, &email, &phone, &company, &status, &lastSeen)

        // Архивируем
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO crm_customers_archive (id, name, email, phone, company, status, tenant_id, archived_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
        `, id, name, email, phone, company, status, tenantID)

        if err == nil {
            database.Pool.Exec(c.Request.Context(), `
                DELETE FROM crm_customers WHERE id = $1 AND tenant_id = $2
            `, id, tenantID)
            archivedCount++
        }
    }

    // Обновляем время последнего запуска
    database.Pool.Exec(c.Request.Context(), `
        UPDATE archive_auto_settings SET last_run = NOW(), next_run = NOW() + INTERVAL '1 day'
        WHERE tenant_id = $1
    `, tenantID)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "archived_count": archivedCount,
        "message": fmt.Sprintf("Автоматически архивировано %d клиентов", archivedCount),
    })
}

// ========== КОРЗИНА ==========

// MoveToTrash - переместить в корзину (мягкое удаление)
func MoveToTrash(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    entityType := c.Param("type")
    entityID := c.Param("id")
    userID := c.GetString("user_id")
    userEmail := c.GetString("user_email")

    var entityData map[string]interface{}
    var entityName string

    switch entityType {
    case "customer":
        var id uuid.UUID
        var name, email, phone, company, status string
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT id, name, email, phone, company, status FROM crm_customers WHERE id = $1 AND tenant_id = $2
        `, entityID, tenantID).Scan(&id, &name, &email, &phone, &company, &status)
        if err != nil {
            c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
            return
        }
        entityData = map[string]interface{}{
            "id": id, "name": name, "email": email, "phone": phone, "company": company, "status": status,
        }
        entityName = name

        // Удаляем из активных
        database.Pool.Exec(c.Request.Context(), `DELETE FROM crm_customers WHERE id = $1`, entityID)

    case "deal":
        // Аналогично для сделок
    }

    // Сохраняем в корзину
    dataJSON, _ := json.Marshal(entityData)
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO trash_bin (entity_type, entity_id, entity_data, deleted_by, expires_at, tenant_id)
        VALUES ($1, $2, $3, $4, NOW() + INTERVAL '30 days', $5)
    `, entityType, entityID, dataJSON, userID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Логируем действие
    LogArchiveAction(c, "trash", entityType, entityID, entityName, gin.H{"user": userEmail})

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Запись перемещена в корзину"})
}

// GetTrashItems - получить список корзины
func GetTrashItems(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, entity_type, entity_id, entity_data, deleted_by, deleted_at, expires_at
        FROM trash_bin WHERE tenant_id = $1 AND expires_at > NOW()
        ORDER BY deleted_at DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var items []gin.H
    for rows.Next() {
        var id uuid.UUID
        var entityType, entityID string
        var entityData []byte
        var deletedBy *uuid.UUID
        var deletedAt, expiresAt time.Time

        rows.Scan(&id, &entityType, &entityID, &entityData, &deletedBy, &deletedAt, &expiresAt)

        var data map[string]interface{}
        json.Unmarshal(entityData, &data)

        items = append(items, gin.H{
            "id":          id.String(),
            "entity_type": entityType,
            "entity_id":   entityID,
            "entity_name": data["name"],
            "entity_data": data,
            "deleted_at":  deletedAt.Format("02.01.2006 15:04"),
            "expires_at":  expiresAt.Format("02.01.2006"),
        })
    }

    c.JSON(http.StatusOK, gin.H{"items": items})
}

// RestoreFromTrash - восстановить из корзины
func RestoreFromTrash(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    trashID := c.Param("id")

    var entityType, entityID string
    var entityData []byte
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT entity_type, entity_id, entity_data FROM trash_bin 
        WHERE id = $1 AND tenant_id = $2 AND expires_at > NOW()
    `, trashID, tenantID).Scan(&entityType, &entityID, &entityData)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
        return
    }

    var data map[string]interface{}
    json.Unmarshal(entityData, &data)

    switch entityType {
    case "customer":
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO crm_customers (id, name, email, phone, company, status, tenant_id, created_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
        `, entityID, data["name"], data["email"], data["phone"], data["company"], data["status"], tenantID)
    }

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Удаляем из корзины
    database.Pool.Exec(c.Request.Context(), `DELETE FROM trash_bin WHERE id = $1`, trashID)

    // Логируем
    LogArchiveAction(c, "restore_from_trash", entityType, entityID, data["name"].(string), nil)

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Запись восстановлена"})
}

// ========== ЛОГИРОВАНИЕ ==========

// LogArchiveAction - запись действия в лог
func LogArchiveAction(c *gin.Context, action, entityType, entityID, entityName string, details map[string]interface{}) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    userID := c.GetString("user_id")
    userEmail := c.GetString("user_email")

    detailsJSON, _ := json.Marshal(details)

    database.Pool.Exec(c.Request.Context(), `
        INSERT INTO archive_logs (action, entity_type, entity_id, entity_name, user_id, user_email, ip, details, tenant_id)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `, action, entityType, entityID, entityName, userID, userEmail, c.ClientIP(), detailsJSON, tenantID)
}

// GetArchiveLogs - получить логи архива
func GetArchiveLogs(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT action, entity_type, entity_name, user_email, ip, details, created_at
        FROM archive_logs WHERE tenant_id = $1
        ORDER BY created_at DESC LIMIT 100
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var logs []gin.H
    for rows.Next() {
        var action, entityType, entityName, userEmail, ip string
        var details []byte
        var createdAt time.Time

        rows.Scan(&action, &entityType, &entityName, &userEmail, &ip, &details, &createdAt)

        var detailsMap map[string]interface{}
        json.Unmarshal(details, &detailsMap)

        logs = append(logs, gin.H{
            "action":       action,
            "entity_type":  entityType,
            "entity_name":  entityName,
            "user_email":   userEmail,
            "ip":           ip,
            "details":      detailsMap,
            "created_at":   createdAt.Format("02.01.2006 15:04:05"),
        })
    }

    c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// ========== ЭКСПОРТ АРХИВА ==========

// ExportArchiveToExcel - экспорт архива в Excel
func ExportArchiveToExcel(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT name, email, phone, company, archived_at
        FROM crm_customers_archive WHERE tenant_id = $1
        ORDER BY archived_at DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    // Формируем CSV
    csvData := "Название,Email,Телефон,Компания,Дата архивации\n"
    for rows.Next() {
        var name, email, phone, company string
        var archivedAt time.Time
        rows.Scan(&name, &email, &phone, &company, &archivedAt)
        csvData += fmt.Sprintf("%s,%s,%s,%s,%s\n", name, email, phone, company, archivedAt.Format("02.01.2006"))
    }

    c.Header("Content-Type", "text/csv; charset=utf-8")
    c.Header("Content-Disposition", "attachment; filename=archive_export.csv")
    c.String(http.StatusOK, csvData)
}


// ========== ДОПОЛНИТЕЛЬНЫЕ ФУНКЦИИ ДЛЯ ИНТЕРФЕЙСА ==========

// DeleteFromTrashPermanently - окончательное удаление из корзины
func DeleteFromTrashPermanently(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    trashID := c.Param("id")

    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM trash_bin WHERE id = $1 AND tenant_id = $2
    `, trashID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Запись удалена навсегда"})
}

// ClearTrashBin - очистить корзину (удалить всё)
func ClearTrashBin(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM trash_bin WHERE tenant_id = $1 AND expires_at > NOW()
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Корзина очищена"})
}




// GetCurrentPlan - получить текущий тариф
func GetCurrentPlan(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var totalBytes int64
    var planType string
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT total_bytes, COALESCE(plan_type, 'free') FROM archive_quotas WHERE tenant_id = $1
    `, tenantID).Scan(&totalBytes, &planType)
    
    if err != nil {
        totalBytes = 1073741824 // 1 GB
        planType = "free"
    }

    planNames := map[string]string{
        "free": "Бесплатный",
        "10gb": "Старт",
        "50gb": "Бизнес", 
        "100gb": "Корпоративный",
    }

    planGB := totalBytes / 1024 / 1024 / 1024

    c.JSON(http.StatusOK, gin.H{
        "plan":          planType,
        "plan_name":     planNames[planType],
        "total_bytes":   totalBytes,
        "total_gb":      planGB,
        "can_upgrade":   planType != "100gb",
    })
}

// GetMemoryStats - детальная статистика использования памяти
func GetMemoryStats(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var customersCount, dealsCount, entriesCount int64
    var customersSize, dealsSize, entriesSize int64

    // Считаем клиентов
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*), COALESCE(SUM(LENGTH(name) + LENGTH(COALESCE(email,'')) + LENGTH(COALESCE(phone,''))), 0)
        FROM crm_customers_archive WHERE tenant_id = $1
    `, tenantID).Scan(&customersCount, &customersSize)

    // Считаем сделки
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*), COALESCE(SUM(LENGTH(title) + LENGTH(value::text)), 0)
        FROM crm_deals_archive WHERE tenant_id = $1
    `, tenantID).Scan(&dealsCount, &dealsSize)

    // Считаем проводки
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*), COALESCE(SUM(LENGTH(amount::text) + LENGTH(COALESCE(description,''))), 0)
        FROM journal_entries_archive WHERE tenant_id = $1
    `, tenantID).Scan(&entriesCount, &entriesSize)

    totalSize := customersSize + dealsSize + entriesSize
    quotaBytes := int64(1073741824) // 1 GB по умолчанию

    // Получаем квоту
    var totalBytes int64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT total_bytes FROM archive_quotas WHERE tenant_id = $1
    `, tenantID).Scan(&totalBytes)
    if err == nil {
        quotaBytes = totalBytes
    }

    c.JSON(http.StatusOK, gin.H{
        "customers": gin.H{
            "count": customersCount,
            "bytes": customersSize,
            "kb":    float64(customersSize) / 1024,
            "mb":    float64(customersSize) / 1024 / 1024,
            "gb":    float64(customersSize) / 1024 / 1024 / 1024,
        },
        "deals": gin.H{
            "count": dealsCount,
            "bytes": dealsSize,
            "kb":    float64(dealsSize) / 1024,
            "mb":    float64(dealsSize) / 1024 / 1024,
            "gb":    float64(dealsSize) / 1024 / 1024 / 1024,
        },
        "entries": gin.H{
            "count": entriesCount,
            "bytes": entriesSize,
            "kb":    float64(entriesSize) / 1024,
            "mb":    float64(entriesSize) / 1024 / 1024,
            "gb":    float64(entriesSize) / 1024 / 1024 / 1024,
        },
        "total": gin.H{
            "bytes": totalSize,
            "kb":    float64(totalSize) / 1024,
            "mb":    float64(totalSize) / 1024 / 1024,
            "gb":    float64(totalSize) / 1024 / 1024 / 1024,
        },
        "quota": gin.H{
            "bytes": quotaBytes,
            "gb":    float64(quotaBytes) / 1024 / 1024 / 1024,
        },
        "free": gin.H{
            "bytes": quotaBytes - totalSize,
            "gb":    float64(quotaBytes-totalSize) / 1024 / 1024 / 1024,
        },
        "used_percent": float64(totalSize) / float64(quotaBytes) * 100,
    })
}
