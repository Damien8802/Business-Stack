package handlers

import (
    "fmt"
    "log"
    "net/http"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)

// GetFincoreTags - получить все теги
func GetFincoreTags(c *gin.Context) {
    log.Println("🔍 === GetFincoreTags START ===")
    
    // Пробуем получить tenant_id из разных мест
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        // Пробуем из заголовка
        tenantID = c.GetHeader("X-Tenant-ID")
    }
    
    log.Printf("📌 tenant_id = '%s'", tenantID)
    
    if tenantID == "" {
        log.Println("❌ tenant_id not found - возвращаем пустой массив")
        c.JSON(http.StatusOK, []gin.H{})
        return
    }
    
    // ✅ ПРЯМОЙ ЗАПРОС - БЕЗ УСЛОВИЯ is_active (проверим все теги)
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, type, color, is_active, created_at
        FROM fincore_analytics_tags
        WHERE tenant_id = $1
        ORDER BY name
    `, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка запроса: %v", err)
        c.JSON(http.StatusOK, []gin.H{})
        return
    }
    defer rows.Close()
    
    var tags []gin.H
    count := 0
    
    for rows.Next() {
        count++
        var id, name, tagType, color string
        var isActive bool
        var createdAt time.Time
        
        err := rows.Scan(&id, &name, &tagType, &color, &isActive, &createdAt)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
            continue
        }
        
        tags = append(tags, gin.H{
            "id":         id,
            "name":       name,
            "type":       tagType,
            "color":      color,
            "is_active":  isActive,
            "created_at": createdAt,
        })
    }
    
    log.Printf("✅ Найдено тегов: %d", len(tags))
    
    if len(tags) == 0 {
        // Проверяем, есть ли теги вообще в таблице
        var total int
        database.Pool.QueryRow(c.Request.Context(), 
            "SELECT COUNT(*) FROM fincore_analytics_tags").Scan(&total)
        log.Printf("📊 Всего тегов в таблице: %d", total)
        
        // Проверяем, какие tenant_id есть
        rows2, _ := database.Pool.Query(c.Request.Context(), 
            "SELECT DISTINCT tenant_id, COUNT(*) FROM fincore_analytics_tags GROUP BY tenant_id")
        defer rows2.Close()
        for rows2.Next() {
            var tid string
            var cnt int
            rows2.Scan(&tid, &cnt)
            log.Printf("📊 tenant_id=%s имеет %d тегов", tid, cnt)
        }
    }
    
    c.JSON(http.StatusOK, tags)
}
// CreateFincoreTag - создать тег
func CreateFincoreTag(c *gin.Context) {
    log.Println("🔍 === CreateFincoreTag START ===")
    
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    
    if tenantID == "" {
        log.Println("❌ tenant_id not found")
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    log.Printf("📌 tenant_id = %s", tenantID)
    
    var req struct {
        Name  string `json:"name"`
        Type  string `json:"type"`
        Color string `json:"color"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    log.Printf("📌 Создаем тег: name=%s, type=%s, color=%s", req.Name, req.Type, req.Color)
    
    if req.Name == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Название тега обязательно"})
        return
    }
    
    if req.Type == "" {
        req.Type = "custom"
    }
    
    if req.Color == "" {
        req.Color = "#667eea"
    }
    
    var id uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO fincore_analytics_tags (tenant_id, name, type, color, is_active, created_at)
        VALUES ($1, $2, $3, $4, true, NOW())
        RETURNING id
    `, tenantID, req.Name, req.Type, req.Color).Scan(&id)
    
    if err != nil {
        log.Printf("❌ Ошибка вставки: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    log.Printf("✅ Тег создан с ID: %s", id.String())
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "id":      id.String(),
        "message": "Тег успешно создан",
    })
}
// UpdateFincoreTag - обновить тег
func UpdateFincoreTag(c *gin.Context) {
    tagID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    var req struct {
        Name     string `json:"name"`
        Color    string `json:"color"`
        IsActive bool   `json:"is_active"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE fincore_analytics_tags
        SET name = COALESCE(NULLIF($1, ''), name),
            color = COALESCE(NULLIF($2, ''), color),
            is_active = $3,
            updated_at = NOW()
        WHERE id = $4 AND tenant_id = $5
    `, req.Name, req.Color, req.IsActive, tagID, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Тег обновлён"})
}

// DeleteFincoreTag - удалить тег
func DeleteFincoreTag(c *gin.Context) {
    tagID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM fincore_analytics_tags
        WHERE id = $1 AND tenant_id = $2
    `, tagID, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    rowsAffected := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Тег не найден"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Тег удалён"})
}

