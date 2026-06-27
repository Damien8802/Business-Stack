package handlers

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)

// ========== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ==========

func getTenantIDForArchive(c *gin.Context) string {
    // ✅ ТОЛЬКО tenant_id из контекста
    return c.GetString("tenant_id")
}
func getDaysInArchive(archivedAt time.Time) int {
    return int(time.Now().Sub(archivedAt).Hours() / 24)
}

// ========== 1. АРХИВИРОВАНИЕ ==========

func ArchiveFincoreEntity(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - tenant not found"})
        return
    }

    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - user not found"})
        return
    }

    var req struct {
        EntityType string          `json:"entity_type" binding:"required"`
        EntityID   string          `json:"entity_id" binding:"required"`
        EntityData json.RawMessage `json:"entity_data" binding:"required"`
        Reason     string          `json:"reason"`
        Status     string          `json:"status"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO fincore_archive (id, tenant_id, entity_type, entity_id, entity_data, 
                                      archived_at, archived_by, archived_reason, original_status)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, NOW(), $5, $6, $7)
    `, tenantID, req.EntityType, req.EntityID, req.EntityData, userID, req.Reason, req.Status)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Объект перемещён в архив"})
}
// ========== 2. ПОЛУЧЕНИЕ СПИСКА С ФИЛЬТРАМИ ==========

func GetFincoreArchiveList(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - tenant not found"})
        return
    }
    // Параметры фильтрации
    entityType := c.DefaultQuery("entity_type", "all")
    status := c.DefaultQuery("status", "all")
    search := c.DefaultQuery("search", "")
    dateFrom := c.DefaultQuery("date_from", "")
    dateTo := c.DefaultQuery("date_to", "")
    daysFilter := c.DefaultQuery("days", "all")

    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
    offset := (page - 1) * limit

    // Строим WHERE условия
    var conditions []string
    var args []interface{}
    argCounter := 1

    conditions = append(conditions, "tenant_id = $"+strconv.Itoa(argCounter))
    args = append(args, tenantID)
    argCounter++

    if entityType != "all" {
        conditions = append(conditions, "entity_type = $"+strconv.Itoa(argCounter))
        args = append(args, entityType)
        argCounter++
    }

    if status != "all" {
        conditions = append(conditions, "original_status = $"+strconv.Itoa(argCounter))
        args = append(args, status)
        argCounter++
    }

    if search != "" {
        conditions = append(conditions, "entity_data::text ILIKE $"+strconv.Itoa(argCounter))
        args = append(args, "%"+search+"%")
        argCounter++
    }

    if dateFrom != "" {
        conditions = append(conditions, "archived_at >= $"+strconv.Itoa(argCounter))
        args = append(args, dateFrom)
        argCounter++
    }

    if dateTo != "" {
        conditions = append(conditions, "archived_at <= $"+strconv.Itoa(argCounter)+" + interval '1 day'")
        args = append(args, dateTo)
        argCounter++
    }

    // Фильтр по дням в архиве
    if daysFilter != "all" {
        switch daysFilter {
        case "0_7":
            conditions = append(conditions, "archived_at > NOW() - interval '7 days'")
        case "8_14":
            conditions = append(conditions, "archived_at <= NOW() - interval '7 days' AND archived_at > NOW() - interval '14 days'")
        case "15_21":
            conditions = append(conditions, "archived_at <= NOW() - interval '14 days' AND archived_at > NOW() - interval '21 days'")
        case "22_30":
            conditions = append(conditions, "archived_at <= NOW() - interval '21 days' AND archived_at > NOW() - interval '30 days'")
        }
    }

    whereClause := strings.Join(conditions, " AND ")

    // Подсчет общего количества
    var total int
    countQuery := "SELECT COUNT(*) FROM fincore_archive WHERE " + whereClause
    err := database.Pool.QueryRow(c.Request.Context(), countQuery, args...).Scan(&total)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Получение данных
    query := `
        SELECT id, entity_type, entity_id, entity_data, archived_at, archived_reason, original_status
        FROM fincore_archive
        WHERE ` + whereClause + `
        ORDER BY archived_at DESC
        LIMIT $` + strconv.Itoa(argCounter) + ` OFFSET $` + strconv.Itoa(argCounter+1)

    args = append(args, limit, offset)

    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var items []gin.H
    for rows.Next() {
        var id, entityID uuid.UUID
        var eType, reason, status string
        var entityDataJSON []byte
        var archivedAt time.Time

        err := rows.Scan(&id, &eType, &entityID, &entityDataJSON, &archivedAt, &reason, &status)
        if err != nil {
            continue
        }

        var entityData map[string]interface{}
        json.Unmarshal(entityDataJSON, &entityData)

        daysInArchive := getDaysInArchive(archivedAt)

        var daysCategory string
        if daysInArchive <= 7 {
            daysCategory = "safe"
        } else if daysInArchive <= 14 {
            daysCategory = "warning"
        } else {
            daysCategory = "danger"
        }

        // Извлекаем поля для таблицы
        date := ""
        if v, ok := entityData["operation_date"]; ok {
            date = v.(string)
        }
        documentNumber := ""
        if v, ok := entityData["document_number"]; ok {
            documentNumber = v.(string)
        }
        debitAccount := ""
        if v, ok := entityData["debit_account"]; ok {
            debitAccount = v.(string)
        }
        creditAccount := ""
        if v, ok := entityData["credit_account"]; ok {
            creditAccount = v.(string)
        }
        counterparty := ""
        if v, ok := entityData["counterparty_name"]; ok {
            counterparty = v.(string)
        }
        amount := 0.0
        if v, ok := entityData["amount"]; ok {
            amount = v.(float64)
        }
        description := ""
        if v, ok := entityData["description"]; ok {
            description = v.(string)
        }

        items = append(items, gin.H{
            "id":               id,
            "entity_type":      eType,
            "entity_id":        entityID,
            "entity_data":      entityData,
            "archived_at":      archivedAt.Format("2006-01-02"),
            "archived_at_full": archivedAt.Format("2006-01-02 15:04:05"),
            "reason":           reason,
            "original_status":  status,
            "days_in_archive":  daysInArchive,
            "days_category":    daysCategory,
            "date":             date,
            "document_number":  documentNumber,
            "debit_account":    debitAccount,
            "credit_account":   creditAccount,
            "counterparty":     counterparty,
            "amount":           amount,
            "description":      description,
            "status":           status,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    items,
        "total":   total,
        "page":    page,
        "limit":   limit,
    })
}

