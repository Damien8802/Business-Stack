package handlers

import (
    "encoding/json"
    "context" 
    "database/sql"
    "fmt"
    "log"  
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "subscription-system/database"
    "subscription-system/middleware"
)

func getCurrentUserID(c *gin.Context) string {
    userID := c.GetString("user_id")
    if userID == "" {
        log.Printf("⚠️ Warning: user_id is empty in context")
        return ""
    }
    return userID
}
// ChartOfAccount структура счета
type ChartOfAccount struct {
    ID          uuid.UUID  `json:"id"`
    Code        string     `json:"code"`
    Name        string     `json:"name"`
    AccountType string     `json:"account_type"`
    ParentID    *uuid.UUID `json:"parent_id"`
    Level       int        `json:"level"`
    IsGroup     bool       `json:"is_group"`
    Currency    string     `json:"currency"`
    Description string     `json:"description"`
    IsActive    bool       `json:"is_active"`
    CreatedAt   time.Time  `json:"created_at"`
}

func GetChartOfAccounts(c *gin.Context) {
    // Используем tenant_id, а не user_id
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        log.Printf("❌ GetChartOfAccounts: tenant_id not found")
        c.JSON(http.StatusOK, gin.H{"success": true, "accounts": []interface{}{}})
        return
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, code, name, account_type, is_active, created_at
        FROM chart_of_accounts
        WHERE tenant_id = $1 AND (deleted_at IS NULL OR deleted_at = '0001-01-01')
        ORDER BY code
    `, tenantID)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"success": true, "accounts": []interface{}{}})
        return
    }
    defer rows.Close()
    
    var accounts []gin.H
    for rows.Next() {
        var id uuid.UUID
        var code, name, accountType string
        var isActive bool
        var createdAt time.Time
        
        err := rows.Scan(&id, &code, &name, &accountType, &isActive, &createdAt)
        if err != nil {
            continue
        }
        
        accounts = append(accounts, gin.H{
            "id":           id,
            "code":         code,
            "name":         name,
            "account_type": accountType,
            "is_active":    isActive,
            "created_at":   createdAt,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":  true,
        "accounts": accounts,
    })
}
func CreateChartOfAccount(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id не найден"})
        return
    }
    
    var req struct {
        Code        string `json:"code" binding:"required"`
        Name        string `json:"name" binding:"required"`
        AccountType string `json:"account_type" binding:"required"`
        IsActive    bool   `json:"is_active"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO chart_of_accounts (id, code, name, account_type, tenant_id, is_active, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, NOW())
    `, uuid.New(), req.Code, req.Name, req.AccountType, tenantID, req.IsActive)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}
func UpdateChartOfAccount(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id не найден"})
        return
    }
    
    accountID := c.Param("id")
    
    var req struct {
        Code        string     `json:"code"`
        Name        string     `json:"name"`
        AccountType string     `json:"account_type"`
        ParentID    *uuid.UUID `json:"parent_id"`
        Level       int        `json:"level"`
        IsGroup     bool       `json:"is_group"`
        Currency    string     `json:"currency"`
        Description string     `json:"description"`
        IsActive    bool       `json:"is_active"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE chart_of_accounts SET
            code = $1, name = $2, account_type = $3, parent_id = $4,
            level = $5, is_group = $6, currency = $7, description = $8,
            is_active = $9, updated_at = NOW()
        WHERE id = $10 AND tenant_id = $11
    `, req.Code, req.Name, req.AccountType, req.ParentID,
        req.Level, req.IsGroup, req.Currency, req.Description,
        req.IsActive, accountID, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Счет обновлен",
    })
}
func DeleteChartOfAccount(c *gin.Context) {
    log.Println("🚨🚨🚨 [1] DeleteChartOfAccount ВЫЗВАНА 🚨🚨🚨")
    
    // Получаем tenant_id из контекста
    log.Println("[2] Получаем tenant_id из контекста...")
    tenantID := c.GetString("tenant_id")
    log.Printf("[3] tenant_id = '%s'", tenantID)
    
    if tenantID == "" {
        log.Printf("❌ [4] DeleteChartOfAccount: tenant_id not found")
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id не найден"})
        return
    }
    log.Println("[5] tenant_id найден успешно")
    
    log.Println("[6] Получаем accountID из параметров...")
    accountID := c.Param("id")
    log.Printf("[7] accountID = '%s'", accountID)
    
    if accountID == "" {
        log.Println("❌ [8] ID счета не указан")
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID счета не указан"})
        return
    }
    log.Println("[9] accountID получен успешно")
    
    log.Printf("[10] 📦 Удаление (архивация) счета: accountID=%s, tenantID=%s", accountID, tenantID)
    
    // Проверяем, есть ли связанные проводки
    log.Println("[11] Проверяем наличие связанных проводок...")
    var hasPostings bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(
            SELECT 1 FROM journal_postings jp
            WHERE jp.account_id = $1
        )
    `, accountID).Scan(&hasPostings)
    
    if err != nil {
        log.Printf("❌ [12] Ошибка проверки связей: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки связей"})
        return
    }
    log.Printf("[13] hasPostings = %v", hasPostings)
    
    if hasPostings {
        log.Println("[14] Счет используется в проводках - отказ от удаления")
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Нельзя удалить счет, так как он используется в проводках",
        })
        return
    }
    log.Println("[15] Счет не используется в проводках, можно архивировать")
    
    // Архивируем счет (мягкое удаление)
    log.Println("[16] Выполняем UPDATE для архивации...")
    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE chart_of_accounts 
        SET deleted_at = NOW(), 
            is_active = false, 
            updated_at = NOW()
        WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
    `, accountID, tenantID)
    
    if err != nil {
        log.Printf("❌ [17] Ошибка архивации счета: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось удалить счет"})
        return
    }
    log.Println("[18] UPDATE выполнен")
    
    rowsAffected := result.RowsAffected()
    log.Printf("[19] Затронуто строк: %d", rowsAffected)
    
    if rowsAffected == 0 {
        log.Println("[20] Счет не найден или уже удален")
        c.JSON(http.StatusNotFound, gin.H{"error": "Счет не найден или уже удален"})
        return
    }
    
    log.Printf("[21] ✅ Счет %s успешно архивирован", accountID)
    c.JSON(http.StatusOK, gin.H{
        "success":  true,
        "message":  "Счет перемещен в архив",
        "archived": true,
    })
    log.Println("[22] Ответ отправлен клиенту")
}
// ==================== ЖУРНАЛ ПРОВОДОК ====================

type JournalEntry struct {
    ID          uuid.UUID  `json:"id"`
    EntryNumber string     `json:"entry_number"`
    EntryDate   time.Time  `json:"entry_date"`
    Description string     `json:"description"`
    SourceType  string     `json:"source_type"`
    SourceID    *uuid.UUID `json:"source_id"`
    TotalAmount float64    `json:"total_amount"`
    Status      string     `json:"status"`
    PostedBy    *uuid.UUID `json:"posted_by"`
    PostedAt    *time.Time `json:"posted_at"`
    Notes       string     `json:"notes"`
    CreatedAt   time.Time  `json:"created_at"`
}

type JournalPosting struct {
    ID           uuid.UUID `json:"id"`
    EntryID      uuid.UUID `json:"entry_id"`
    AccountID    uuid.UUID `json:"account_id"`
    AccountCode  string    `json:"account_code"`
    AccountName  string    `json:"account_name"`
    DebitAmount  float64   `json:"debit_amount"`
    CreditAmount float64   `json:"credit_amount"`
    Description  string    `json:"description"`
    CreatedAt    time.Time `json:"created_at"`
}

func GetJournalEntries(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userEmail := c.GetString("user_email")
    userRole := c.GetString("role")
    
    if userEmail == "dev@businesstack.ru" || userRole == "owner" {
        query := `
            SELECT id, operation_date, document_number, document_type,
                   counterparty_name, counterparty_inn, debit_amount, credit_amount, 
                   debit_account, credit_account, description, created_at, updated_at
            FROM journal_entries
            ORDER BY operation_date DESC
            LIMIT 100
        `
        rows, err := database.Pool.Query(c.Request.Context(), query)
        if err != nil {
            c.JSON(http.StatusOK, gin.H{"entries": []interface{}{}, "total": 0})
            return
        }
        defer rows.Close()

        entries := make([]gin.H, 0)

        for rows.Next() {
            var id uuid.UUID
            var opDate time.Time
            var docNumber, docType, counterpartyName, counterpartyINN, description, debitAccount, creditAccount string
            var debit, credit float64
            var createdAt, updatedAt time.Time

            err := rows.Scan(&id, &opDate, &docNumber, &docType, &counterpartyName, &counterpartyINN,
                &debit, &credit, &debitAccount, &creditAccount, &description, &createdAt, &updatedAt)
            if err != nil {
                continue
            }

            entries = append(entries, gin.H{
                "id":                id,
                "operation_date":    opDate.Format("2006-01-02"),
                "document_number":   docNumber,
                "document_type":     docType,
                "counterparty_name": counterpartyName,
                "counterparty_inn":  counterpartyINN,
                "debit_amount":      debit,
                "credit_amount":     credit,
                "debit_account":     debitAccount,
                "credit_account":    creditAccount,
                "description":       description,
                "created_at":        createdAt.Format("2006-01-02 15:04:05"),
                "updated_at":        updatedAt.Format("2006-01-02 15:04:05"),
            })
        }
        c.JSON(http.StatusOK, gin.H{"entries": entries, "total": len(entries)})
        return
    }
    
    if tenantID == uuid.Nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    query := `
        SELECT id, operation_date, document_number, document_type,
               counterparty_name, counterparty_inn, debit_amount, credit_amount, 
               debit_account, credit_account, description, created_at, updated_at
        FROM journal_entries
        WHERE tenant_id = $1
        ORDER BY operation_date DESC
        LIMIT 100
    `
    rows, err := database.Pool.Query(c.Request.Context(), query, tenantID)
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"entries": []interface{}{}, "total": 0})
        return
    }
    defer rows.Close()

    entries := make([]gin.H, 0)

    for rows.Next() {
        var id uuid.UUID
        var opDate time.Time
        var docNumber, docType, counterpartyName, counterpartyINN, description, debitAccount, creditAccount string
        var debit, credit float64
        var createdAt, updatedAt time.Time

        err := rows.Scan(&id, &opDate, &docNumber, &docType, &counterpartyName, &counterpartyINN,
            &debit, &credit, &debitAccount, &creditAccount, &description, &createdAt, &updatedAt)
        if err != nil {
            continue
        }

        entries = append(entries, gin.H{
            "id":                id,
            "operation_date":    opDate.Format("2006-01-02"),
            "document_number":   docNumber,
            "document_type":     docType,
            "counterparty_name": counterpartyName,
            "counterparty_inn":  counterpartyINN,
            "debit_amount":      debit,
            "credit_amount":     credit,
            "debit_account":     debitAccount,
            "credit_account":    creditAccount,
            "description":       description,
            "created_at":        createdAt.Format("2006-01-02 15:04:05"),
            "updated_at":        updatedAt.Format("2006-01-02 15:04:05"),
        })
    }

    c.JSON(http.StatusOK, gin.H{"entries": entries, "total": len(entries)})
}

func GetJournalEntry(c *gin.Context) {
    userID := getUserID(c)
    entryID := c.Param("id")
    
    var e JournalEntry
    var sourceID sql.NullString
    var postedBy sql.NullString
    var postedAt sql.NullTime
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, entry_number, entry_date, description, source_type, source_id,
               total_amount, entry_status, posted_by, posted_at, notes, created_at
        FROM journal_entries
        WHERE id = $1 AND user_id = $2
    `, entryID, userID).Scan(
        &e.ID, &e.EntryNumber, &e.EntryDate, &e.Description,
        &e.SourceType, &sourceID, &e.TotalAmount, &e.Status,
        &postedBy, &postedAt, &e.Notes, &e.CreatedAt,
    )
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Проводка не найдена"})
        return
    }
    
    if sourceID.Valid {
        id, _ := uuid.Parse(sourceID.String)
        e.SourceID = &id
    }
    if postedBy.Valid {
        id, _ := uuid.Parse(postedBy.String)
        e.PostedBy = &id
    }
    if postedAt.Valid {
        e.PostedAt = &postedAt.Time
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT p.id, p.entry_id, p.account_id, a.code, a.name,
               p.debit_amount, p.credit_amount, p.description, p.created_at
        FROM journal_postings p
        JOIN chart_of_accounts a ON p.account_id = a.id
        WHERE p.entry_id = $1
    `, entryID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка загрузки проводок"})
        return
    }
    defer rows.Close()
    
    var postings []JournalPosting
    for rows.Next() {
        var p JournalPosting
        err := rows.Scan(
            &p.ID, &p.EntryID, &p.AccountID, &p.AccountCode,
            &p.AccountName, &p.DebitAmount, &p.CreditAmount,
            &p.Description, &p.CreatedAt,
        )
        if err != nil {
            continue
        }
        postings = append(postings, p)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":  true,
        "entry":    e,
        "postings": postings,
    })
}

func CreateJournalEntry(c *gin.Context) {
    userID := getUserID(c)
    
    var req struct {
        EntryDate   string  `json:"entry_date"`
        Description string  `json:"description" binding:"required"`
        SourceType  string  `json:"source_type"`
        SourceID    string  `json:"source_id"`
        Notes       string  `json:"notes"`
        Postings    []struct {
            AccountID    string  `json:"account_id" binding:"required"`
            DebitAmount  float64 `json:"debit_amount"`
            CreditAmount float64 `json:"credit_amount"`
            Description  string  `json:"description"`
        } `json:"postings" binding:"required"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    var totalDebit, totalCredit float64
    for _, p := range req.Postings {
        totalDebit += p.DebitAmount
        totalCredit += p.CreditAmount
    }
    
    if totalDebit != totalCredit {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Сумма дебета должна равняться сумме кредита",
        })
        return
    }
    
    entryNumber := fmt.Sprintf("ЖР-%d", time.Now().UnixNano()%1000000)
    
    entryDate := time.Now()
    if req.EntryDate != "" {
        ed, _ := time.Parse("2006-01-02", req.EntryDate)
        entryDate = ed
    }
    
    var sourceID *uuid.UUID
    if req.SourceID != "" {
        id, _ := uuid.Parse(req.SourceID)
        sourceID = &id
    }
    
    tx, err := database.Pool.Begin(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка транзакции"})
        return
    }
    defer tx.Rollback(c.Request.Context())
    
    var entryID uuid.UUID
    err = tx.QueryRow(c.Request.Context(), `
        INSERT INTO journal_entries (
            user_id, entry_number, entry_date, description, source_type,
            source_id, total_amount, entry_status, notes, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, 'draft', $8, NOW(), NOW())
        RETURNING id
    `, userID, entryNumber, entryDate, req.Description,
        req.SourceType, sourceID, totalDebit, req.Notes).Scan(&entryID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось создать проводку"})
        return
    }
    
    for _, p := range req.Postings {
        accountID, _ := uuid.Parse(p.AccountID)
        _, err = tx.Exec(c.Request.Context(), `
            INSERT INTO journal_postings (entry_id, account_id, debit_amount, credit_amount, description)
            VALUES ($1, $2, $3, $4, $5)
        `, entryID, accountID, p.DebitAmount, p.CreditAmount, p.Description)
        
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось добавить проводки"})
            return
        }
    }
    
    if err := tx.Commit(c.Request.Context()); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":      true,
        "entry_id":     entryID,
        "entry_number": entryNumber,
        "message":      "Проводка создана",
    })
}