// GetFincoreReportByTag - отчёт по тегам

// AssignTagToEntry - привязать ОДИН тег к проводке
func AssignTagToEntry(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    var req struct {
        EntryID string `json:"entry_id"`
        TagID   string `json:"tag_id"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO journal_entry_tags (entry_id, tag_id)
        VALUES ($1, $2)
        ON CONFLICT DO NOTHING
    `, req.EntryID, req.TagID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Тег привязан к проводке"})
}

// RemoveTagFromEntry - отвязать тег от проводки
func RemoveTagFromEntry(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    entryID := c.Param("entry_id")
    tagID := c.Param("tag_id")
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM journal_entry_tags
        WHERE entry_id = $1 AND tag_id = $2
    `, entryID, tagID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Тег отвязан от проводки"})
}

// ExportFincoreReport - экспорт отчёта в Excel
func ExportFincoreReport(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    startDate := c.Query("start_date")
    endDate := c.Query("end_date")
    
    if startDate == "" {
        startDate = time.Now().AddDate(0, -1, 0).Format("2006-01-01")
    }
    if endDate == "" {
        endDate = time.Now().Format("2006-01-02")
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            t.name as tag_name,
            t.type as tag_type,
            COALESCE(SUM(j.debit_amount), 0) as total_debit,
            COALESCE(SUM(j.credit_amount), 0) as total_credit,
            COALESCE(SUM(j.debit_amount - j.credit_amount), 0) as balance
        FROM fincore_analytics_tags t
        LEFT JOIN journal_entry_tags jet ON jet.tag_id = t.id
        LEFT JOIN journal_entries j ON jet.entry_id = j.id AND j.operation_date BETWEEN $2 AND $3
        WHERE t.tenant_id = $1 AND t.is_active = true
        GROUP BY t.id, t.name, t.type
        ORDER BY balance DESC
    `, tenantID, startDate, endDate)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    html := `<html><head><meta charset="UTF-8"><title>Управленческий отчёт</title></head><body>
    <h2>Отчёт по аналитике</h2>
    <p>Период: ` + startDate + ` - ` + endDate + `</p>
    <table border="1">
        <thead><tr><th>Тег</th><th>Тип</th><th>Дебет</th><th>Кредит</th><th>Сальдо</th></tr></thead><tbody>`
    
    for rows.Next() {
        var name, tagType string
        var debit, credit, balance float64
        rows.Scan(&name, &tagType, &debit, &credit, &balance)
        html += fmt.Sprintf("<tr><td>%s</td><td>%s</td><td align='right'>%.2f</td><td align='right'>%.2f</td><td align='right'>%.2f</td></tr>", 
            name, tagType, debit, credit, balance)
    }
    
    html += `</tbody></table><p>Сформировано: ` + time.Now().Format("2006-01-02 15:04:05") + `</p></body></html>`
    
    filename := fmt.Sprintf("fincore_report_%s_%s.xls", startDate, endDate)
    c.Header("Content-Type", "application/vnd.ms-excel")
    c.Header("Content-Disposition", "attachment; filename="+filename)
    c.String(http.StatusOK, html)
}

