package handlers

import (
    "encoding/json"
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)

// getTenantIDForArchive - получение tenant_id из контекста
func getTenantIDForArchive(c *gin.Context) string {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    return tenantID
}

// ArchiveFincoreEntity - универсальное архивирование
func ArchiveFincoreEntity(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
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

    userID := c.GetString("user_id")

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

// GetFincoreArchiveList - получить список архивированных объектов
func GetFincoreArchiveList(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    entityType := c.DefaultQuery("entity_type", "all")
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
    offset := (page - 1) * limit

    var total int
    var rows interface{}

    if entityType == "all" {
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT COUNT(*) FROM fincore_archive WHERE tenant_id = $1
        `, tenantID).Scan(&total)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        rows, err = database.Pool.Query(c.Request.Context(), `
            SELECT id, entity_type, entity_id, entity_data, archived_at, archived_reason, original_status
            FROM fincore_archive
            WHERE tenant_id = $1
            ORDER BY archived_at DESC
            LIMIT $2 OFFSET $3
        `, tenantID, limit, offset)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
    } else {
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT COUNT(*) FROM fincore_archive WHERE tenant_id = $1 AND entity_type = $2
        `, tenantID, entityType).Scan(&total)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        rows, err = database.Pool.Query(c.Request.Context(), `
            SELECT id, entity_type, entity_id, entity_data, archived_at, archived_reason, original_status
            FROM fincore_archive
            WHERE tenant_id = $1 AND entity_type = $2
            ORDER BY archived_at DESC
            LIMIT $3 OFFSET $4
        `, tenantID, entityType, limit, offset)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
    }

    defer rows.(interface{ Close() error }).Close()

    var items []gin.H
    for rows.(interface{ Next() bool }).Next() {
        var id, entityID uuid.UUID
        var eType, reason, status string
        var entityDataJSON []byte
        var archivedAt time.Time

        err := rows.(interface {
            Scan(...interface{}) error
        }).Scan(&id, &eType, &entityID, &entityDataJSON, &archivedAt, &reason, &status)
        if err != nil {
            continue
        }

        var entityData map[string]interface{}
        json.Unmarshal(entityDataJSON, &entityData)

        items = append(items, gin.H{
            "id":              id,
            "entity_type":     eType,
            "entity_id":       entityID,
            "entity_data":     entityData,
            "archived_at":     archivedAt.Format("2006-01-02 15:04:05"),
            "reason":          reason,
            "original_status": status,
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

// RestoreFincoreFromArchive - восстановить из архива
func RestoreFincoreFromArchive(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    archiveID := c.Param("id")

    var entityType, entityID string
    var entityDataJSON []byte

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT entity_type, entity_id, entity_data
        FROM fincore_archive
        WHERE id = $1 AND tenant_id = $2
    `, archiveID, tenantID).Scan(&entityType, &entityID, &entityDataJSON)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
        return
    }

    switch entityType {
    case "journal_entry":
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO journal_entries (id, tenant_id, operation_date, document_number, document_type,
                                         counterparty_name, counterparty_inn, debit_account, credit_account,
                                         amount, description, status, created_at, updated_at)
            SELECT $1, $2, 
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
            WHERE id = $3
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

// PermanentDeleteFincoreFromArchive - удалить навсегда
func PermanentDeleteFincoreFromArchive(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
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

    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Объект удалён навсегда"})
}

// GetFincoreArchiveStats - статистика архива
func GetFincoreArchiveStats(c *gin.Context) {
    tenantID := getTenantIDForArchive(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var totalCount int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_archive WHERE tenant_id = $1
    `, tenantID).Scan(&totalCount)

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
        "success":     true,
        "total_count": totalCount,
        "statistics":  stats,
    })
}