func PostJournalEntry(c *gin.Context) {
    userID := getUserID(c)
    entryID := c.Param("id")
    
    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE journal_entries 
        SET entry_status = 'posted', posted_by = $1, posted_at = NOW(), updated_at = NOW()
        WHERE id = $2 AND user_id = $3 AND entry_status = 'draft'
    `, userID, entryID, userID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось провести проводку"})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Проводка не найдена или уже проведена"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Проводка проведена",
    })
}

func DeleteJournalEntry(c *gin.Context) {
    id := c.Param("id")
    
    result, err := database.Pool.Exec(c.Request.Context(), 
        "DELETE FROM journal_entries WHERE id = $1", id)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Проводка не найдена"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

func CreateJournalEntrySimple(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)

    if tenantID == uuid.Nil {
        log.Printf("❌ CreateJournalEntrySimple: tenant_id not found")
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": "Ошибка авторизации. Выйдите и зайдите заново.",
        })
        return
    }
    
    var req struct {
        OperationDate   string  `json:"operation_date"`
        DocumentNumber  string  `json:"document_number"`
        DocumentType    string  `json:"document_type"`
        CounterpartyName string `json:"counterparty_name"`
        CounterpartyINN  string `json:"counterparty_inn"`
        DebitAmount     float64 `json:"debit_amount"`
        CreditAmount    float64 `json:"credit_amount"`
        DebitAccount    string  `json:"debit_account"`
        CreditAccount   string  `json:"credit_account"`
        Description     string  `json:"description"`
        TagType         string  `json:"tag_type"`   // ← ДОБАВЛЕНО
        TagName         string  `json:"tag_name"`   // ← ДОБАВЛЕНО
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    if req.DebitAmount != req.CreditAmount {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Сумма дебета должна равняться сумме кредита",
        })
        return
    }
    
    operationDate, err := time.Parse("2006-01-02", req.OperationDate)
    if err != nil {
        operationDate = time.Now()
    }
    
    id := uuid.New()
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO journal_entries (
            id, tenant_id, operation_date, document_number, document_type,
            counterparty_name, counterparty_inn, debit_amount, credit_amount, 
            debit_account, credit_account, description, amount, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
    `, 
        id, tenantID, operationDate, req.DocumentNumber, req.DocumentType,
        req.CounterpartyName, req.CounterpartyINN, 
        req.DebitAmount, req.CreditAmount,
        req.DebitAccount, req.CreditAccount, 
        req.Description,
        req.DebitAmount,
    )
    
    if err != nil {
        fmt.Printf("❌ Ошибка создания проводки: %v\n", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "Не удалось создать проводку",
            "details": err.Error(),
        })
        return
    }
    
    // ✅ АВТОМАТИЧЕСКАЯ ПРИВЯЗКА ТЕГА
    if req.TagType != "" && req.TagName != "" {
        go func() {
            var tagID string
            err := database.Pool.QueryRow(context.Background(), `
                SELECT id FROM fincore_analytics_tags
                WHERE tenant_id = $1 AND type = $2 AND name = $3 AND is_active = true
            `, tenantID, req.TagType, req.TagName).Scan(&tagID)
            
            if err != nil {
                err = database.Pool.QueryRow(context.Background(), `
                    INSERT INTO fincore_analytics_tags (tenant_id, name, type, color, created_at)
                    VALUES ($1, $2, $3, '#667eea', NOW())
                    RETURNING id
                `, tenantID, req.TagName, req.TagType).Scan(&tagID)
                if err != nil {
                    return
                }
            }
            
            if tagID != "" {
                _, err = database.Pool.Exec(context.Background(), `
                    INSERT INTO journal_entry_tags (entry_id, tag_id)
                    VALUES ($1, $2)
                    ON CONFLICT DO NOTHING
                `, id, tagID)
                if err != nil {
                    log.Printf("⚠️ Ошибка привязки тега: %v", err)
                }
            }
        }()
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true, 
        "id": id,
        "message": "Проводка создана",
    })
}
// Альтернативная версия с использованием двух таблиц (journal_entries + journal_postings)
func CreateJournalEntryWithPostings(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    
    var req struct {
        OperationDate   string  `json:"operation_date"`
        DocumentNumber  string  `json:"document_number"`
        DocumentType    string  `json:"document_type"`
        CounterpartyName string `json:"counterparty_name"`
        CounterpartyINN  string `json:"counterparty_inn"`
        Description     string  `json:"description"`
        Postings        []struct {
            DebitAccount  string  `json:"debit_account"`
            CreditAccount string  `json:"credit_account"`
            Amount        float64 `json:"amount"`
        } `json:"postings"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Проверяем, что есть хотя бы одна проводка
    if len(req.Postings) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Не указаны проводки"})
        return
    }
    
    // Рассчитываем общую сумму
    var totalAmount float64
    for _, p := range req.Postings {
        totalAmount += p.Amount
    }
    
    operationDate, _ := time.Parse("2006-01-02", req.OperationDate)
    if operationDate.IsZero() {
        operationDate = time.Now()
    }
    
    // Начинаем транзакцию
    tx, err := database.Pool.Begin(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка начала транзакции"})
        return
    }
    defer tx.Rollback(c.Request.Context())
    
    // Создаем запись в journal_entries
    entryID := uuid.New()
    _, err = tx.Exec(c.Request.Context(), `
        INSERT INTO journal_entries (
            id, tenant_id, operation_date, document_number, document_type,
            counterparty_name, counterparty_inn, total_amount, description, 
            created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
    `,
        entryID, tenantID, operationDate, req.DocumentNumber, req.DocumentType,
        req.CounterpartyName, req.CounterpartyINN, totalAmount, req.Description,
    )
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось создать запись"})
        return
    }
    
    // Создаем проводки в journal_postings
    for _, posting := range req.Postings {
        // Получаем ID счетов по их кодам
        var debitAccountID, creditAccountID uuid.UUID
        
        err = tx.QueryRow(c.Request.Context(), `
            SELECT id FROM chart_of_accounts 
            WHERE code = $1 AND tenant_id = $2
        `, posting.DebitAccount, tenantID).Scan(&debitAccountID)
        
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{
                "error": fmt.Sprintf("Счет дебета %s не найден", posting.DebitAccount),
            })
            return
        }
        
        err = tx.QueryRow(c.Request.Context(), `
            SELECT id FROM chart_of_accounts 
            WHERE code = $1 AND tenant_id = $2
        `, posting.CreditAccount, tenantID).Scan(&creditAccountID)
        
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{
                "error": fmt.Sprintf("Счет кредита %s не найден", posting.CreditAccount),
            })
            return
        }
        
        // Добавляем проводку по дебету
        _, err = tx.Exec(c.Request.Context(), `
            INSERT INTO journal_postings (entry_id, account_id, debit_amount, created_at)
            VALUES ($1, $2, $3, NOW())
        `, entryID, debitAccountID, posting.Amount)
        
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания дебетовой проводки"})
            return
        }
        
        // Добавляем проводку по кредиту
        _, err = tx.Exec(c.Request.Context(), `
            INSERT INTO journal_postings (entry_id, account_id, credit_amount, created_at)
            VALUES ($1, $2, $3, NOW())
        `, entryID, creditAccountID, posting.Amount)
        
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания кредитовой проводки"})
            return
        }
    }
    
    // Фиксируем транзакцию
    if err := tx.Commit(c.Request.Context()); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "id": entryID,
        "message": "Проводка создана",
    })
}

// Получение проводок с деталями счетов
func GetJournalEntriesWithDetails(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    
    query := `
        SELECT 
            je.id, 
            je.operation_date, 
            je.document_number, 
            je.document_type,
            je.counterparty_name, 
            je.counterparty_inn, 
            je.description,
            je.total_amount,
            je.created_at,
            COALESCE(
                json_agg(
                    json_build_object(
                        'id', jp.id,
                        'debit_amount', jp.debit_amount,
                        'credit_amount', jp.credit_amount,
                        'account_id', ca.id,
                        'account_code', ca.code,
                        'account_name', ca.name
                    )
                ) FILTER (WHERE jp.id IS NOT NULL), 
                '[]'
            ) as postings
        FROM journal_entries je
        LEFT JOIN journal_postings jp ON je.id = jp.entry_id
        LEFT JOIN chart_of_accounts ca ON jp.account_id = ca.id
        WHERE je.tenant_id = $1
        GROUP BY je.id
        ORDER BY je.operation_date DESC
        LIMIT 100
    `
    
    rows, err := database.Pool.Query(c.Request.Context(), query, tenantID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var entries []gin.H
    for rows.Next() {
        var id uuid.UUID
        var opDate time.Time
        var docNumber, docType, counterpartyName, counterpartyINN, description string
        var totalAmount float64
        var createdAt time.Time
        var postingsJSON string
        
        err := rows.Scan(&id, &opDate, &docNumber, &docType, &counterpartyName, 
            &counterpartyINN, &description, &totalAmount, &createdAt, &postingsJSON)
        if err != nil {
            continue
        }
        
        entries = append(entries, gin.H{
            "id":                id,
            "operation_date":    opDate.Format("2006-01-02"),
            "document_number":   docNumber,
            "document_type":     docType,
            "counterparty_name": counterpartyName,
            "counterparty_inn":  counterpartyINN,
            "description":       description,
            "total_amount":      totalAmount,
            "postings":          postingsJSON,
            "created_at":        createdAt,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "entries": entries,
    })
}
func UpdateJournalEntrySimple(c *gin.Context) {
    id := c.Param("id")
    tenantID := middleware.GetTenantIDFromContext(c)

    var req struct {
        OperationDate   string  `json:"operation_date"`
        DebitAmount     float64 `json:"debit_amount"`
        CreditAmount    float64 `json:"credit_amount"`
        DebitAccount    string  `json:"debit_account"`
        CreditAccount   string  `json:"credit_account"`
        Description     string  `json:"description"`
        TagType         string  `json:"tag_type"`  
        TagName         string  `json:"tag_name"`  
    }


    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var operationDate time.Time
    if req.OperationDate == "" {
        operationDate = time.Now()
    } else {
        var err error
        operationDate, err = time.Parse("2006-01-02", req.OperationDate)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат даты"})
            return
        }
    }

    // ИСПРАВЛЕНО: обновляем и поле amount
    query := `
        UPDATE journal_entries 
        SET operation_date = $1,
            debit_amount = $2,
            credit_amount = $3,
            debit_account = $4,
            credit_account = $5,
            description = $6,
            amount = $7,
            updated_at = NOW()
        WHERE id = $8 AND tenant_id = $9
    `

    result, err := database.Pool.Exec(c.Request.Context(), query,
        operationDate, req.DebitAmount, req.CreditAmount,
        req.DebitAccount, req.CreditAccount, req.Description,
        req.DebitAmount, // ← amount = debit_amount
        id, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}
func GetFinancePayments(c *gin.Context) {
    userID := getCurrentUserID(c)
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            id, 
            amount, 
            status, 
            COALESCE(plan_name, '') as plan_name,
            COALESCE(user_name, '') as user_name,
            COALESCE(purpose, '') as purpose,
            created_at,
            COALESCE(payment_number, '') as payment_number
        FROM payments
        WHERE user_id = $1
        ORDER BY created_at DESC
        LIMIT 100
    `, userID)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "payments": []interface{}{},
        })
        return
    }
    defer rows.Close()
    
    var payments []gin.H
    for rows.Next() {
        var id uuid.UUID
        var amount float64
        var status, planName, userName, purpose, paymentNumber string
        var createdAt time.Time
        
        err := rows.Scan(&id, &amount, &status, &planName, &userName, &purpose, &createdAt, &paymentNumber)
        if err != nil {
            continue
        }
        
        payments = append(payments, gin.H{
            "id":             id.String(),
            "amount":         amount,
            "status":         status,
            "plan_name":      planName,
            "user_name":      userName,
            "purpose":        purpose,
            "created_at":     createdAt,
            "payment_number": paymentNumber,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "payments": payments,
    })
}
func CreateFinancePayment(c *gin.Context) {
    userID := getCurrentUserID(c)

    var req struct {
        Amount          float64 `json:"amount" binding:"required"`
        Currency        string  `json:"currency"`
        PlanName        string  `json:"plan_name"`
        UserName        string  `json:"user_name"`
        Purpose         string  `json:"purpose"`
        PaymentDate     string  `json:"payment_date"`
        Status          string  `json:"status"`
        PaymentType     string  `json:"payment_type"`
        PaymentMethod   string  `json:"payment_method"`
        CounterpartyName string `json:"counterparty_name"`
        Method          string  `json:"method"`  // ← ДОБАВИТЬ ЭТО ПОЛЕ
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Значения по умолчанию
    if req.Currency == "" {
        req.Currency = "RUB"
    }
    if req.Status == "" {
        req.Status = "completed"
    }
    if req.PaymentType == "" {
        req.PaymentType = "income"
    }
    if req.PaymentMethod == "" {
        req.PaymentMethod = "bank_transfer"
    }
    if req.Method == "" {
        req.Method = "bank_transfer"  // ← ЗНАЧЕНИЕ ПО УМОЛЧАНИЮ
    }
    if req.CounterpartyName == "" {
        req.CounterpartyName = req.UserName
    }

    paymentNumber := fmt.Sprintf("ПЛ-%d", time.Now().UnixNano()%1000000)

    paymentDate := time.Now()
    if req.PaymentDate != "" {
        if pd, err := time.Parse("2006-01-02", req.PaymentDate); err == nil {
            paymentDate = pd
        }
    }

    var id uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO payments (
            id, user_id, payment_number, payment_date, amount, currency,
            plan_name, user_name, purpose, status, payment_type, payment_method,
            counterparty_name, method, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW())
        RETURNING id
    `,
        uuid.New(), userID, paymentNumber, paymentDate, req.Amount, req.Currency,
        req.PlanName, req.UserName, req.Purpose, req.Status, req.PaymentType, req.PaymentMethod,
        req.CounterpartyName, req.Method,
    ).Scan(&id)

    if err != nil {
        fmt.Printf("❌ Ошибка создания платежа: %v\n", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":   "Не удалось создать платеж",
            "details": err.Error(),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":        true,
        "id":             id,
        "payment_number": paymentNumber,
        "message":        "Платеж создан",
    })
}
func UpdateFinancePaymentStatus(c *gin.Context) {
    userID := getUserID(c)
    paymentID := c.Param("id")
    
    var req struct {
        Status string `json:"status" binding:"required"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE payments 
        SET payment_status = $1, updated_at = NOW()
        WHERE id = $2 AND user_id = $3
    `, req.Status, paymentID, userID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось обновить статус"})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Платеж не найден"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Статус платежа обновлен",
    })
}

func GetCashOperations(c *gin.Context) {
    userID := getUserID(c)
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, operation_date, operation_type, amount, currency,
               counterparty_name, purpose, cashier_name, document_number, created_at
        FROM cash_operations
        WHERE user_id = $1
        ORDER BY operation_date DESC
    `, userID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var operations []CashOperation
    for rows.Next() {
        var o CashOperation
        err := rows.Scan(
            &o.ID, &o.OperationDate, &o.OperationType, &o.Amount,
            &o.Currency, &o.CounterpartyName, &o.Purpose,
            &o.CashierName, &o.DocumentNumber, &o.CreatedAt,
        )
        if err != nil {
            continue
        }
        operations = append(operations, o)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "operations": operations,
    })
}