// GetTopTags - топ тегов по прибыли
func GetTopTags(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    limit := c.DefaultQuery("limit", "5")
    startDate := c.Query("start_date")
    endDate := c.Query("end_date")
    
    if startDate == "" {
        startDate = time.Now().AddDate(0, -1, 0).Format("2006-01-01")
    }
    if endDate == "" {
        endDate = time.Now().Format("2006-01-02")
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            t.name,
            t.color,
            COALESCE(SUM(j.debit_amount - j.credit_amount), 0) as balance
        FROM fincore_analytics_tags t
        LEFT JOIN journal_entry_tags jet ON jet.tag_id = t.id
        LEFT JOIN journal_entries j ON jet.entry_id = j.id AND j.operation_date BETWEEN $2 AND $3
        WHERE t.tenant_id = $1 AND t.is_active = true
        GROUP BY t.id, t.name, t.color
        ORDER BY balance DESC
        LIMIT $4
    `, tenantID, startDate, endDate, limit)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var tags []gin.H
    for rows.Next() {
        var name, color string
        var balance float64
        rows.Scan(&name, &color, &balance)
        tags = append(tags, gin.H{
            "name":    name,
            "color":   color,
            "balance": balance,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    tags,
    })
}

// AutoAssignTagToEntry - автоматически привязывает тег к проводке
func AutoAssignTagToEntry(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        return
    }
    
    var req struct {
        EntryID   string `json:"entry_id"`
        TagType   string `json:"tag_type"`
        TagName   string `json:"tag_name"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        return
    }
    
    var tagID string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id FROM fincore_analytics_tags
        WHERE tenant_id = $1 AND type = $2 AND name = $3 AND is_active = true
    `, tenantID, req.TagType, req.TagName).Scan(&tagID)
    
    if err != nil {
        err = database.Pool.QueryRow(c.Request.Context(), `
            INSERT INTO fincore_analytics_tags (tenant_id, name, type, color, created_at)
            VALUES ($1, $2, $3, '#667eea', NOW())
            RETURNING id
        `, tenantID, req.TagName, req.TagType).Scan(&tagID)
        if err != nil {
            return
        }
    }
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO journal_entry_tags (entry_id, tag_id)
        VALUES ($1, $2)
        ON CONFLICT DO NOTHING
    `, req.EntryID, tagID)
}

// GetEntryTags - получить теги для конкретной проводки
func GetEntryTags(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }

    entryID := c.Param("entryId")
    if entryID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "entryId required"})
        return
    }

    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM journal_entries WHERE id = $1 AND tenant_id = $2)
    `, entryID, tenantID).Scan(&exists)

    if err != nil || !exists {
        c.JSON(http.StatusNotFound, gin.H{"error": "Entry not found"})
        return
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT t.id, t.name, t.type, t.color
        FROM fincore_analytics_tags t
        JOIN journal_entry_tags jet ON jet.tag_id = t.id
        WHERE jet.entry_id = $1 AND t.tenant_id = $2 AND t.is_active = true
        ORDER BY t.name
    `, entryID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var tags []gin.H
    for rows.Next() {
        var id, name, tagType, color string

        err := rows.Scan(&id, &name, &tagType, &color)
        if err != nil {
            continue
        }

        tags = append(tags, gin.H{
            "id":    id,
            "name":  name,
            "type":  tagType,
            "color": color,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "tags":    tags,
        "count":   len(tags),
    })
}

// GetEntriesByTag - получить проводки по тегу (для фильтрации)
func GetEntriesByTag(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }

    tagID := c.Param("tagId")
    if tagID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tagId required"})
        return
    }

    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM fincore_analytics_tags WHERE id = $1 AND tenant_id = $2)
    `, tagID, tenantID).Scan(&exists)

    if err != nil || !exists {
        c.JSON(http.StatusNotFound, gin.H{"error": "Tag not found"})
        return
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT jet.entry_id
        FROM journal_entry_tags jet
        WHERE jet.tag_id = $1
    `, tagID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var entryIDs []string
    for rows.Next() {
        var id uuid.UUID
        rows.Scan(&id)
        entryIDs = append(entryIDs, id.String())
    }

    c.JSON(http.StatusOK, gin.H{
        "success":   true,
        "entry_ids": entryIDs,
        "count":     len(entryIDs),
    })
}