// ========== 3. ВОССТАНОВЛЕНИЕ ИЗ АРХИВА ==========

func RestoreFincoreFromArchive(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - tenant not found"})
        return
    }
    archiveID := c.Param("id")

    var entityType, entityID string
    var entityDataJSON []byte
    var archivedAt time.Time

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT entity_type, entity_id, entity_data, archived_at
        FROM fincore_archive
        WHERE id = $1 AND tenant_id = $2
    `, archiveID, tenantID).Scan(&entityType, &entityID, &entityDataJSON, &archivedAt)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
        return
    }

    daysInArchive := getDaysInArchive(archivedAt)
    if daysInArchive > 30 {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Срок хранения в архиве истёк (30 дней). Восстановление невозможно.",
        })
        return
    }

    switch entityType {
    case "journal_entry":
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO journal_entries (id, tenant_id, operation_date, document_number, document_type,
                                         counterparty_name, counterparty_inn, debit_account, credit_account,
                                         amount, description, status, created_at, updated_at)
            SELECT $1::UUID, $2, 
                   (entity_data->>'operation_date')::DATE,
                   entity_data->>'document_number',
                   entity_data->>'document_type',
                   entity_data->>'counterparty_name',
                   entity_data->>'counterparty_inn',
                   entity_data->>'debit_account',
                   entity_data->>'credit_account',
                   (entity_data->>'amount')::DECIMAL,
                   entity_data->>'description',
                   entity_data->>'status',
                   NOW(), NOW()
            FROM fincore_archive
            WHERE id = $3 AND tenant_id = $2
        `, entityID, tenantID, archiveID)
    default:
        c.JSON(http.StatusBadRequest, gin.H{"error": "Восстановление для этого типа пока не реализовано"})
        return
    }

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        DELETE FROM fincore_archive WHERE id = $1 AND tenant_id = $2
    `, archiveID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Объект восстановлен"})
}

// ========== 4. УДАЛЕНИЕ НАВСЕГДА ==========

func PermanentDeleteFincoreFromArchive(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - tenant not found"})
        return
    }
    archiveID := c.Param("id")

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM fincore_archive WHERE id = $1 AND tenant_id = $2
    `, archiveID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    rows := result.RowsAffected()
    if rows == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Объект удалён навсегда"})
}