func CreateCashOperation(c *gin.Context) {
    userID := getUserID(c)
    
    var req struct {
        OperationDate    string  `json:"operation_date"`
        OperationType    string  `json:"operation_type" binding:"required"`
        Amount           float64 `json:"amount" binding:"required"`
        Currency         string  `json:"currency"`
        CounterpartyName string `json:"counterparty_name"`
        Purpose          string  `json:"purpose"`
        CashierName      string  `json:"cashier_name"`
        DocumentNumber   string  `json:"document_number"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    if req.Currency == "" {
        req.Currency = "RUB"
    }
    
    operationDate := time.Now()
    if req.OperationDate != "" {
        od, _ := time.Parse("2006-01-02", req.OperationDate)
        operationDate = od
    }
    
    var id uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO cash_operations (
            user_id, operation_date, operation_type, amount, currency,
            counterparty_name, purpose, cashier_name, document_number, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
        RETURNING id
    `, userID, operationDate, req.OperationType, req.Amount, req.Currency,
        req.CounterpartyName, req.Purpose, req.CashierName, req.DocumentNumber).Scan(&id)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось создать операцию"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "id":      id,
        "message": "Кассовая операция создана",
    })
}

func GetJournalEntriesSimple(c *gin.Context) {
    log.Println("🔍 === START GetJournalEntriesSimple ===")
    
    tenantIDValue, exists := c.Get("tenant_id")
    if !exists {
        log.Println("❌ tenant_id not found in context")
        c.JSON(http.StatusOK, gin.H{"entries": []gin.H{}})
        return
    }
    
    tenantIDStr, ok := tenantIDValue.(string)
    if !ok {
        log.Printf("❌ Invalid tenant_id type: %T", tenantIDValue)
        c.JSON(http.StatusOK, gin.H{"entries": []gin.H{}})
        return
    }
    
    tenantID, err := uuid.Parse(tenantIDStr)
    if err != nil {
        log.Printf("❌ Invalid tenant_id UUID format: %v", err)
        c.JSON(http.StatusOK, gin.H{"entries": []gin.H{}})
        return
    }
    
    log.Printf("✅ tenant_id = %s", tenantID)
    
    // ✅ ИСПРАВЛЕНО: Добавлен подзапрос для загрузки тегов
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            j.id, j.operation_date, j.document_number, j.document_type,
            j.counterparty_name, j.counterparty_inn, 
            j.debit_amount, j.credit_amount, 
            j.debit_account, j.credit_account, 
            j.description, j.created_at, 
            COALESCE(j.status, 'draft') as status, j.amount,
            COALESCE(
                (SELECT json_agg(json_build_object('id', t.id, 'name', t.name))
                 FROM journal_entry_tags jet
                 JOIN fincore_analytics_tags t ON jet.tag_id = t.id
                 WHERE jet.entry_id = j.id AND t.is_active = true),
                '[]'::json
            ) as tags
        FROM journal_entries j
        WHERE j.tenant_id = $1
        ORDER BY j.operation_date DESC
        LIMIT 200
    `, tenantID)
    
    if err != nil {
        log.Printf("❌ Query error: %v", err)
        c.JSON(http.StatusOK, gin.H{"entries": []gin.H{}})
        return
    }
    defer rows.Close()
    
    var entries []gin.H
    rowNum := 0
    
    for rows.Next() {
        rowNum++
        var id uuid.UUID
        var opDate time.Time
        var debit, credit float64
        var createdAt time.Time
        var status string
        var amount sql.NullFloat64
        var tagsJSON []byte
        
        var documentNumber, documentType, counterpartyName, counterpartyINN, description sql.NullString
        var debitAccount, creditAccount sql.NullString
        
        err := rows.Scan(
            &id, &opDate,
            &documentNumber, &documentType,
            &counterpartyName, &counterpartyINN,
            &debit, &credit,
            &debitAccount, &creditAccount,
            &description, &createdAt,
            &status, &amount,
            &tagsJSON,
        )
        if err != nil {
            log.Printf("❌ Scan error row %d: %v", rowNum, err)
            continue
        }
        
        // Парсим теги
        var tags []gin.H
        if len(tagsJSON) > 0 {
            if err := json.Unmarshal(tagsJSON, &tags); err != nil {
                log.Printf("⚠️ Ошибка парсинга тегов: %v", err)
                tags = []gin.H{}
            }
        }
        
        docNumberStr := ""
        if documentNumber.Valid {
            docNumberStr = documentNumber.String
        }
        
        docTypeStr := ""
        if documentType.Valid {
            docTypeStr = documentType.String
        }
        
        counterpartyNameStr := ""
        if counterpartyName.Valid {
            counterpartyNameStr = counterpartyName.String
        }
        
        counterpartyINNStr := ""
        if counterpartyINN.Valid {
            counterpartyINNStr = counterpartyINN.String
        }
        
        descriptionStr := ""
        if description.Valid {
            descriptionStr = description.String
        }
        
        debitAccountStr := ""
        if debitAccount.Valid {
            debitAccountStr = debitAccount.String
        }
        
        creditAccountStr := ""
        if creditAccount.Valid {
            creditAccountStr = creditAccount.String
        }
        
        displayAmount := debit
        if amount.Valid && amount.Float64 > 0 {
            displayAmount = amount.Float64
        }
        
        entries = append(entries, gin.H{
            "id":                id,
            "operation_date":    opDate.Format("2006-01-02"),
            "document_number":   docNumberStr,
            "document_type":     docTypeStr,
            "counterparty_name": counterpartyNameStr,
            "counterparty_inn":  counterpartyINNStr,
            "debit_amount":      debit,
            "credit_amount":     credit,
            "debit_account":     debitAccountStr,
            "credit_account":    creditAccountStr,
            "description":       descriptionStr,
            "created_at":        createdAt,
            "status":            status,
            "amount":            displayAmount,
            "tags":              tags, // ✅ ТЕПЕРЬ ЕСТЬ ТЕГИ!
        })
    }
    
    log.Printf("🎉 Итого: обработано %d строк, возвращается %d записей", rowNum, len(entries))
    log.Println("🔍 === END GetJournalEntriesSimple ===")
    
    c.JSON(http.StatusOK, gin.H{"entries": entries})
}
// Следующая функция начинается здесь
func DeleteJournalEntrySimple(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    entryID := c.Param("id")
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM journal_entries
        WHERE id = $1 AND tenant_id = $2
    `, entryID, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

func UpdateJournalEntry(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    entryID := c.Param("id")
    
    var req struct {
        OperationDate  string  `json:"operation_date"`
        DebitAccount   string  `json:"debit_account"`
        CreditAccount  string  `json:"credit_account"`
        DebitAmount    float64 `json:"debit_amount"`
        CreditAmount   float64 `json:"credit_amount"`
        Description    string  `json:"description"`
        Status         string  `json:"status"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Если передан только статус - обновляем только статус (кнопка "Провести")
    if req.Status != "" && req.DebitAccount == "" && req.CreditAccount == "" && req.DebitAmount == 0 {
        _, err := database.Pool.Exec(c.Request.Context(), `
            UPDATE journal_entries 
            SET status = $1, updated_at = NOW()
            WHERE id = $2 AND tenant_id = $3
        `, req.Status, entryID, tenantID)
        
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        
        c.JSON(http.StatusOK, gin.H{"success": true})
        return
    }
    
    // Полное обновление проводки
    operationDate, _ := time.Parse("2006-01-02", req.OperationDate)
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE journal_entries SET
            operation_date = $1,
            debit_account = $2,
            credit_account = $3,
            debit_amount = $4,
            credit_amount = $5,
            description = $6,
            status = COALESCE($7, status),
            updated_at = NOW()
        WHERE id = $8 AND tenant_id = $9
    `, operationDate, req.DebitAccount, req.CreditAccount, 
        req.DebitAmount, req.CreditAmount, req.Description, 
        req.Status, entryID, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}
type Payment struct {
    ID               uuid.UUID  `json:"id"`
    PaymentNumber    string     `json:"payment_number"`
    PaymentDate      time.Time  `json:"payment_date"`
    PaymentType      string     `json:"payment_type"`
    Amount           float64    `json:"amount"`
    Currency         string     `json:"currency"`
    PaymentMethod    string     `json:"payment_method"`
    CounterpartyID   *uuid.UUID `json:"counterparty_id"`
    CounterpartyType string     `json:"counterparty_type"`
    CounterpartyName string     `json:"counterparty_name"`
    Purpose          string     `json:"purpose"`
    Status           string     `json:"status"`
    DocumentNumber   string     `json:"document_number"`
    EntryID          *uuid.UUID `json:"entry_id"`
    CreatedAt        time.Time  `json:"created_at"`
}

type CashOperation struct {
    ID               uuid.UUID `json:"id"`
    OperationDate    time.Time `json:"operation_date"`
    OperationType    string    `json:"operation_type"`
    Amount           float64   `json:"amount"`
    Currency         string    `json:"currency"`
    CounterpartyName string    `json:"counterparty_name"`
    Purpose          string    `json:"purpose"`
    CashierName      string    `json:"cashier_name"`
    DocumentNumber   string    `json:"document_number"`
    CreatedAt        time.Time `json:"created_at"`
}

func BulkCreateJournalEntries(c *gin.Context) {
    userID := getCurrentUserID(c)
    
    var entries []struct {
        Date          string  `json:"date"`
        DebitAccount  string  `json:"debit_account"`
        CreditAccount string  `json:"credit_account"`
        Amount        float64 `json:"amount"`
        Description   string  `json:"description"`
    }
    
    if err := c.BindJSON(&entries); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    tx, err := database.Pool.Begin(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer tx.Rollback(c.Request.Context())
    
    for _, entry := range entries {
        date, _ := time.Parse("2006-01-02", entry.Date)
        _, err := tx.Exec(c.Request.Context(), `
            INSERT INTO journal_entries (id, user_id, operation_date, debit_account, credit_account, debit_amount, description, created_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
        `, uuid.New(), userID, date, entry.DebitAccount, entry.CreditAccount, entry.Amount, entry.Description)
        
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
    }
    
    if err := tx.Commit(c.Request.Context()); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "count": len(entries)})
}

func ImportJournalEntries(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Импорт временно недоступен"})
}
func ExportJournalEntries(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT operation_date, debit_account, credit_account, debit_amount, description, created_at
        FROM journal_entries
        WHERE tenant_id = $1
        ORDER BY operation_date DESC
        LIMIT 1000
    `, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var entries []gin.H
    for rows.Next() {
        var opDate time.Time
        var debitAccount, creditAccount, description string
        var amount float64
        var createdAt time.Time
        
        err := rows.Scan(&opDate, &debitAccount, &creditAccount, &amount, &description, &createdAt)
        if err != nil {
            continue
        }
        
        entries = append(entries, gin.H{
            "date":            opDate.Format("02.01.2006"),
            "debit_account":   debitAccount,
            "credit_account":  creditAccount,
            "amount":          amount,
            "description":     description,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "data": entries})
}

// UpdatePayment - обновление платежа
// UpdatePayment - обновление платежа (суммы)
func UpdatePayment(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Не авторизован"})
        return
    }
    
    paymentID := c.Param("id")
    
    var req struct {
        Amount float64 `json:"amount"`
    }
    
    // Пробуем получить JSON
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
        return
    }
    
    if req.Amount <= 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Сумма должна быть больше 0"})
        return
    }
    
    // Проверяем, существует ли платеж
    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM payments WHERE id = $1 AND user_id = $2)
    `, paymentID, userID).Scan(&exists)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки платежа: " + err.Error()})
        return
    }
    
    if !exists {
        c.JSON(http.StatusNotFound, gin.H{"error": "Платёж не найден"})
        return
    }
    
    // Обновляем сумму
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE payments 
        SET amount = $1, updated_at = NOW()
        WHERE id = $2 AND user_id = $3
    `, req.Amount, paymentID, userID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления: " + err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true, 
        "message": "Платёж обновлён",
        "new_amount": req.Amount,
    })
}
// DeletePayment - удаление платежа
func DeletePayment(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Не авторизован"})
        return
    }
    
    paymentID := c.Param("id")
    
    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM payments 
        WHERE id = $1 AND user_id = $2
    `, paymentID, userID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    rowsAffected := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Платёж не найден"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Платёж удалён"})
}

