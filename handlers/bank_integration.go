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

// Подключение к банку
func ConnectBankAccount(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    var req struct {
        BankName      string `json:"bank_name" binding:"required"`
        AccountNumber string `json:"account_number" binding:"required"`
        BIC           string `json:"bic" binding:"required"`
        APIKey        string `json:"api_key" binding:"required"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    accountID := uuid.New()
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO bank_accounts (id, company_id, bank_name, account_number, bic, api_key, is_active, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, true, NOW())
    `, accountID, companyID, req.BankName, req.AccountNumber, req.BIC, req.APIKey)
    
    if err != nil {
        log.Printf("❌ Ошибка подключения банка: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect bank account"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "message":    "Банковский счёт подключён",
        "account_id": accountID,
    })
}

// Синхронизация выписок
func SyncBankStatements(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    accountID := c.Param("id")
    
    // Получаем данные счёта
    var bankName, apiKey string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT bank_name, api_key FROM bank_accounts WHERE id = $1 AND company_id = $2
    `, accountID, companyID).Scan(&bankName, &apiKey)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
        return
    }
    
    // Имитация получения данных из API банка
    transactions := simulateBankTransactions(bankName, apiKey)
    
    // Сохраняем транзакции в БД
    for _, t := range transactions {
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO bank_transactions (id, account_id, transaction_date, amount, description, counterparty, purpose, created_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
            ON CONFLICT (id) DO NOTHING
        `, uuid.New(), accountID, t.Date, t.Amount, t.Description, t.Counterparty, t.Purpose)
        
        if err != nil {
            log.Printf("⚠️ Ошибка сохранения транзакции: %v", err)
        }
    }
    
    c.JSON(http.StatusOK, gin.H{
        "message": fmt.Sprintf("Синхронизировано %d транзакций", len(transactions)),
        "count":   len(transactions),
    })
}