// ========== 5. СТАТИСТИКА АРХИВА ==========

func GetFincoreArchiveStats(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - tenant not found"})
        return
    }
    var totalCount, draftCount, postedCount int
    var safeCount, warningCount, dangerCount int
    var days0_7, days8_14, days15_21, days22_30 int

    // Общее количество
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive WHERE tenant_id = $1
    `, tenantID).Scan(&totalCount)

    // По статусам
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive WHERE tenant_id = $1 AND original_status = 'draft'
    `, tenantID).Scan(&draftCount)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive WHERE tenant_id = $1 AND original_status = 'posted'
    `, tenantID).Scan(&postedCount)

    // ===== ПРАВИЛЬНАЯ ЛОГИКА ДЛЯ СЧЕТЧИКОВ =====
    // Безопасные: 0-14 дней (архивировано не более 14 дней назад)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive 
        WHERE tenant_id = $1 AND archived_at > NOW() - interval '14 days'
    `, tenantID).Scan(&safeCount)

    // Истекают: 15-21 дней (архивировано от 14 до 21 дней назад)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive 
        WHERE tenant_id = $1 AND archived_at <= NOW() - interval '14 days' 
        AND archived_at > NOW() - interval '21 days'
    `, tenantID).Scan(&warningCount)

    // Просрочены: >21 дней (архивировано более 21 дня назад)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive 
        WHERE tenant_id = $1 AND archived_at <= NOW() - interval '21 days'
    `, tenantID).Scan(&dangerCount)

    // ===== ДЕТАЛИЗАЦИЯ ПО ДНЯМ =====
    // 0-7 дней
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive 
        WHERE tenant_id = $1 AND archived_at > NOW() - interval '7 days'
    `, tenantID).Scan(&days0_7)

    // 8-14 дней
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive 
        WHERE tenant_id = $1 AND archived_at <= NOW() - interval '7 days' 
        AND archived_at > NOW() - interval '14 days'
    `, tenantID).Scan(&days8_14)

    // 15-21 дней
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive 
        WHERE tenant_id = $1 AND archived_at <= NOW() - interval '14 days' 
        AND archived_at > NOW() - interval '21 days'
    `, tenantID).Scan(&days15_21)

    // 22-30 дней
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive 
        WHERE tenant_id = $1 AND archived_at <= NOW() - interval '21 days' 
        AND archived_at > NOW() - interval '30 days'
    `, tenantID).Scan(&days22_30)

    // Статистика по типам
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT entity_type, COUNT(*) as count
        FROM fincore_archive
        WHERE tenant_id = $1
        GROUP BY entity_type
    `, tenantID)

    var stats []gin.H
    if err == nil {
        defer rows.Close()
        for rows.Next() {
            var eType string
            var count int
            rows.Scan(&eType, &count)
            stats = append(stats, gin.H{
                "type":  eType,
                "count": count,
            })
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "total_count":   totalCount,
        "draft_count":   draftCount,
        "posted_count":  postedCount,
        "safe_count":    safeCount,    // 0-14 дней
        "warning_count": warningCount, // 15-21 дней
        "danger_count":  dangerCount,  // >21 дней
        "days0_7":       days0_7,
        "days8_14":      days8_14,
        "days15_21":     days15_21,
        "days22_30":     days22_30,
        "statistics":    stats,
    })
}
// ========== 6. ОЧИСТКА ВСЕГО АРХИВА ==========

func ClearAllFincoreArchive(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - tenant not found"})
        return
    }
    var count int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive WHERE tenant_id = $1
    `, tenantID).Scan(&count)

    if count == 0 {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "message": "Архив уже пуст",
            "deleted": 0,
        })
        return
    }

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM fincore_archive WHERE tenant_id = $1
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    rows := result.RowsAffected()
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Архив полностью очищен",
        "deleted": rows,
    })
}

