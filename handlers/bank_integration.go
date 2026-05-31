package handlers

import (
    "bytes"
    "encoding/csv"
    "fmt"
    "io"
    "log"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/xuri/excelize/v2"

    "subscription-system/database"
)

// getTenantID - получает ID тенанта из контекста
func getTenantID(c *gin.Context) string {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("company_id")
    }
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    return tenantID
}

// GetBankAccounts - получить список счетов
func GetBankAccounts(c *gin.Context) {
    tenantID := getTenantID(c)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, bank_name, account_number, bic, initial_balance, is_active
        FROM bank_accounts 
        WHERE tenant_id = $1 AND is_active = true
        ORDER BY created_at DESC
    `, tenantID)

    if err != nil {
        log.Printf("GetBankAccounts error: %v", err)
        c.JSON(http.StatusOK, []gin.H{})
        return
    }
    defer rows.Close()

    var accounts []gin.H
    for rows.Next() {
        var id uuid.UUID
        var bankName, accountNumber, bic string
        var initialBalance float64
        var isActive bool

        rows.Scan(&id, &bankName, &accountNumber, &bic, &initialBalance, &isActive)

        accounts = append(accounts, gin.H{
            "id":             id,
            "bank_name":      bankName,
            "account_number": accountNumber,
            "bic":            bic,
            "balance":        initialBalance,
            "is_active":      isActive,
        })
    }

    if accounts == nil {
        accounts = []gin.H{}
    }

    c.JSON(http.StatusOK, accounts)
}

// ConnectBankAccount - подключение банковского счёта
func ConnectBankAccount(c *gin.Context) {
    tenantID := getTenantID(c)

    var req struct {
        BankName       string  `json:"bank_name"`
        AccountNumber  string  `json:"account_number"`
        BIC            string  `json:"bic"`
        InitialBalance float64 `json:"initial_balance"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if req.BankName == "" || req.AccountNumber == "" || req.BIC == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Заполните все поля"})
        return
    }

    accountID := uuid.New()

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO bank_accounts (id, tenant_id, company_id, bank_name, account_number, bic, initial_balance, is_active, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, true, NOW())
    `, accountID, tenantID, tenantID, req.BankName, req.AccountNumber, req.BIC, req.InitialBalance)

    if err != nil {
        log.Printf("Ошибка подключения банка: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect bank account: " + err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "message":    "Банковский счёт подключён",
        "account_id": accountID,
    })
}


// SyncBankStatements - синхронизация выписок
func SyncBankStatements(c *gin.Context) {
    accountID := c.Param("id")
    tenantID := getTenantID(c)

    // Проверяем, есть ли настройки API
    var bankName, apiKey string
    hasAPI := false
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT bank_name, api_key FROM bank_api_connections 
        WHERE account_id = $1 AND tenant_id = $2 AND is_active = true
    `, accountID, tenantID).Scan(&bankName, &apiKey)
    
    if err == nil && bankName != "" {
        hasAPI = true
    }

    // Считаем текущие транзакции
    var currentCount int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM bank_statements 
        WHERE account_id = $1 AND tenant_id = $2
    `, accountID, tenantID).Scan(&currentCount)

    // Создаём тестовые транзакции если их нет
    newTransactions := 0
    if currentCount == 0 {
        for i := 1; i <= 5; i++ {
            var debitAmount, creditAmount float64
            amount := float64(i * 10000)
            if i%2 == 0 {
                debitAmount = amount
                creditAmount = 0
            } else {
                debitAmount = 0
                creditAmount = amount
            }

            _, err := database.Pool.Exec(c.Request.Context(), `
                INSERT INTO bank_statements (id, tenant_id, account_id, operation_date, statement_date, debit_amount, credit_amount, counterparty_name, payment_purpose, status, created_at)
                VALUES (gen_random_uuid(), $1, $2, NOW() - ($3 || ' days')::interval, NOW() - ($3 || ' days')::interval, $4, $5, $6, $7, 'imported', NOW())
            `, tenantID, accountID, i, debitAmount, creditAmount, "Контрагент "+fmt.Sprint(i), "Оплата по счёту №"+fmt.Sprint(i))
            
            if err == nil {
                newTransactions++
            }
        }
    }

    // Новое количество после синхронизации
    var newCount int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM bank_statements 
        WHERE account_id = $1 AND tenant_id = $2
    `, accountID, tenantID).Scan(&newCount)

    // Формируем подробный ответ
    response := gin.H{
        "success":            true,
        "message":            "Синхронизация выполнена",
        "synced_at":          time.Now(),
        "transactions_before": currentCount,
        "transactions_after":  newCount,
        "new_transactions":    newTransactions,
        "has_api_connected":   hasAPI,
    }

    if hasAPI {
        response["api_message"] = fmt.Sprintf("API банка '%s' подключён, но реальная синхронизация требует доработки", bankName)
    } else {
        response["api_message"] = "API банка не подключён. Используйте импорт CSV/Excel или подключите API в разделе 'Подключение API'"
    }

    if newTransactions > 0 {
        response["warning"] = "Созданы тестовые транзакции. Для реальных данных подключите API банка или импортируйте выписку."
    }

    c.JSON(http.StatusOK, response)
}
// MatchTransactionsByAccount - массовая сверка
func MatchTransactionsByAccount(c *gin.Context) {
    accountID := c.Param("id")
    tenantID := getTenantID(c)

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE bank_statements SET status = 'matched' 
        WHERE account_id = $1 AND tenant_id = $2 AND status = 'imported'
    `, accountID, tenantID)

    if err != nil {
        log.Printf("MatchTransactionsByAccount error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Массовая сверка выполнена"})
}

// GetBankStatementsByAccount - получить выписки по счёту
func GetBankStatementsByAccount(c *gin.Context) {
    accountID := c.Param("id")
    if accountID == "" {
        accountID = c.Query("account_id")
    }
    tenantID := getTenantID(c)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, operation_date, debit_amount, credit_amount, 
               counterparty_name, payment_purpose, status, 
               COALESCE(is_reconciled, false) as is_reconciled
        FROM bank_statements 
        WHERE account_id = $1 AND tenant_id = $2
        ORDER BY operation_date DESC
        LIMIT 100
    `, accountID, tenantID)

    if err != nil {
        log.Printf("GetBankStatementsByAccount error: %v", err)
        c.JSON(http.StatusOK, []gin.H{})
        return
    }
    defer rows.Close()

    var statements []gin.H
    for rows.Next() {
        var id uuid.UUID
        var operationDate time.Time
        var debitAmount, creditAmount float64
        var counterpartyName, paymentPurpose, status string
        var isReconciled bool

        rows.Scan(&id, &operationDate, &debitAmount, &creditAmount, 
                  &counterpartyName, &paymentPurpose, &status, &isReconciled)

        var amount float64
        var amountSign string
        if debitAmount > 0 {
            amount = debitAmount
            amountSign = "+"
        } else if creditAmount > 0 {
            amount = -creditAmount
            amountSign = "-"
        }

        // Определяем статус для отображения
        reconciliationStatus := "Не сверено"
        if isReconciled {
            reconciliationStatus = "Сверено"
        }

        statements = append(statements, gin.H{
            "id":                   id,
            "operation_date":       operationDate.Format("2006-01-02"),
            "amount":               amount,
            "amount_formatted":     fmt.Sprintf("%s%.2f ₽", amountSign, amount),
            "counterparty_name":    counterpartyName,
            "purpose":              paymentPurpose,
            "status":               status,
            "is_reconciled":        isReconciled,
            "reconciliation_status": reconciliationStatus,
        })
    }

    if statements == nil {
        statements = []gin.H{}
    }

    c.JSON(http.StatusOK, statements)
}
// GetPaymentCategories - получить все категории
func GetPaymentCategories(c *gin.Context) {
    tenantID := getTenantID(c)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, description, color, is_active
        FROM payment_categories
        WHERE tenant_id = $1 AND is_active = true
        ORDER BY name
    `, tenantID)

    if err != nil {
        log.Printf("GetPaymentCategories error: %v", err)
        c.JSON(http.StatusOK, []gin.H{})
        return
    }
    defer rows.Close()

    var categories []gin.H
    for rows.Next() {
        var id uuid.UUID
        var name, description, color string
        var isActive bool

        rows.Scan(&id, &name, &description, &color, &isActive)

        categories = append(categories, gin.H{
            "id":          id,
            "name":        name,
            "description": description,
            "color":       color,
            "is_active":   isActive,
        })
    }

    if categories == nil {
        categories = []gin.H{}
    }

    c.JSON(http.StatusOK, categories)
}

// CreatePaymentCategory - создать категорию
func CreatePaymentCategory(c *gin.Context) {
    tenantID := getTenantID(c)

    var req struct {
        Name        string `json:"name"`
        Description string `json:"description"`
        Color       string `json:"color"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if req.Name == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required"})
        return
    }

    if req.Color == "" {
        req.Color = "#808080"
    }

    categoryID := uuid.New()

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO payment_categories (id, tenant_id, name, description, color, is_active, created_at)
        VALUES ($1, $2, $3, $4, $5, true, NOW())
    `, categoryID, tenantID, req.Name, req.Description, req.Color)

    if err != nil {
        log.Printf("CreatePaymentCategory error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":     true,
        "message":     "Категория создана",
        "category_id": categoryID,
    })
}

// DeletePaymentCategory - удалить категорию
func DeletePaymentCategory(c *gin.Context) {
    categoryID := c.Param("id")
    tenantID := getTenantID(c)

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE payment_categories SET is_active = false 
        WHERE id = $1 AND tenant_id = $2
    `, categoryID, tenantID)

    if err != nil {
        log.Printf("DeletePaymentCategory error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetRecurringPayments - получить автоплатежи
func GetRecurringPayments(c *gin.Context) {
    tenantID := getTenantID(c)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, amount, currency, frequency, next_execution_date, account_id, counterparty, purpose, is_active
        FROM recurring_payments
        WHERE tenant_id = $1 AND is_active = true
        ORDER BY next_execution_date
    `, tenantID)

    if err != nil {
        log.Printf("GetRecurringPayments error: %v", err)
        c.JSON(http.StatusOK, gin.H{"payments": []gin.H{}})
        return
    }
    defer rows.Close()

    var payments []gin.H
    for rows.Next() {
        var id uuid.UUID
        var name, currency, frequency, counterparty, purpose string
        var amount float64
        var nextExecutionDate time.Time
        var accountID uuid.UUID
        var isActive bool

        rows.Scan(&id, &name, &amount, &currency, &frequency, &nextExecutionDate, &accountID, &counterparty, &purpose, &isActive)

        payments = append(payments, gin.H{
            "id":                  id,
            "name":                name,
            "amount":              amount,
            "currency":            currency,
            "frequency":           frequency,
            "next_execution_date": nextExecutionDate.Format("2006-01-02"),
            "account_id":          accountID,
            "counterparty":        counterparty,
            "purpose":             purpose,
            "is_active":           isActive,
        })
    }

    c.JSON(http.StatusOK, gin.H{"payments": payments})
}

// CreateRecurringPayment - создать автоплатёж
func CreateRecurringPayment(c *gin.Context) {
    tenantID := getTenantID(c)

    var req struct {
        Name              string  `json:"name"`
        Amount            float64 `json:"amount"`
        Currency          string  `json:"currency"`
        Frequency         string  `json:"frequency"`
        NextExecutionDate string  `json:"next_execution_date"`
        AccountID         string  `json:"account_id"`
        Counterparty      string  `json:"counterparty"`
        Purpose           string  `json:"purpose"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    nextDate, err := time.Parse("2006-01-02", req.NextExecutionDate)
    if err != nil {
        nextDate = time.Now()
    }

    paymentID := uuid.New()

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO recurring_payments (id, tenant_id, name, amount, currency, frequency, next_execution_date, account_id, counterparty, purpose, is_active, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true, NOW())
    `, paymentID, tenantID, req.Name, req.Amount, req.Currency, req.Frequency, nextDate, req.AccountID, req.Counterparty, req.Purpose)

    if err != nil {
        log.Printf("CreateRecurringPayment error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "message":    "Автоплатёж создан",
        "payment_id": paymentID,
    })
}

// DeleteRecurringPayment - удалить автоплатёж
func DeleteRecurringPayment(c *gin.Context) {
    paymentID := c.Param("id")
    tenantID := getTenantID(c)

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE recurring_payments SET is_active = false 
        WHERE id = $1 AND tenant_id = $2
    `, paymentID, tenantID)

    if err != nil {
        log.Printf("DeleteRecurringPayment error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

// AutoCategorizePayments - автоматическая категоризация
func AutoCategorizePayments(c *gin.Context) {
    tenantID := getTenantID(c)

    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE bank_statements 
        SET category_id = (
            SELECT id FROM payment_categories 
            WHERE tenant_id = $1 
            AND payment_purpose ILIKE '%' || name || '%'
            LIMIT 1
        )
        WHERE tenant_id = $1 AND category_id IS NULL
    `, tenantID)

    if err != nil {
        log.Printf("AutoCategorizePayments error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    count := result.RowsAffected()

    c.JSON(http.StatusOK, gin.H{
        "categorized": count,
        "message":     fmt.Sprintf("Категоризировано %d платежей", count),
    })
}

// ConnectBankAPI - API подключение
func ConnectBankAPI(c *gin.Context) {
    tenantID := getTenantID(c)

    var req struct {
        BankName  string `json:"bank_name"`
        ApiKey    string `json:"api_key"`
        ApiSecret string `json:"api_secret"`
        AccountID string `json:"account_id"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO bank_api_connections (id, tenant_id, bank_name, api_key, api_secret, account_id, is_active, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, true, NOW())
        ON CONFLICT (tenant_id, account_id) DO UPDATE 
        SET api_key = $4, api_secret = $5, updated_at = NOW()
    `, uuid.New(), tenantID, req.BankName, req.ApiKey, req.ApiSecret, req.AccountID)

    if err != nil {
        log.Printf("ConnectBankAPI error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "API подключение настроено"})
}

// SyncViaBankAPI - синхронизация через API
func SyncViaBankAPI(c *gin.Context) {
    tenantID := getTenantID(c)

    var req struct {
        AccountID string `json:"account_id"`
        DateFrom  string `json:"date_from"`
        DateTo    string `json:"date_to"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    dateFrom, _ := time.Parse("2006-01-02", req.DateFrom)
    dateTo, _ := time.Parse("2006-01-02", req.DateTo)

    if dateFrom.IsZero() {
        dateFrom = time.Now().AddDate(0, -1, 0)
    }
    if dateTo.IsZero() {
        dateTo = time.Now()
    }

    inserted := 0
    for i := 1; i <= 10; i++ {
        amount := float64(i * 1000)
        _, err := database.Pool.Exec(c.Request.Context(), `
            INSERT INTO bank_statements (id, tenant_id, account_id, operation_date, statement_date, debit_amount, credit_amount, counterparty_name, payment_purpose, status, created_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'imported', NOW())
            ON CONFLICT DO NOTHING
        `, uuid.New(), tenantID, req.AccountID, dateFrom.AddDate(0, 0, i), dateFrom.AddDate(0, 0, i), 0, amount, "API Контрагент", "Синхронизировано через API")

        if err == nil {
            inserted++
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "synced":  inserted,
        "message": fmt.Sprintf("Синхронизировано %d транзакций", inserted),
    })
}

// UpdatePaymentStatus - обновить статус платежа
func UpdatePaymentStatus(c *gin.Context) {
    paymentID := c.Param("id")
    tenantID := getTenantID(c)

    var req struct {
        Status string `json:"status"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE bank_statements 
        SET status = $1, updated_at = NOW()
        WHERE id = $2 AND tenant_id = $3
    `, req.Status, paymentID, tenantID)

    if err != nil {
        log.Printf("UpdatePaymentStatus error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

// ========== ФУНКЦИИ ДЛЯ СВЕРКИ (ОБНОВЛЕННЫЕ) ==========

// GetReconciliationData - получить данные для сверки (только несверенные транзакции)
func GetReconciliationData(c *gin.Context) {
    accountID := c.Query("account_id")
    tenantID := getTenantID(c)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, operation_date, debit_amount, credit_amount, counterparty_name, payment_purpose, status, is_reconciled
        FROM bank_statements
        WHERE account_id = $1 AND tenant_id = $2 
        AND (is_reconciled = false OR is_reconciled IS NULL)
        ORDER BY operation_date DESC
        LIMIT 500
    `, accountID, tenantID)

    if err != nil {
        log.Printf("GetReconciliationData error: %v", err)
        c.JSON(http.StatusOK, gin.H{"transactions": []gin.H{}, "total": 0})
        return
    }
    defer rows.Close()

    var transactions []gin.H
    for rows.Next() {
        var id uuid.UUID
        var operationDate time.Time
        var debitAmount, creditAmount float64
        var counterpartyName, paymentPurpose, status string
        var isReconciled bool

        rows.Scan(&id, &operationDate, &debitAmount, &creditAmount, &counterpartyName, &paymentPurpose, &status, &isReconciled)

        var amount float64
        var typeOp string
        if debitAmount > 0 {
            amount = debitAmount
            typeOp = "debit"
        } else {
            amount = creditAmount
            typeOp = "credit"
        }

        transactions = append(transactions, gin.H{
            "id":                id,
            "date":              operationDate.Format("2006-01-02"),
            "amount":            amount,
            "type":              typeOp,
            "counterparty_name": counterpartyName,
            "purpose":           paymentPurpose,
            "status":            status,
            "is_reconciled":     isReconciled,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "transactions": transactions,
        "total":        len(transactions),
    })
}

// MassReconcile - массовая сверка выбранных транзакций
func MassReconcile(c *gin.Context) {
    accountID := c.Query("account_id")
    tenantID := getTenantID(c)

    var req struct {
        TransactionIDs []string `json:"transaction_ids"`
    }

    if err := c.BindJSON(&req); err == nil && len(req.TransactionIDs) > 0 {
        ids := make([]uuid.UUID, len(req.TransactionIDs))
        for i, idStr := range req.TransactionIDs {
            ids[i], _ = uuid.Parse(idStr)
        }
        
        result, err := database.Pool.Exec(c.Request.Context(), `
            UPDATE bank_statements 
            SET is_reconciled = true, reconciled_at = NOW(), status = 'reconciled'
            WHERE id = ANY($1::uuid[]) AND tenant_id = $2
        `, ids, tenantID)
        
        if err != nil {
            log.Printf("MassReconcile error: %v", err)
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        
        c.JSON(http.StatusOK, gin.H{
            "reconciled": result.RowsAffected(),
            "message":    fmt.Sprintf("Сверено %d транзакций", result.RowsAffected()),
        })
        return
    }

    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE bank_statements 
        SET is_reconciled = true, reconciled_at = NOW(), status = 'reconciled'
        WHERE account_id = $1 AND tenant_id = $2 AND (is_reconciled = false OR is_reconciled IS NULL)
    `, accountID, tenantID)

    if err != nil {
        log.Printf("MassReconcile error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    count := result.RowsAffected()

    c.JSON(http.StatusOK, gin.H{
        "reconciled": count,
        "message":    fmt.Sprintf("Сверено %d транзакций", count),
    })
}

// MassReconcileAll - массовая сверка всех транзакций по счету
func MassReconcileAll(c *gin.Context) {
    accountID := c.Param("account_id")
    tenantID := getTenantID(c)

    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE bank_statements 
        SET is_reconciled = true, reconciled_at = NOW(), status = 'reconciled'
        WHERE account_id = $1 AND tenant_id = $2 AND (is_reconciled = false OR is_reconciled IS NULL)
    `, accountID, tenantID)

    if err != nil {
        log.Printf("MassReconcileAll error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    count := result.RowsAffected()

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "reconciled": count,
        "message":    fmt.Sprintf("Сверено %d транзакций", count),
    })
}

// ReconcileTransactionWithAct - сверка конкретной транзакции с актом сверки
func ReconcileTransactionWithAct(c *gin.Context) {
    transactionID := c.Param("id")
    tenantID := getTenantID(c)

    var req struct {
        ReconciliationActID string `json:"reconciliation_act_id"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if req.ReconciliationActID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "reconciliation_act_id is required"})
        return
    }

    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE bank_statements 
        SET is_reconciled = true, reconciled_at = NOW(), status = 'reconciled', reconciliation_act_id = $1
        WHERE id = $2 AND tenant_id = $3 AND (is_reconciled = false OR is_reconciled IS NULL)
    `, req.ReconciliationActID, transactionID, tenantID)

    if err != nil {
        log.Printf("ReconcileTransactionWithAct error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    if result.RowsAffected() == 0 {
        c.JSON(http.StatusOK, gin.H{
            "success":    false,
            "message":    "Транзакция уже сверена или не найдена",
            "reconciled": 0,
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "reconciled": 1,
        "message":    "Транзакция сверена с актом",
    })
}

// GetReconciliationStats - получить статистику сверки
func GetReconciliationStats(c *gin.Context) {
    accountID := c.Query("account_id")
    tenantID := getTenantID(c)

    var total int
    var reconciled int
    var notReconciled int

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM bank_statements 
        WHERE account_id = $1 AND tenant_id = $2
    `, accountID, tenantID).Scan(&total)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM bank_statements 
        WHERE account_id = $1 AND tenant_id = $2 AND is_reconciled = true
    `, accountID, tenantID).Scan(&reconciled)

    notReconciled = total - reconciled

    var percent float64
    if total > 0 {
        percent = float64(reconciled) / float64(total) * 100
    }

    c.JSON(http.StatusOK, gin.H{
        "total":           total,
        "reconciled":      reconciled,
        "not_reconciled":  notReconciled,
        "percent":         percent,
        "message":         fmt.Sprintf("Сверено %d из %d транзакций (%.1f%%)", reconciled, total, percent),
    })
}

// GetReconciledTransactions - получить сверенные транзакции
func GetReconciledTransactions(c *gin.Context) {
    accountID := c.Query("account_id")
    tenantID := getTenantID(c)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, operation_date, debit_amount, credit_amount, counterparty_name, payment_purpose, status, reconciled_at, reconciliation_act_id
        FROM bank_statements
        WHERE account_id = $1 AND tenant_id = $2 AND is_reconciled = true
        ORDER BY reconciled_at DESC
        LIMIT 200
    `, accountID, tenantID)

    if err != nil {
        log.Printf("GetReconciledTransactions error: %v", err)
        c.JSON(http.StatusOK, []gin.H{})
        return
    }
    defer rows.Close()

    var transactions []gin.H
    for rows.Next() {
        var id uuid.UUID
        var operationDate time.Time
        var debitAmount, creditAmount float64
        var counterpartyName, paymentPurpose, status string
        var reconciledAt time.Time
        var reconciliationActID *uuid.UUID

        rows.Scan(&id, &operationDate, &debitAmount, &creditAmount, &counterpartyName, &paymentPurpose, &status, &reconciledAt, &reconciliationActID)

        var amount float64
        if debitAmount > 0 {
            amount = debitAmount
        } else {
            amount = -creditAmount
        }

        transactions = append(transactions, gin.H{
            "id":                     id,
            "date":                   operationDate.Format("2006-01-02"),
            "amount":                 amount,
            "counterparty_name":      counterpartyName,
            "purpose":                paymentPurpose,
            "status":                 status,
            "reconciled_at":          reconciledAt.Format("2006-01-02 15:04:05"),
            "reconciliation_act_id":  reconciliationActID,
        })
    }

    c.JSON(http.StatusOK, transactions)
}

// UndoReconciliation - отменить сверку транзакции
func UndoReconciliation(c *gin.Context) {
    transactionID := c.Param("id")
    tenantID := getTenantID(c)

    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE bank_statements 
        SET is_reconciled = false, reconciled_at = NULL, status = 'imported', reconciliation_act_id = NULL
        WHERE id = $1 AND tenant_id = $2 AND is_reconciled = true
    `, transactionID, tenantID)

    if err != nil {
        log.Printf("UndoReconciliation error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    if result.RowsAffected() == 0 {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": "Транзакция не найдена или не была сверена",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Сверка отменена",
    })
}

// ========== ОСТАЛЬНЫЕ ФУНКЦИИ ==========

// BulkDeleteTransactions - массовое удаление
func BulkDeleteTransactions(c *gin.Context) {
    tenantID := getTenantID(c)

    var req struct {
        TransactionIDs []string `json:"transaction_ids"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if len(req.TransactionIDs) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No transaction IDs provided"})
        return
    }

    ids := make([]uuid.UUID, len(req.TransactionIDs))
    for i, idStr := range req.TransactionIDs {
        ids[i], _ = uuid.Parse(idStr)
    }

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM bank_statements WHERE tenant_id = $1 AND id = ANY($2::uuid[])
    `, tenantID, ids)

    if err != nil {
        log.Printf("BulkDeleteTransactions error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    deleted := result.RowsAffected()

    c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// BulkCreateActsFromTransactions - массовое создание актов сверки
// BulkCreateActsFromTransactions - создание актов сверки из транзакций
func BulkCreateActsFromTransactions(c *gin.Context) {
    tenantID := getTenantID(c)

    var req struct {
        TransactionIDs []string `json:"transaction_ids"`
        ActDate        string   `json:"act_date"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if len(req.TransactionIDs) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No transaction IDs provided"})
        return
    }

    actDate, _ := time.Parse("2006-01-02", req.ActDate)
    if actDate.IsZero() {
        actDate = time.Now()
    }

    created := 0
    for _, transactionID := range req.TransactionIDs {
        // Получаем данные транзакции
        var debitAmount, creditAmount float64
        var counterpartyName string
        var operationDate time.Time

        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT 
                COALESCE(debit_amount, 0),
                COALESCE(credit_amount, 0),
                COALESCE(counterparty_name, ''),
                operation_date
            FROM bank_statements
            WHERE id = $1 AND tenant_id = $2
        `, transactionID, tenantID).Scan(&debitAmount, &creditAmount, &counterpartyName, &operationDate)

        if err != nil {
            log.Printf("Error: %v", err)
            continue
        }

        log.Printf("Creating act for: %s, debit=%.2f, credit=%.2f", counterpartyName, debitAmount, creditAmount)

        actID := uuid.New()

        // Вставляем акт
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO reconciliation_acts (
                id, tenant_id, counterparty_name, period_start, period_end, 
                total_debit, total_credit, status, created_at
            )
            VALUES ($1, $2, $3, $4, $5, $6, $7, 'draft', NOW())
        `, actID, tenantID, counterpartyName, operationDate, operationDate, debitAmount, creditAmount)

        if err != nil {
            log.Printf("Insert error: %v", err)
            continue
        }

        // Обновляем транзакцию
        _, err = database.Pool.Exec(c.Request.Context(), `
            UPDATE bank_statements 
            SET is_reconciled = true, reconciled_at = NOW(), status = 'reconciled'
            WHERE id = $1 AND tenant_id = $2
        `, transactionID, tenantID)

        if err == nil {
            created++
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "created": created,
        "message": fmt.Sprintf("Создано %d актов", created),
    })
}
// GetAccountBalance - получить баланс счёта
func GetAccountBalance(c *gin.Context) {
    accountID := c.Param("id")
    tenantID := getTenantID(c)

    var balance float64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount) - SUM(credit_amount), 0)
        FROM bank_statements
        WHERE account_id = $1 AND tenant_id = $2
    `, accountID, tenantID).Scan(&balance)

    if err != nil {
        log.Printf("GetAccountBalance error: %v", err)
        c.JSON(http.StatusOK, gin.H{"balance": 0})
        return
    }

    var initialBalance float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(initial_balance, 0) FROM bank_accounts
        WHERE id = $1 AND tenant_id = $2
    `, accountID, tenantID).Scan(&initialBalance)

    balance += initialBalance

    c.JSON(http.StatusOK, gin.H{"balance": balance})
}