// ArchiveChartOfAccount - архивирование счета (мягкое удаление)
func ArchiveChartOfAccount(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        log.Printf("❌ ArchiveChartOfAccount: tenant_id not found")
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id не найден"})
        return
    }
    
    accountID := c.Param("id")
    
    log.Printf("📦 Архивация счета: accountID=%s, tenantID=%s", accountID, tenantID)
    
    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE chart_of_accounts 
        SET deleted_at = NOW(), 
            is_active = false, 
            updated_at = NOW()
        WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
    `, accountID, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка архивации: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка архивации"})
        return
    }
    
    rows := result.RowsAffected()
    log.Printf("✅ Архивировано строк: %d", rows)
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Счет архивирован",
        "rows": rows,
    })
}
// RestoreChartOfAccount - восстановление счета из архива
func RestoreChartOfAccount(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        log.Printf("❌ RestoreChartOfAccount: tenant_id not found in context")
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id не найден"})
        return
    }
    
    accountID := c.Param("id")
    
    // Проверяем права
    userEmail := c.GetString("user_email")
    userRole := c.GetString("role")
    platformRole := c.GetString("platform_role")
    
    if userEmail != "dev@businessstack.ru" && platformRole != "owner" && userRole != "owner" && userRole != "admin" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Недостаточно прав для восстановления счета"})
        return
    }
    
    // Проверяем существование архивированного счета
    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM chart_of_accounts 
                      WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NOT NULL)
    `, accountID, tenantID).Scan(&exists)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки счета"})
        return
    }
    
    if !exists {
        c.JSON(http.StatusNotFound, gin.H{"error": "Архивированный счет не найден"})
        return
    }
    
    // Восстанавливаем счет
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE chart_of_accounts 
        SET deleted_at = NULL, 
            is_active = true, 
            updated_at = NOW()
        WHERE id = $1 AND tenant_id = $2
    `, accountID, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка восстановления счета: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка восстановления счета"})
        return
    }
    
    log.Printf("✅ Счет %s успешно восстановлен", accountID)
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Счет успешно восстановлен",
    })
}

// GetArchivedChartOfAccounts - получение списка архивированных счетов
func GetArchivedChartOfAccounts(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        log.Printf("❌ GetArchivedChartOfAccounts: tenant_id not found")
        c.JSON(http.StatusOK, gin.H{"success": true, "accounts": []interface{}{}, "total": 0, "page": 1, "total_pages": 0})
        return
    }
    
    // Получаем параметры пагинации
    page := 1
    if p := c.Query("page"); p != "" {
        if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
            page = parsed
        }
    }
    
    limit := 8 // количество на странице
    if l := c.Query("limit"); l != "" {
        if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
            limit = parsed
        }
    }
    
    offset := (page - 1) * limit
    
    log.Printf("📋 Запрос архивированных счетов: tenant=%s, page=%d, limit=%d", tenantID, page, limit)
    
    // Считаем общее количество
    var total int
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*)
        FROM chart_of_accounts
        WHERE tenant_id = $1 AND deleted_at IS NOT NULL
    `, tenantID).Scan(&total)
    
    if err != nil {
        log.Printf("❌ Ошибка подсчета: %v", err)
        c.JSON(http.StatusOK, gin.H{"success": true, "accounts": []interface{}{}, "total": 0, "page": 1, "total_pages": 0})
        return
    }
    
    // Получаем записи с пагинацией
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            id,
            COALESCE(code, '') as code,
            COALESCE(name, '') as name,
            COALESCE(account_type, '') as account_type,
            parent_id,
            COALESCE(level, 0) as level,
            COALESCE(is_group, false) as is_group,
            COALESCE(currency, 'RUB') as currency,
            COALESCE(description, '') as description,
            COALESCE(is_active, false) as is_active,
            created_at,
            updated_at,
            deleted_at
        FROM chart_of_accounts
        WHERE tenant_id = $1 AND deleted_at IS NOT NULL
        ORDER BY deleted_at DESC
        LIMIT $2 OFFSET $3
    `, tenantID, limit, offset)
    
    if err != nil {
        log.Printf("❌ Ошибка получения: %v", err)
        c.JSON(http.StatusOK, gin.H{"success": true, "accounts": []interface{}{}, "total": 0, "page": 1, "total_pages": 0})
        return
    }
    defer rows.Close()
    
    var accounts []gin.H
    for rows.Next() {
        var id uuid.UUID
        var code, name, accountType, currency, description string
        var parentID *uuid.UUID
        var level int
        var isGroup, isActive bool
        var createdAt, updatedAt, deletedAt time.Time
        
        err := rows.Scan(
            &id, &code, &name, &accountType, &parentID,
            &level, &isGroup, &currency, &description, &isActive,
            &createdAt, &updatedAt, &deletedAt,
        )
        if err != nil {
            log.Printf("Ошибка сканирования: %v", err)
            continue
        }
        
        accounts = append(accounts, gin.H{
            "id":           id,
            "code":         code,
            "name":         name,
            "account_type": accountType,
            "parent_id":    parentID,
            "level":        level,
            "is_group":     isGroup,
            "currency":     currency,
            "description":  description,
            "is_active":    isActive,
            "created_at":   createdAt,
            "updated_at":   updatedAt,
            "deleted_at":   deletedAt,
        })
    }
    
    totalPages := (total + limit - 1) / limit
    
    log.Printf("✅ Найдено: %d, показано: %d, страница: %d/%d", total, len(accounts), page, totalPages)
    
    c.JSON(http.StatusOK, gin.H{
        "success":     true,
        "accounts":    accounts,
        "total":       total,
        "page":        page,
        "limit":       limit,
        "total_pages": totalPages,
    })
}
// PermanentDeleteChartOfAccount - полное удаление счета (только если он архивирован)
func PermanentDeleteChartOfAccount(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id не найден"})
        return
    }
    
    accountID := c.Param("id")
    
    // Проверяем права - только владелец платформы
    userEmail := c.GetString("user_email")
    platformRole := c.GetString("platform_role")
    
    if userEmail != "dev@businessstack.ru" && platformRole != "owner" {
        c.JSON(http.StatusForbidden, gin.H{"error": "Только владелец платформы может полностью удалять счета"})
        return
    }
    
    // Проверяем, что счет архивирован
    var isDeleted bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT deleted_at IS NOT NULL FROM chart_of_accounts 
        WHERE id = $1 AND tenant_id = $2
    `, accountID, tenantID).Scan(&isDeleted)
    
    if err != nil {
        if err == sql.ErrNoRows {
            c.JSON(http.StatusNotFound, gin.H{"error": "Счет не найден"})
            return
        }
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки счета"})
        return
    }
    
    if !isDeleted {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Сначала архивируйте счет, затем удалите"})
        return
    }
    
    // Полное удаление счета
    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM chart_of_accounts 
        WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NOT NULL
    `, accountID, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка удаления счета: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления счета"})
        return
    }
    
    rowsAffected := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Счет не найден или не архивирован"})
        return
    }
    
    log.Printf("✅ Счет %s полностью удален", accountID)
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Счет полностью удален",
    })
}

// ========== АРХИВ ЖУРНАЛА ПРОВОДОК ==========

// MoveJournalToArchive - переместить проводку в архив
func MoveJournalToArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    id := c.Param("id")
    userID := c.GetString("user_id")

    // Получаем проводку
    var entry struct {
        ID             uuid.UUID
        OperationDate  time.Time
        DocumentNumber string
        DocumentType   string
        CounterpartyName string
        CounterpartyINN  string
        DebitAccount   string
        CreditAccount  string
        Amount         float64
        Description    string
        Status         string
        CreatedAt      time.Time
        UpdatedAt      time.Time
    }

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, operation_date, COALESCE(document_number, ''), COALESCE(document_type, ''),
               COALESCE(counterparty_name, ''), COALESCE(counterparty_inn, ''),
               COALESCE(debit_account, ''), COALESCE(credit_account, ''),
               amount, COALESCE(description, ''), 
               CASE WHEN status IS NULL THEN 'draft' ELSE status END,
               created_at, updated_at
        FROM journal_entries
        WHERE id = $1 AND tenant_id = $2
    `, id, tenantID).Scan(
        &entry.ID, &entry.OperationDate, &entry.DocumentNumber, &entry.DocumentType,
        &entry.CounterpartyName, &entry.CounterpartyINN, &entry.DebitAccount,
        &entry.CreditAccount, &entry.Amount, &entry.Description, &entry.Status,
        &entry.CreatedAt, &entry.UpdatedAt,
    )

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Проводка не найдена"})
        return
    }

    // Сохраняем в архив
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO journal_archive (
            id, original_id, tenant_id, operation_date, document_number, document_type,
            counterparty_name, counterparty_inn, debit_account, credit_account,
            amount, description, status, archived_at, archived_by, created_at, updated_at
        ) VALUES (
            gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), $13, $14, $15
        )
    `, entry.ID, tenantID, entry.OperationDate, entry.DocumentNumber, entry.DocumentType,
        entry.CounterpartyName, entry.CounterpartyINN, entry.DebitAccount, entry.CreditAccount,
        entry.Amount, entry.Description, entry.Status, userID, entry.CreatedAt, entry.UpdatedAt)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Удаляем из основного журнала
    _, err = database.Pool.Exec(c.Request.Context(), `
        DELETE FROM journal_entries WHERE id = $1 AND tenant_id = $2
    `, id, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Проводка перемещена в архив"})
}

// GetJournalArchiveList - получить список архивированных проводок
func GetJournalArchiveList(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    status := c.DefaultQuery("status", "all")
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
    offset := (page - 1) * limit

    var query string
    var countQuery string
    var args []interface{}
    args = append(args, tenantID)

    if status == "all" {
        query = `
            SELECT id, original_id, operation_date, document_number, counterparty_name,
                   debit_account, credit_account, amount, description, status, archived_at
            FROM journal_archive
            WHERE tenant_id = $1
            ORDER BY archived_at DESC
            LIMIT $2 OFFSET $3
        `
        countQuery = `SELECT COUNT(*) FROM journal_archive WHERE tenant_id = $1`
        args = append(args, limit, offset)
    } else {
        query = `
            SELECT id, original_id, operation_date, document_number, counterparty_name,
                   debit_account, credit_account, amount, description, status, archived_at
            FROM journal_archive
            WHERE tenant_id = $1 AND status = $2
            ORDER BY archived_at DESC
            LIMIT $3 OFFSET $4
        `
        countQuery = `SELECT COUNT(*) FROM journal_archive WHERE tenant_id = $1 AND status = $2`
        args = append(args, status, limit, offset)
    }

    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var archives []gin.H
    for rows.Next() {
        var id, originalID uuid.UUID
        var opDate time.Time
        var docNumber, counterparty, debitAcc, creditAcc, description, status string
        var amount float64
        var archivedAt time.Time

        rows.Scan(&id, &originalID, &opDate, &docNumber, &counterparty,
            &debitAcc, &creditAcc, &amount, &description, &status, &archivedAt)

        archives = append(archives, gin.H{
            "id":              id,
            "original_id":     originalID,
            "date":            opDate.Format("2006-01-02"),
            "document_number": docNumber,
            "counterparty":    counterparty,
            "debit_account":   debitAcc,
            "credit_account":  creditAcc,
            "amount":          amount,
            "description":     description,
            "status":          status,
            "status_text":     map[string]string{"draft": "📝 Черновик", "posted": "✅ Проведена"}[status],
            "archived_at":     archivedAt.Format("2006-01-02 15:04:05"),
        })
    }

    var total int
    database.Pool.QueryRow(c.Request.Context(), countQuery, tenantID).Scan(&total)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    archives,
        "total":   total,
        "page":    page,
        "limit":   limit,
    })
}

// RestoreJournalFromArchive - восстановить из архива
func RestoreJournalFromArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    archiveID := c.Param("id")

    var archive struct {
        OriginalID      uuid.UUID
        OperationDate   time.Time
        DocumentNumber  string
        DocumentType    string
        CounterpartyName string
        CounterpartyINN  string
        DebitAccount    string
        CreditAccount   string
        Amount          float64
        Description     string
        Status          string
        CreatedAt       time.Time
        UpdatedAt       time.Time
    }

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT original_id, operation_date, document_number, document_type,
               counterparty_name, counterparty_inn, debit_account, credit_account,
               amount, description, status, created_at, updated_at
        FROM journal_archive
        WHERE id = $1 AND tenant_id = $2
    `, archiveID, tenantID).Scan(
        &archive.OriginalID, &archive.OperationDate, &archive.DocumentNumber,
        &archive.DocumentType, &archive.CounterpartyName, &archive.CounterpartyINN,
        &archive.DebitAccount, &archive.CreditAccount, &archive.Amount,
        &archive.Description, &archive.Status, &archive.CreatedAt, &archive.UpdatedAt,
    )

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись в архиве не найдена"})
        return
    }

    // Восстанавливаем в основной журнал
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO journal_entries (
            id, tenant_id, operation_date, document_number, document_type,
            counterparty_name, counterparty_inn, debit_account, credit_account,
            amount, description, status, created_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
    `, archive.OriginalID, tenantID, archive.OperationDate, archive.DocumentNumber,
        archive.DocumentType, archive.CounterpartyName, archive.CounterpartyINN,
        archive.DebitAccount, archive.CreditAccount, archive.Amount,
        archive.Description, archive.Status, archive.CreatedAt, archive.UpdatedAt)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Удаляем из архива
    _, err = database.Pool.Exec(c.Request.Context(), `
        DELETE FROM journal_archive WHERE id = $1 AND tenant_id = $2
    `, archiveID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Проводка восстановлена"})
}

// PermanentDeleteJournalArchive - удалить навсегда из архива
func PermanentDeleteJournalArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    archiveID := c.Param("id")

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM journal_archive WHERE id = $1 AND tenant_id = $2
    `, archiveID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Проводка удалена навсегда"})
}