// ========== 7. АВТООЧИСТКА АРХИВА ==========

func AutoCleanFincoreArchive() {
    result, err := database.Pool.Exec(context.Background(), `
        DELETE FROM fincore_archive 
        WHERE archived_at < NOW() - interval '30 days'
    `)

    if err != nil {
        log.Printf("❌ Ошибка автоочистки архива: %v", err)
        return
    }

    rows := result.RowsAffected()
    if rows > 0 {
        log.Printf("🧹 Автоочистка архива: удалено %d записей (старше 30 дней)", rows)
    }
}

// ========== 8. МАССОВОЕ ВОССТАНОВЛЕНИЕ ==========

func MassRestoreFromArchive(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - tenant not found"})
        return
    }
    var req struct {
        IDs []string `json:"ids" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    successCount := 0
    failCount := 0

    for _, id := range req.IDs {
        var entityType, entityID string
        var entityDataJSON []byte
        var archivedAt time.Time

        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT entity_type, entity_id, entity_data, archived_at
            FROM fincore_archive
            WHERE id = $1 AND tenant_id = $2
        `, id, tenantID).Scan(&entityType, &entityID, &entityDataJSON, &archivedAt)

        if err != nil {
            failCount++
            continue
        }

        daysInArchive := getDaysInArchive(archivedAt)
        if daysInArchive > 30 {
            failCount++
            continue
        }

        switch entityType {
        case "journal_entry":
            _, err = database.Pool.Exec(c.Request.Context(), `
                INSERT INTO journal_entries (id, tenant_id, operation_date, document_number, document_type,
                                             counterparty_name, counterparty_inn, debit_account, credit_account,
                                             amount, description, status, created_at, updated_at)
                SELECT $1::UUID, $2, 
                       (entity_data->>'operation_date')::DATE,
                       entity_data->>'document_number',
                       entity_data->>'document_type',
                       entity_data->>'counterparty_name',
                       entity_data->>'counterparty_inn',
                       entity_data->>'debit_account',
                       entity_data->>'credit_account',
                       (entity_data->>'amount')::DECIMAL,
                       entity_data->>'description',
                       entity_data->>'status',
                       NOW(), NOW()
                FROM fincore_archive
                WHERE id = $3 AND tenant_id = $2
            `, entityID, tenantID, id)
        default:
            failCount++
            continue
        }

        if err != nil {
            failCount++
            continue
        }

        _, err = database.Pool.Exec(c.Request.Context(), `
            DELETE FROM fincore_archive WHERE id = $1 AND tenant_id = $2
        `, id, tenantID)

        if err != nil {
            failCount++
            continue
        }

        successCount++
    }

    c.JSON(http.StatusOK, gin.H{
        "success":         true,
        "restored_count":  successCount,
        "failed_count":    failCount,
        "message":         fmt.Sprintf("Восстановлено: %d, Ошибок: %d", successCount, failCount),
    })
}

// ========== 9. МАССОВОЕ УДАЛЕНИЕ ==========

func MassPermanentDeleteFromArchive(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized - tenant not found"})
        return
    }
    var req struct {
        IDs []string `json:"ids" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    successCount := 0
    failCount := 0

    for _, id := range req.IDs {
        result, err := database.Pool.Exec(c.Request.Context(), `
            DELETE FROM fincore_archive WHERE id = $1 AND tenant_id = $2
        `, id, tenantID)

        if err != nil {
            failCount++
            continue
        }

        rows := result.RowsAffected()
        if rows > 0 {
            successCount++
        } else {
            failCount++
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "deleted_count": successCount,
        "failed_count":  failCount,
        "message":       fmt.Sprintf("Удалено: %d, Ошибок: %d", successCount, failCount),
    })
}