// Автоматическое сопоставление с проводками
func AutoMatchTransactions(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    // Получаем несопоставленные транзакции
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT bt.id, bt.amount, bt.description, bt.counterparty, bt.purpose
        FROM bank_transactions bt
        JOIN bank_accounts ba ON bt.account_id = ba.id
        WHERE ba.company_id = $1 AND bt.is_matched = false
    `, companyID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load transactions"})
        return
    }
    defer rows.Close()
    
    matched := 0
    for rows.Next() {
        var id uuid.UUID
        var amount float64
        var description, counterparty, purpose string
        
        rows.Scan(&id, &amount, &description, &counterparty, &purpose)
        
        // Ищем соответствующую проводку в журнале
        var entryID uuid.UUID
        err = database.Pool.QueryRow(c.Request.Context(), `
            SELECT id FROM journal_entries 
            WHERE company_id = $1 AND amount = $2 AND description ILIKE $3
            LIMIT 1
        `, companyID, amount, "%"+counterparty+"%").Scan(&entryID)
        
        if err == nil {
            // Сопоставляем
            database.Pool.Exec(c.Request.Context(), `
                UPDATE bank_transactions SET is_matched = true, matched_entry_id = $1
                WHERE id = $2
            `, entryID, id)
            matched++
        }
    }
    
    c.JSON(http.StatusOK, gin.H{
        "message": fmt.Sprintf("Сопоставлено %d транзакций", matched),
        "matched": matched,
    })
}

// Получение выписок
func GetBankStatements(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    startDate := c.Query("start_date")
    endDate := c.Query("end_date")
    
    query := `
        SELECT bt.id, bt.transaction_date, bt.amount, bt.description, bt.counterparty, bt.purpose, bt.is_matched
        FROM bank_transactions bt
        JOIN bank_accounts ba ON bt.account_id = ba.id
        WHERE ba.company_id = $1
    `
    args := []interface{}{companyID}
    argPos := 2
    
    if startDate != "" {
        query += fmt.Sprintf(" AND bt.transaction_date >= $%d", argPos)
        args = append(args, startDate)
        argPos++
    }
    if endDate != "" {
        query += fmt.Sprintf(" AND bt.transaction_date <= $%d", argPos)
        args = append(args, endDate)
        argPos++
    }
    query += " ORDER BY bt.transaction_date DESC"
    
    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load statements"})
        return
    }
    defer rows.Close()
    
    var transactions []gin.H
    for rows.Next() {
        var id uuid.UUID
        var date time.Time
        var amount float64
        var description, counterparty, purpose string
        var isMatched bool
        
        rows.Scan(&id, &date, &amount, &description, &counterparty, &purpose, &isMatched)
        
        transactions = append(transactions, gin.H{
            "id":          id,
            "date":        date.Format("2006-01-02"),
            "amount":      amount,
            "description": description,
            "counterparty": counterparty,
            "purpose":     purpose,
            "is_matched":  isMatched,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "transactions": transactions,
        "total":        len(transactions),
    })
}

// Тип для имитации транзакций
type simTransaction struct {
    Date         time.Time
    Amount       float64
    Description  string
    Counterparty string
    Purpose      string
}

// Имитация данных из банка
func simulateBankTransactions(bankName, apiKey string) []simTransaction {
    // Имитация получения данных из API банка
    return []simTransaction{
        {
            Date:         time.Now().AddDate(0, 0, -1),
            Amount:       150000.00,
            Description:  "Поступление от ООО Ромашка",
            Counterparty: "ООО Ромашка",
            Purpose:      "Оплата по договору №123",
        },
        {
            Date:         time.Now().AddDate(0, 0, -2),
            Amount:       -50000.00,
            Description:  "Оплата поставщику",
            Counterparty: "ИП Иванов",
            Purpose:      "Товары по накладной №456",
        },
        {
            Date:         time.Now().AddDate(0, 0, -3),
            Amount:       250000.00,
            Description:  "Поступление от ООО Лютик",
            Counterparty: "ООО Лютик",
            Purpose:      "Аванс по договору",
        },
    }
}

// GetBankAccounts - получить список счетов
func GetBankAccounts(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, bank_name, account_number, bic, is_active, last_sync
        FROM bank_accounts WHERE company_id = $1
    `, companyID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load accounts"})
        return
    }
    defer rows.Close()
    
    var accounts []gin.H
    for rows.Next() {
        var id uuid.UUID
        var bankName, accountNumber, bic string
        var isActive bool
        var lastSync *time.Time
        
        rows.Scan(&id, &bankName, &accountNumber, &bic, &isActive, &lastSync)
        
        accounts = append(accounts, gin.H{
            "id": id,
            "bank_name": bankName,
            "account_number": accountNumber,
            "bic": bic,
            "is_active": isActive,
            "last_sync": lastSync,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{"accounts": accounts})
}

// GetBankStatementsByAccount - получить выписки по счёту
func GetBankStatementsByAccount(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    accountID := c.Query("account_id")
    startDate := c.Query("start_date")
    endDate := c.Query("end_date")
    
    query := `
        SELECT bt.id, bt.transaction_date, bt.amount, bt.description, bt.counterparty, bt.purpose, bt.is_matched
        FROM bank_transactions bt
        JOIN bank_accounts ba ON bt.account_id = ba.id
        WHERE ba.company_id = $1 AND ba.id = $2
    `
    args := []interface{}{companyID, accountID}
    argPos := 3
    
    if startDate != "" {
        query += fmt.Sprintf(" AND bt.transaction_date >= $%d", argPos)
        args = append(args, startDate)
        argPos++
    }
    if endDate != "" {
        query += fmt.Sprintf(" AND bt.transaction_date <= $%d", argPos)
        args = append(args, endDate)
        argPos++
    }
    query += " ORDER BY bt.transaction_date DESC"
    
    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load statements"})
        return
    }
    defer rows.Close()
    
    var transactions []gin.H
    for rows.Next() {
        var id uuid.UUID
        var date time.Time
        var amount float64
        var description, counterparty, purpose string
        var isMatched bool
        
        rows.Scan(&id, &date, &amount, &description, &counterparty, &purpose, &isMatched)
        
        transactions = append(transactions, gin.H{
            "id": id,
            "date": date.Format("2006-01-02"),
            "amount": amount,
            "description": description,
            "counterparty": counterparty,
            "purpose": purpose,
            "is_matched": isMatched,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{"transactions": transactions, "total": len(transactions)})
}

// MatchTransactionsByAccount - сопоставить транзакции по счёту
func MatchTransactionsByAccount(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    accountID := c.Param("id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT bt.id, bt.amount, bt.description, bt.counterparty, bt.purpose
        FROM bank_transactions bt
        JOIN bank_accounts ba ON bt.account_id = ba.id
        WHERE ba.company_id = $1 AND ba.id = $2 AND bt.is_matched = false
    `, companyID, accountID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load transactions"})
        return
    }
    defer rows.Close()
    
    matched := 0
    for rows.Next() {
        var id uuid.UUID
        var amount float64
        var description, counterparty, purpose string
        
        rows.Scan(&id, &amount, &description, &counterparty, &purpose)
        
        var entryID uuid.UUID
        err = database.Pool.QueryRow(c.Request.Context(), `
            SELECT id FROM journal_entries 
            WHERE company_id = $1 AND amount = $2 AND description ILIKE $3
            LIMIT 1
        `, companyID, amount, "%"+counterparty+"%").Scan(&entryID)
        
        if err == nil {
            database.Pool.Exec(c.Request.Context(), `
                UPDATE bank_transactions SET is_matched = true, matched_entry_id = $1
                WHERE id = $2
            `, entryID, id)
            matched++
        }
    }
    
    c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Сопоставлено %d транзакций", matched), "matched": matched})
}// ExportBankStatementsToExcel - экспорт выписок в Excel
func ExportBankStatementsToExcel(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    accountID := c.Query("account_id")
    startDate := c.Query("start_date")
    endDate := c.Query("end_date")
    
    if accountID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "account_id required"})
        return
    }
    
    if startDate == "" {
        startDate = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
    }
    if endDate == "" {
        endDate = time.Now().Format("2006-01-02")
    }
    
    // Получаем информацию о счёте
    var bankName, accountNumber string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT bank_name, account_number FROM bank_accounts 
        WHERE id = $1 AND company_id = $2
    `, accountID, companyID).Scan(&bankName, &accountNumber)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
        return
    }
    
    // Получаем транзакции
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT transaction_date, amount, counterparty, purpose, is_matched
        FROM bank_transactions
        WHERE account_id = $1 AND transaction_date BETWEEN $2 AND $3
        ORDER BY transaction_date DESC
    `, accountID, startDate, endDate)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    // Формируем HTML для Excel
    html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Банковская выписка</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        h1 { color: #333; }
        .header { margin-bottom: 30px; }
        .info { margin-bottom: 20px; color: #666; }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { border: 1px solid #ddd; padding: 12px; text-align: left; }
        th { background-color: #4472C4; color: white; }
        .matched { color: green; }
        .unmatched { color: orange; }
        .total { font-weight: bold; background-color: #f9f9f9; }
    </style>
</head>
<body>
    <h1>Банковская выписка</h1>
    <div class="header">
        <div class="info"><strong>Банк:</strong> ` + bankName + `</div>
        <div class="info"><strong>Счёт:</strong> ` + accountNumber + `</div>
        <div class="info"><strong>Период:</strong> ` + startDate + ` - ` + endDate + `</div>
        <div class="info"><strong>Дата формирования:</strong> ` + time.Now().Format("2006-01-02 15:04:05") + `</div>
    </div>
    <table>
        <thead>
            <tr><th>Дата</th><th>Сумма, ₽</th><th>Контрагент</th><th>Назначение</th><th>Статус</th></tr>
        </thead>
        <tbody>`
    
    var totalIncome, totalExpense float64
    for rows.Next() {
        var date time.Time
        var amount float64
        var counterparty, purpose string
        var isMatched bool
        
        rows.Scan(&date, &amount, &counterparty, &purpose, &isMatched)
        
        if amount > 0 {
            totalIncome += amount
        } else {
            totalExpense += -amount
        }
        
        status := "⏳ Не сопоставлено"
        if isMatched {
            status = "✅ Сопоставлено"
        }
        
        html += fmt.Sprintf(`
            <tr>
                <td>%s</td>
                <td style="text-align: right">%.2f</td>
                <td>%s</td>
                <td>%s</td>
                <td>%s</td>
            </tr>
        `, date.Format("2006-01-02"), amount, counterparty, purpose, status)
    }
    
    balance := totalIncome - totalExpense
    
    html += fmt.Sprintf(`
            <tr class="total">
                <td colspan="4" style="text-align: right"><strong>Итого поступления:</strong></td>
                <td style="text-align: right"><strong>%.2f ₽</strong></td>
            </tr>
            <tr class="total">
                <td colspan="4" style="text-align: right"><strong>Итого списания:</strong></td>
                <td style="text-align: right"><strong>%.2f ₽</strong></td>
            </tr>
            <tr class="total">
                <td colspan="4" style="text-align: right"><strong>Остаток:</strong></td>
                <td style="text-align: right"><strong>%.2f ₽</strong></td>
            </tr>
        </tbody>
    </table>
    <div style="margin-top: 30px; text-align: center; color: #999; font-size: 12px;">
        Сформировано в FinCore | SaaSPro ERP
    </div>
</body>
</html>`, totalIncome, totalExpense, balance)
    
    filename := fmt.Sprintf("bank_statement_%s_%s_%s.xls", accountID[:8], startDate, endDate)
    c.Header("Content-Type", "application/vnd.ms-excel")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
    c.String(http.StatusOK, html)
}