// GetJournalArchiveStats - статистика архива
func GetJournalArchiveStats(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var draftCount, postedCount, totalCount int

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM journal_archive WHERE tenant_id = $1 AND status = 'draft'
    `, tenantID).Scan(&draftCount)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM journal_archive WHERE tenant_id = $1 AND status = 'posted'
    `, tenantID).Scan(&postedCount)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM journal_archive WHERE tenant_id = $1
    `, tenantID).Scan(&totalCount)

    c.JSON(http.StatusOK, gin.H{
        "success":      true,
        "draft_count":  draftCount,
        "posted_count": postedCount,
        "total_count":  totalCount,
    })
}

// ========== МАССОВЫЕ ОПЕРАЦИИ (ЧЕГО НЕ ХВАТАЕТ) ==========

// MassMoveToArchive - массовое перемещение в архив
func MassMoveToArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var req struct {
        IDs []string `json:"ids" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if len(req.IDs) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No IDs provided"})
        return
    }

    userID := c.GetString("user_id")
    var successCount, failCount int
    var failedIDs []string

    for _, id := range req.IDs {
        // Получаем проводку
        var entry struct {
            ID             uuid.UUID
            OperationDate  time.Time
            DocumentNumber string
            DocumentType   string
            CounterpartyName string
            CounterpartyINN  string
            DebitAccount   string
            CreditAccount  string
            Amount         float64
            Description    string
            Status         string
            CreatedAt      time.Time
            UpdatedAt      time.Time
        }

        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT id, operation_date, COALESCE(document_number, ''), COALESCE(document_type, ''),
                   COALESCE(counterparty_name, ''), COALESCE(counterparty_inn, ''),
                   COALESCE(debit_account, ''), COALESCE(credit_account, ''),
                   amount, COALESCE(description, ''), 
                   CASE WHEN status IS NULL THEN 'draft' ELSE status END,
                   created_at, updated_at
            FROM journal_entries
            WHERE id = $1 AND tenant_id = $2
        `, id, tenantID).Scan(
            &entry.ID, &entry.OperationDate, &entry.DocumentNumber, &entry.DocumentType,
            &entry.CounterpartyName, &entry.CounterpartyINN, &entry.DebitAccount,
            &entry.CreditAccount, &entry.Amount, &entry.Description, &entry.Status,
            &entry.CreatedAt, &entry.UpdatedAt,
        )

        if err != nil {
            failCount++
            failedIDs = append(failedIDs, id)
            continue
        }

        // Сохраняем в архив
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO journal_archive (
                id, original_id, tenant_id, operation_date, document_number, document_type,
                counterparty_name, counterparty_inn, debit_account, credit_account,
                amount, description, status, archived_at, archived_by, created_at, updated_at
            ) VALUES (
                gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), $13, $14, $15
            )
        `, entry.ID, tenantID, entry.OperationDate, entry.DocumentNumber, entry.DocumentType,
            entry.CounterpartyName, entry.CounterpartyINN, entry.DebitAccount, entry.CreditAccount,
            entry.Amount, entry.Description, entry.Status, userID, entry.CreatedAt, entry.UpdatedAt)

        if err != nil {
            failCount++
            failedIDs = append(failedIDs, id)
            continue
        }

        // Удаляем из основного журнала
        _, err = database.Pool.Exec(c.Request.Context(), `
            DELETE FROM journal_entries WHERE id = $1 AND tenant_id = $2
        `, id, tenantID)

        if err != nil {
            failCount++
            failedIDs = append(failedIDs, id)
            continue
        }

        successCount++
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "success_count": successCount,
        "fail_count":    failCount,
        "failed_ids":    failedIDs,
        "message":       fmt.Sprintf("✅ Перемещено в архив: %d, ❌ Ошибок: %d", successCount, failCount),
    })
}