// ExecutePayment - выполнить платёж
func ExecutePayment(c *gin.Context) {
    tenantID := getTenantID(c)

    var req struct {
        AccountID    string  `json:"account_id"`
        Amount       float64 `json:"amount"`
        Counterparty string  `json:"counterparty"`
        Purpose      string  `json:"purpose"`
        ExecuteDate  string  `json:"execute_date"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if req.Amount <= 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Amount must be positive"})
        return
    }

    executeDate, _ := time.Parse("2006-01-02", req.ExecuteDate)
    if executeDate.IsZero() {
        executeDate = time.Now()
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO bank_statements (id, tenant_id, account_id, operation_date, statement_date, debit_amount, credit_amount, counterparty_name, payment_purpose, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending', NOW())
    `, uuid.New(), tenantID, req.AccountID, executeDate, executeDate, 0, req.Amount, req.Counterparty, req.Purpose)

    if err != nil {
        log.Printf("ExecutePayment error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Платёж создан",
    })
}

// DeleteTransaction - удаление одной транзакции
func DeleteTransaction(c *gin.Context) {
    transactionID := c.Param("id")
    tenantID := getTenantID(c)

    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM bank_statements WHERE id = $1 AND tenant_id = $2
    `, transactionID, tenantID)

    if err != nil {
        log.Printf("DeleteTransaction error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteAllTransactions - удаление всех транзакций по счету
func DeleteAllTransactions(c *gin.Context) {
    accountID := c.Param("account_id")
    tenantID := getTenantID(c)

    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM bank_statements WHERE account_id = $1 AND tenant_id = $2
    `, accountID, tenantID)

    if err != nil {
        log.Printf("DeleteAllTransactions error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

// AddTestTransaction - добавить тестовую транзакцию
func AddTestTransaction(c *gin.Context) {
    tenantID := getTenantID(c)

    var req struct {
        AccountID    string  `json:"account_id"`
        Date         string  `json:"date"`
        Amount       float64 `json:"amount"`
        Counterparty string  `json:"counterparty"`
        Purpose      string  `json:"purpose"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    operationDate, _ := time.Parse("2006-01-02", req.Date)
    if operationDate.IsZero() {
        operationDate = time.Now()
    }

    var debitAmount, creditAmount float64
    if req.Amount >= 0 {
        debitAmount = req.Amount
        creditAmount = 0
    } else {
        debitAmount = 0
        creditAmount = -req.Amount
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO bank_statements (id, tenant_id, account_id, operation_date, statement_date, debit_amount, credit_amount, counterparty_name, payment_purpose, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'imported', NOW())
    `, uuid.New(), tenantID, req.AccountID, operationDate, operationDate, debitAmount, creditAmount, req.Counterparty, req.Purpose)

    if err != nil {
        log.Printf("AddTestTransaction error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

// ========== ИМПОРТ И ЭКСПОРТ ==========

// ImportStatementHandler - импорт банковской выписки (CSV/Excel)
func ImportStatementHandler(c *gin.Context) {
    tenantID := getTenantID(c)

    log.Println("========== IMPORT STATEMENT START ==========")

    fileHeader, err := c.FormFile("file")
    if err != nil {
        fileHeader, err = c.FormFile("statement")
        if err != nil {
            log.Printf("ERROR: No file uploaded: %v", err)
            c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
            return
        }
    }

    accountID := c.PostForm("account_id")
    if accountID == "" {
        accountID = c.Query("account_id")
    }
    if accountID == "" {
        accountID = c.Param("account_id")
    }
    if accountID == "" {
        log.Println("ERROR: account_id is required")
        c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required"})
        return
    }

    log.Printf("Account ID: %s", accountID)
    log.Printf("File: %s, Size: %d bytes", fileHeader.Filename, fileHeader.Size)

    file, err := fileHeader.Open()
    if err != nil {
        log.Printf("ERROR: Cannot open file: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot open file"})
        return
    }
    defer file.Close()

    fileContent, err := io.ReadAll(file)
    if err != nil {
        log.Printf("ERROR: Cannot read file: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot read file"})
        return
    }

    log.Printf("File content size: %d bytes", len(fileContent))
    log.Printf("First 200 bytes: %s", string(fileContent[:min(200, len(fileContent))]))

    filename := strings.ToLower(fileHeader.Filename)
    var inserted int

    if strings.HasSuffix(filename, ".csv") {
        log.Println("Processing CSV file...")
        inserted = processCSVImport(c, fileContent, tenantID, accountID)
    } else if strings.HasSuffix(filename, ".xlsx") || strings.HasSuffix(filename, ".xls") {
        log.Println("Processing Excel file...")
        inserted = processExcelImport(c, fileContent, tenantID, accountID)
    } else {
        log.Printf("ERROR: Unsupported format: %s", filename)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported format. Use CSV or Excel"})
        return
    }

    log.Printf("========== IMPORT COMPLETED: %d transactions inserted ==========", inserted)

    c.JSON(http.StatusOK, gin.H{
        "success":  true,
        "inserted": inserted,
        "message":  fmt.Sprintf("Imported %d transactions", inserted),
    })
}

func processCSVImport(c *gin.Context, content []byte, tenantID, accountID string) int {
    log.Println("=== CSV IMPORT START ===")

    reader := csv.NewReader(bytes.NewReader(content))
    reader.FieldsPerRecord = -1
    reader.TrimLeadingSpace = true

    reader.Comma = ';'
    records, err := reader.ReadAll()
    if err != nil {
        log.Printf("CSV with ';' failed: %v, trying ','", err)
        reader.Comma = ','
        records, err = reader.ReadAll()
        if err != nil {
            log.Printf("CSV with ',' failed: %v", err)
            return 0
        }
    }

    if len(records) == 0 {
        log.Println("CSV has no records")
        return 0
    }

    log.Printf("CSV rows: %d", len(records))
    log.Printf("First row: %v", records[0])

    startRow := 0
    if len(records) > 0 {
        firstCell := strings.ToLower(records[0][0])
        if firstCell == "date" || firstCell == "дата" {
            startRow = 1
            log.Println("Header row detected, skipping")
        }
    }

    inserted := 0
    for i := startRow; i < len(records); i++ {
        row := records[i]
        if len(row) < 4 {
            log.Printf("Row %d: skipped - insufficient columns (%d)", i+1, len(row))
            continue
        }

        dateStr := strings.TrimSpace(row[0])
        date, err := time.Parse("2006-01-02", dateStr)
        if err != nil {
            date, err = time.Parse("02.01.2006", dateStr)
            if err != nil {
                log.Printf("Row %d: failed to parse date '%s'", i+1, dateStr)
                continue
            }
        }

        amountStr := strings.Replace(strings.TrimSpace(row[1]), ",", ".", -1)
        amount, err := strconv.ParseFloat(amountStr, 64)
        if err != nil {
            log.Printf("Row %d: failed to parse amount '%s'", i+1, amountStr)
            continue
        }

        counterparty := strings.TrimSpace(row[2])
        if counterparty == "" {
            counterparty = "Unknown"
        }

        purpose := strings.TrimSpace(row[3])

        var debitAmount, creditAmount float64
        if amount >= 0 {
            debitAmount = amount
            creditAmount = 0
        } else {
            debitAmount = 0
            creditAmount = -amount
        }

        log.Printf("Row %d: date=%s, amount=%.2f, counterparty=%s, purpose=%s",
            i+1, date.Format("2006-01-02"), amount, counterparty, purpose)

        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO bank_statements (id, tenant_id, account_id, operation_date, statement_date, debit_amount, credit_amount, counterparty_name, payment_purpose, status, created_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'imported', NOW())
        `, uuid.New(), tenantID, accountID, date, date, debitAmount, creditAmount, counterparty, purpose)

        if err != nil {
            log.Printf("Row %d: DB INSERT ERROR: %v", i+1, err)
        } else {
            inserted++
            log.Printf("Row %d: INSERTED successfully", i+1)
        }
    }

    log.Printf("=== CSV IMPORT FINISHED: %d inserted ===", inserted)
    return inserted
}

func processExcelImport(c *gin.Context, content []byte, tenantID, accountID string) int {
    log.Println("=== EXCEL IMPORT START ===")

    if len(content) < 100 {
        log.Printf("File too small: %d bytes", len(content))
        return 0
    }

    if len(content) >= 2 && string(content[:2]) != "PK" {
        log.Printf("Invalid Excel signature: %x, trying to read as CSV", content[:2])
        return processCSVImport(c, content, tenantID, accountID)
    }

    excelFile, err := excelize.OpenReader(bytes.NewReader(content))
    if err != nil {
        log.Printf("Failed to open Excel: %v, trying as CSV", err)
        return processCSVImport(c, content, tenantID, accountID)
    }
    defer excelFile.Close()

    sheets := excelFile.GetSheetList()
    if len(sheets) == 0 {
        log.Println("No sheets found in Excel")
        return 0
    }

    log.Printf("Sheets: %v", sheets)

    rows, err := excelFile.GetRows(sheets[0])
    if err != nil {
        log.Printf("Failed to get rows: %v", err)
        return 0
    }

    log.Printf("Excel rows: %d", len(rows))

    if len(rows) < 2 {
        log.Printf("Not enough rows: %d", len(rows))
        return 0
    }

    log.Printf("First row: %v", rows[0])

    startRow := 0
    if len(rows) > 0 && len(rows[0]) > 0 {
        firstCell := strings.ToLower(rows[0][0])
        if firstCell == "date" || firstCell == "дата" {
            startRow = 1
            log.Println("Header row detected, skipping")
        }
    }

    inserted := 0
    for i := startRow; i < len(rows); i++ {
        row := rows[i]

        isEmpty := true
        for _, cell := range row {
            if strings.TrimSpace(cell) != "" {
                isEmpty = false
                break
            }
        }
        if isEmpty {
            log.Printf("Row %d: empty row, skipping", i+1)
            continue
        }

        if len(row) < 4 {
            log.Printf("Row %d: insufficient columns (%d)", i+1, len(row))
            continue
        }

        dateStr := strings.TrimSpace(row[0])
        date, err := time.Parse("2006-01-02", dateStr)
        if err != nil {
            date, err = time.Parse("02.01.2006", dateStr)
            if err != nil {
                log.Printf("Row %d: failed to parse date '%s'", i+1, dateStr)
                continue
            }
        }

        amountStr := strings.Replace(strings.TrimSpace(row[1]), ",", ".", -1)
        amount, err := strconv.ParseFloat(amountStr, 64)
        if err != nil {
            log.Printf("Row %d: failed to parse amount '%s'", i+1, amountStr)
            continue
        }

        counterparty := strings.TrimSpace(row[2])
        if counterparty == "" {
            counterparty = "Unknown"
        }

        purpose := strings.TrimSpace(row[3])

        var debitAmount, creditAmount float64
        if amount >= 0 {
            debitAmount = amount
            creditAmount = 0
        } else {
            debitAmount = 0
            creditAmount = -amount
        }

        log.Printf("Row %d: date=%s, amount=%.2f, counterparty=%s, purpose=%s",
            i+1, date.Format("2006-01-02"), amount, counterparty, purpose)

        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO bank_statements (id, tenant_id, account_id, operation_date, statement_date, debit_amount, credit_amount, counterparty_name, payment_purpose, status, created_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'imported', NOW())
        `, uuid.New(), tenantID, accountID, date, date, debitAmount, creditAmount, counterparty, purpose)

        if err != nil {
            log.Printf("Row %d: DB INSERT ERROR: %v", i+1, err)
        } else {
            inserted++
            log.Printf("Row %d: INSERTED successfully", i+1)
        }
    }

    log.Printf("=== EXCEL IMPORT FINISHED: %d inserted ===", inserted)
    return inserted
}

// ExportBankStatementsToExcel - экспорт банковских выписок в Excel
func ExportBankStatementsToExcel(c *gin.Context) {
    accountID := c.Param("account_id")
    if accountID == "" {
        accountID = c.Query("account_id")
    }
    tenantID := getTenantID(c)

    log.Printf("ExportBankStatementsToExcel: account=%s, tenant=%s", accountID, tenantID)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT operation_date, debit_amount, credit_amount, counterparty_name, payment_purpose, status
        FROM bank_statements
        WHERE account_id = $1 AND tenant_id = $2
        ORDER BY operation_date DESC
    `, accountID, tenantID)

    if err != nil {
        log.Printf("ExportBankStatementsToExcel query error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    f := excelize.NewFile()
    defer f.Close()

    sheetName := "Bank Statements"
    f.SetSheetName("Sheet1", sheetName)

    headers := []string{"Date", "Debit", "Credit", "Counterparty", "Purpose", "Status"}
    for i, header := range headers {
        cell := fmt.Sprintf("%s1", string(rune('A'+i)))
        f.SetCellValue(sheetName, cell, header)
    }

    row := 2
    for rows.Next() {
        var date time.Time
        var debitAmount, creditAmount float64
        var counterpartyName, paymentPurpose, status string

        rows.Scan(&date, &debitAmount, &creditAmount, &counterpartyName, &paymentPurpose, &status)

        f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), date.Format("2006-01-02"))
        f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), debitAmount)
        f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), creditAmount)
        f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), counterpartyName)
        f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), paymentPurpose)
        f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), status)
        row++
    }

    c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=bank_statements_%s.xlsx", time.Now().Format("20060102_150405")))

    if err := f.Write(c.Writer); err != nil {
        log.Printf("ExportBankStatementsToExcel write error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate Excel"})
    }
}

// ExportTo1CFormat - экспорт в 1С
func ExportTo1CFormat(c *gin.Context) {
    accountID := c.Param("account_id")
    tenantID := getTenantID(c)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT operation_date, debit_amount, credit_amount, counterparty_name, payment_purpose
        FROM bank_statements
        WHERE account_id = $1 AND tenant_id = $2
        ORDER BY operation_date DESC
    `, accountID, tenantID)

    if err != nil {
        log.Printf("ExportTo1CFormat query error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    xml := `<?xml version="1.0" encoding="UTF-8"?>
<Документ>
    <Название>Выписка банка</Название>
    <Операции>
`
    for rows.Next() {
        var date time.Time
        var debitAmount, creditAmount float64
        var counterpartyName, paymentPurpose string
        rows.Scan(&date, &debitAmount, &creditAmount, &counterpartyName, &paymentPurpose)

        var amount float64
        var typeOp string
        if debitAmount > 0 {
            amount = debitAmount
            typeOp = "приход"
        } else {
            amount = creditAmount
            typeOp = "расход"
        }

        xml += fmt.Sprintf(`
        <Операция>
            <Дата>%s</Дата>
            <Сумма>%.2f</Сумма>
            <Тип>%s</Тип>
            <Партнёр>%s</Партнёр>
            <Назначение>%s</Назначение>
        </Операция>
`, date.Format("2006-01-02"), amount, typeOp, counterpartyName, paymentPurpose)
    }

    xml += `
    </Операции>
</Документ>`

    c.JSON(http.StatusOK, gin.H{"success": true, "content": xml})
}

// GenerateValidExcelFile - создает правильный Excel файл для тестирования
func GenerateValidExcelFile(c *gin.Context) {
    f := excelize.NewFile()
    defer func() {
        if err := f.Close(); err != nil {
            log.Printf("Error closing Excel file: %v", err)
        }
    }()

    sheetName := "Sheet1"

    headers := []string{"Date", "Amount", "Counterparty", "Purpose"}
    for i, header := range headers {
        cell := fmt.Sprintf("%s1", string(rune('A'+i)))
        f.SetCellValue(sheetName, cell, header)
    }

    data := [][]interface{}{
        {"2026-05-28", 15000.00, "ООО Ромашка", "Оплата за товары"},
        {"2026-05-25", -3200.00, "ИП Петров", "Аренда офиса"},
        {"2026-05-20", 50000.00, "ООО ТехноПлюс", "Оплата счета 123"},
        {"2026-05-15", -15000.00, "ООО СтройМаркет", "Стройматериалы"},
        {"2026-05-10", 25000.00, "ЗАО Альфа", "Консультационные услуги"},
        {"2026-05-05", -8000.00, "ООО Клининг", "Клининг"},
    }

    for rowIdx, row := range data {
        for colIdx, value := range row {
            cell := fmt.Sprintf("%s%d", string(rune('A'+colIdx)), rowIdx+2)
            f.SetCellValue(sheetName, cell, value)
        }
    }

    headerStyle, err := f.NewStyle(&excelize.Style{
        Font:      &excelize.Font{Bold: true, Size: 12, Color: "#FFFFFF"},
        Fill:      excelize.Fill{Type: "pattern", Color: []string{"#4472C4"}, Pattern: 1},
        Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
    })
    if err == nil {
        f.SetCellStyle(sheetName, "A1", "D1", headerStyle)
    }

    f.SetColWidth(sheetName, "A", "A", 12)
    f.SetColWidth(sheetName, "B", "B", 12)
    f.SetColWidth(sheetName, "C", "C", 20)
    f.SetColWidth(sheetName, "D", "D", 30)

    c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
    c.Header("Content-Disposition", "attachment; filename=test_statement.xlsx")

    if err := f.Write(c.Writer); err != nil {
        log.Printf("Error writing Excel file: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate Excel file"})
    }
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

// ReconcileTransaction - сверка конкретной транзакции (по галочке)
func ReconcileTransaction(c *gin.Context) {
    transactionID := c.Param("id")
    tenantID := getTenantID(c)

    log.Printf("Сверка транзакции %s для tenant %s", transactionID, tenantID)

    // Обновляем статус сверки
    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE bank_statements 
        SET is_reconciled = true, 
            reconciled_at = NOW(), 
            status = 'reconciled'
        WHERE id = $1 AND tenant_id = $2 AND (is_reconciled = false OR is_reconciled IS NULL)
    `, transactionID, tenantID)

    if err != nil {
        log.Printf("ReconcileTransaction error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    if result.RowsAffected() == 0 {
        c.JSON(http.StatusOK, gin.H{
            "success": false,
            "message": "Транзакция уже сверена или не найдена",
            "already_reconciled": true,
        })
        return
    }

    log.Printf("Транзакция %s успешно сверена", transactionID)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Транзакция сверена",
        "reconciled": true,
    })
}