// GetFincoreReportByTagWithEntries - получить отчёт по тегу с проводками
func GetFincoreReportByTagWithEntries(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }

    tagID := c.Param("tagId")
    if tagID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tagId required"})
        return
    }

    startDate := c.Query("start_date")
    endDate := c.Query("end_date")

    dateFilter := ""
    args := []interface{}{tagID}
    argIndex := 2

    if startDate != "" && endDate != "" {
        dateFilter = fmt.Sprintf(" AND j.operation_date BETWEEN $%d AND $%d", argIndex, argIndex+1)
        args = append(args, startDate, endDate)
        argIndex += 2
    }

    query := fmt.Sprintf(`
        SELECT 
            j.id,
            j.operation_date,
            j.document_number,
            j.counterparty_name,
            j.debit_account,
            j.credit_account,
            j.debit_amount,
            j.credit_amount,
            j.description,
            j.status
        FROM journal_entries j
        JOIN journal_entry_tags jet ON jet.entry_id = j.id
        WHERE jet.tag_id = $1
        %s
        ORDER BY j.operation_date DESC
        LIMIT 100
    `, dateFilter)

    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var entries []gin.H
    for rows.Next() {
        var id uuid.UUID
        var opDate time.Time
        var docNumber, counterparty, debitAcc, creditAcc, description, status string
        var debit, credit float64

        err := rows.Scan(&id, &opDate, &docNumber, &counterparty,
            &debitAcc, &creditAcc, &debit, &credit, &description, &status)
        if err != nil {
            continue
        }

        entries = append(entries, gin.H{
            "id":               id,
            "operation_date":   opDate.Format("2006-01-02"),
            "document_number":  docNumber,
            "counterparty":     counterparty,
            "debit_account":    debitAcc,
            "credit_account":   creditAcc,
            "debit_amount":     debit,
            "credit_amount":    credit,
            "description":      description,
            "status":           status,
            "status_text":      map[string]string{"draft": "📝 Черновик", "posted": "✅ Проведена"}[status],
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "entries": entries,
        "count":   len(entries),
    })
}

// ===== ГЛАВНАЯ ФУНКЦИЯ ДЛЯ СОХРАНЕНИЯ ТЕГОВ =====
// AssignTagsToEntry - привязать НЕСКОЛЬКО тегов к проводке
func AssignTagsToEntry(c *gin.Context) {
    entryID := c.Param("entryId")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    var req struct {
        TagIDs []string `json:"tag_ids"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Проверяем, что проводка принадлежит этому tenant
    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM journal_entries WHERE id = $1 AND tenant_id = $2)
    `, entryID, tenantID).Scan(&exists)
    
    if err != nil || !exists {
        c.JSON(http.StatusNotFound, gin.H{"error": "Entry not found"})
        return
    }
    
    // Удаляем старые теги
    _, err = database.Pool.Exec(c.Request.Context(), `
        DELETE FROM journal_entry_tags WHERE entry_id = $1
    `, entryID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    // Добавляем новые теги
    if len(req.TagIDs) > 0 {
        for _, tagID := range req.TagIDs {
            _, err = database.Pool.Exec(c.Request.Context(), `
                INSERT INTO journal_entry_tags (entry_id, tag_id)
                VALUES ($1, $2)
                ON CONFLICT DO NOTHING
            `, entryID, tagID)
            
            if err != nil {
                // Используем fmt.Printf, чтобы не подключать log
                fmt.Printf("⚠️ Ошибка привязки тега %s: %v\n", tagID, err)
            }
        }
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Теги сохранены",
        "count":   len(req.TagIDs),
    })
}