// MassDeleteEntries - массовое удаление проводок
func MassDeleteEntries(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var req struct {
        IDs []string `json:"ids" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if len(req.IDs) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No IDs provided"})
        return
    }

    var successCount, failCount int
    var failedIDs []string

    for _, id := range req.IDs {
        // Проверяем статус (нельзя удалять проведённые)
        var status string
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT COALESCE(status, 'draft') FROM journal_entries 
            WHERE id = $1 AND tenant_id = $2
        `, id, tenantID).Scan(&status)

        if err != nil {
            failCount++
            failedIDs = append(failedIDs, id)
            continue
        }

        if status == "posted" {
            failCount++
            failedIDs = append(failedIDs, id)
            continue
        }

        _, err = database.Pool.Exec(c.Request.Context(), `
            DELETE FROM journal_entries WHERE id = $1 AND tenant_id = $2
        `, id, tenantID)

        if err != nil {
            failCount++
            failedIDs = append(failedIDs, id)
            continue
        }

        successCount++
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "success_count": successCount,
        "fail_count":    failCount,
        "failed_ids":    failedIDs,
        "message":       fmt.Sprintf("✅ Удалено: %d, ❌ Ошибок: %d", successCount, failCount),
    })
}

// MassPostEntries - массовое проведение проводок
func MassPostEntries(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var req struct {
        IDs []string `json:"ids" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if len(req.IDs) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No IDs provided"})
        return
    }

    var successCount, failCount int
    var failedIDs []string

    for _, id := range req.IDs {
        result, err := database.Pool.Exec(c.Request.Context(), `
            UPDATE journal_entries 
            SET status = 'posted', updated_at = NOW()
            WHERE id = $1 AND tenant_id = $2 
            AND (status IS NULL OR status = 'draft')
        `, id, tenantID)

        if err != nil {
            failCount++
            failedIDs = append(failedIDs, id)
            continue
        }

        if result.RowsAffected() == 0 {
            failCount++
            failedIDs = append(failedIDs, id)
            continue
        }

        successCount++
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "success_count": successCount,
        "fail_count":    failCount,
        "failed_ids":    failedIDs,
        "message":       fmt.Sprintf("✅ Проведено: %d, ❌ Ошибок: %d", successCount, failCount),
    })
}

// DeleteTemplatePosting - удаление шаблона проводки
func DeleteTemplatePosting(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    templateID := c.Param("id")
    userID := c.GetString("user_id")

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM posting_templates 
        WHERE id = $1 AND user_id = $2
    `, templateID, userID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Шаблон не найден"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Шаблон удалён"})
}

// ========== УВЕДОМЛЕНИЯ О ПРЕВЫШЕНИИ БЮДЖЕТА ==========
func CheckBudgetAlerts(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Unauthorized"})
        return
    }

    tagID := c.Query("tag_id")
    year := c.Query("year")

    if tagID == "" || year == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tag_id and year required"})
        return
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT month, planned, actual
        FROM budgets
        WHERE tag_id = $1 AND year = $2
        ORDER BY month
    `, tagID, year)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var alerts []gin.H
    months := []string{"Янв", "Фев", "Мар", "Апр", "Май", "Июн", "Июл", "Авг", "Сен", "Окт", "Ноя", "Дек"}

    for rows.Next() {
        var month int
        var planned, actual float64
        rows.Scan(&month, &planned, &actual)

        if planned > 0 && actual > planned*1.1 {
            alerts = append(alerts, gin.H{
                "month":     months[month-1],
                "planned":   planned,
                "actual":    actual,
                "overspend": actual - planned,
                "percent":   (actual - planned) / planned * 100,
                "message":   fmt.Sprintf("⚠️ Расходы за %s превысили план на %.0f%%", months[month-1], ((actual-planned)/planned*100)),
            })
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "alerts":  alerts,
        "count":   len(alerts),
    })
}

// ========== ПРОГНОЗИРОВАНИЕ ==========
func GetForecast(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Unauthorized"})
        return
    }

    months := 6
    if m := c.Query("months"); m != "" {
        if val, err := strconv.Atoi(m); err == nil && val > 0 {
            months = val
        }
    }

    // Получаем исторические данные за последние 12 месяцев
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            DATE_TRUNC('month', operation_date) as month,
            SUM(debit_amount) as revenue,
            SUM(debit_amount - credit_amount) as profit
        FROM journal_entries
        WHERE tenant_id = $1 
            AND operation_date >= NOW() - INTERVAL '12 months'
        GROUP BY DATE_TRUNC('month', operation_date)
        ORDER BY month DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var history []gin.H
    for rows.Next() {
        var month time.Time
        var revenue, profit float64
        rows.Scan(&month, &revenue, &profit)
        history = append(history, gin.H{
            "month":   month.Format("2006-01"),
            "revenue": revenue,
            "profit":  profit,
        })
    }

    // Простой прогноз: среднее за последние 3 месяца
    var avgRevenue, avgProfit float64
    if len(history) >= 3 {
        for i := 0; i < 3 && i < len(history); i++ {
            if rev, ok := history[i]["revenue"].(float64); ok {
                avgRevenue += rev
            }
            if prof, ok := history[i]["profit"].(float64); ok {
                avgProfit += prof
            }
        }
        avgRevenue /= 3
        avgProfit /= 3
    }

    var forecast []gin.H
    now := time.Now()
    for i := 1; i <= months; i++ {
        futureMonth := now.AddDate(0, i, 0)
        forecast = append(forecast, gin.H{
            "month":   futureMonth.Format("2006-01"),
            "revenue": avgRevenue * (1 + float64(i)*0.02),
            "profit":  avgProfit * (1 + float64(i)*0.015),
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success":     true,
        "history":     history,
        "forecast":    forecast,
        "avg_revenue": avgRevenue,
        "avg_profit":  avgProfit,
    })
}

// ========== УПРАВЛЕНЧЕСКИЙ ДАШБОРД ==========
func GetManagementDashboard(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Unauthorized"})
        return
    }

    dateFrom := c.Query("start_date")
    dateTo := c.Query("end_date")

    if dateFrom == "" || dateTo == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "start_date and end_date required"})
        return
    }

    // Деньги на счетах
    var cashBalance float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount - credit_amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND operation_date BETWEEN $2 AND $3
    `, tenantID, dateFrom, dateTo).Scan(&cashBalance)

    // Дебиторская задолженность
    var receivables float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND operation_date BETWEEN $2 AND $3
            AND debit_account LIKE '62%'
    `, tenantID, dateFrom, dateTo).Scan(&receivables)

    // Кредиторская задолженность
    var payables float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(credit_amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND operation_date BETWEEN $2 AND $3
            AND credit_account LIKE '60%'
    `, tenantID, dateFrom, dateTo).Scan(&payables)

    // Чистая прибыль
    var profit float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount - credit_amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND operation_date BETWEEN $2 AND $3
    `, tenantID, dateFrom, dateTo).Scan(&profit)

    c.JSON(http.StatusOK, gin.H{
        "success":      true,
        "cash_balance": cashBalance,
        "receivables":  receivables,
        "payables":     payables,
        "profit":       profit,
    })
}

// ========== ОТЧЁТ ПО ТЕГАМ (ДЛЯ ABC-АНАЛИЗА) ==========
func GetFincoreReportByTag(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
        return
    }

    startDate := c.Query("start_date")
    endDate := c.Query("end_date")

    // Строим фильтр по датам
    dateFilter := ""
    args := []interface{}{tenantID}
    argIndex := 2

    if startDate != "" && endDate != "" {
        dateFilter = fmt.Sprintf(" AND j.operation_date BETWEEN $%d AND $%d", argIndex, argIndex+1)
        args = append(args, startDate, endDate)
        argIndex += 2
    }

    query := fmt.Sprintf(`
        SELECT 
            t.id,
            t.name,
            t.type,
            t.color,
            COALESCE(SUM(j.debit_amount), 0) as total_debit,
            COALESCE(SUM(j.credit_amount), 0) as total_credit,
            COALESCE(SUM(j.debit_amount - j.credit_amount), 0) as balance
        FROM fincore_analytics_tags t
        LEFT JOIN journal_entry_tags jet ON jet.tag_id = t.id
        LEFT JOIN journal_entries j ON jet.entry_id = j.id
        WHERE t.tenant_id = $1 AND t.is_active = true
        %s
        GROUP BY t.id, t.name, t.type, t.color
        ORDER BY balance DESC
    `, dateFilter)

    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
        return
    }
    defer rows.Close()

    var result []gin.H
    for rows.Next() {
        var id uuid.UUID
        var name, tagType, color string
        var totalDebit, totalCredit, balance float64

        err := rows.Scan(&id, &name, &tagType, &color, &totalDebit, &totalCredit, &balance)
        if err != nil {
            continue
        }

        result = append(result, gin.H{
            "tag_id":       id,
            "tag_name":     name,
            "tag_type":     tagType,
            "tag_color":    color,
            "total_debit":  totalDebit,
            "total_credit": totalCredit,
            "balance":      balance,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    result,
        "total":   len(result),
    })
}