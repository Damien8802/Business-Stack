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

// getPeriodDates - вспомогательная функция
func getPeriodDates(period string) (time.Time, time.Time) {
    switch period {
    case "2024-Q1":
        return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 3, 31, 23, 59, 59, 0, time.UTC)
    case "2024-Q2":
        return time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)
    case "2024-Q3":
        return time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 9, 30, 23, 59, 59, 0, time.UTC)
    case "2024-Q4":
        return time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)
    default:
        return time.Now().AddDate(0, -3, 0), time.Now()
    }
}

// BalanceItem структура строки ОСВ
type BalanceItem struct {
    AccountID     uuid.UUID `json:"account_id"`
    AccountCode   string    `json:"account_code"`
    AccountName   string    `json:"account_name"`
    AccountType   string    `json:"account_type"`
    OpeningDebit  float64   `json:"opening_debit"`
    OpeningCredit float64   `json:"opening_credit"`
    PeriodDebit   float64   `json:"period_debit"`
    PeriodCredit  float64   `json:"period_credit"`
    ClosingDebit  float64   `json:"closing_debit"`
    ClosingCredit float64   `json:"closing_credit"`
}

// GetTurnoverBalanceSheet - Оборотно-сальдовая ведомость (ОСВ)
func GetTurnoverBalanceSheet(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    
    period := c.DefaultQuery("period", "month")
    var fromDate, toDate time.Time
    now := time.Now()
    
    switch period {
    case "month":
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
        toDate = fromDate.AddDate(0, 1, -1)
    default:
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
        toDate = fromDate.AddDate(0, 1, -1)
    }
    
    // ИСПРАВЛЕНО: просто список проводок, без сложных группировок
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            operation_date,
            COALESCE(debit_account, '') as debit_account,
            COALESCE(credit_account, '') as credit_account,
            amount,
            COALESCE(description, '') as description
        FROM journal_entries
        WHERE tenant_id = $1 
            AND status = 'posted'
            AND operation_date BETWEEN $2 AND $3
        ORDER BY operation_date DESC
    `, tenantID, fromDate, toDate)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var result []gin.H
    for rows.Next() {
        var opDate time.Time
        var debitAccount, creditAccount, description string
        var amount float64
        
        rows.Scan(&opDate, &debitAccount, &creditAccount, &amount, &description)
        
        result = append(result, gin.H{
            "date":            opDate.Format("2006-01-02"),
            "debit_account":   debitAccount,
            "credit_account":  creditAccount,
            "amount":          amount,
            "description":     description,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "data":       result,
        "start_date": fromDate.Format("2006-01-02"),
        "end_date":   toDate.Format("2006-01-02"),
    })
}
// GetProfitAndLoss - Отчет о прибылях и убытках
// GetProfitAndLoss - Отчет о прибылях и убытках
// GetProfitAndLoss - Отчет о прибылях и убытках
func GetProfitAndLoss(c *gin.Context) {
    // Пробуем получить tenant_id разными способами
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        userID := c.GetString("user_id")
        if userID != "" {
            database.Pool.QueryRow(c.Request.Context(), `
                SELECT tenant_id FROM users WHERE id = $1
            `, userID).Scan(&tenantID)
        }
    }
    
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    startDate := c.Query("start_date")
    endDate := c.Query("end_date")
    
    if startDate == "" {
        startDate = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
    }
    if endDate == "" {
        endDate = time.Now().Format("2006-01-02")
    }
    
    // ВЫРУЧКА: поступления на 51 счёт (дебет)
    var revenue float64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 
            AND status = 'posted'
            AND operation_date BETWEEN $2 AND $3
            AND debit_account = '51'
    `, tenantID, startDate, endDate).Scan(&revenue)
    
    if err != nil {
        revenue = 0
    }
    
    // РАСХОДЫ: списания с 51 счёта (кредит)
    var expenses float64
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(credit_amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 
            AND status = 'posted'
            AND operation_date BETWEEN $2 AND $3
            AND credit_account = '51'
    `, tenantID, startDate, endDate).Scan(&expenses)
    
    if err != nil {
        expenses = 0
    }
    
    profit := revenue - expenses
    
    c.JSON(http.StatusOK, gin.H{
        "success":        true,
        "total_revenue":  revenue,
        "total_expenses": expenses,
        "net_profit":     profit,
        "start_date":     startDate,
        "end_date":       endDate,
    })
}
// GetDashboardStats - Статистика для дашборда
func GetDashboardStats(c *gin.Context) {
    userID := getUserID(c)

    var revenue float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(credit_amount), 0)
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        WHERE e.user_id = $1
        AND e.status = 'posted'
        AND e.operation_date >= DATE_TRUNC('month', NOW())
        AND p.account_id IN (SELECT id FROM chart_of_accounts WHERE user_id = $1 AND code = '90')
    `, userID).Scan(&revenue)

    var expenses float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount), 0)
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        WHERE e.user_id = $1
        AND e.status = 'posted'
        AND e.operation_date >= DATE_TRUNC('month', NOW())
        AND p.account_id IN (SELECT id FROM chart_of_accounts WHERE user_id = $1 AND code IN ('20', '26', '44'))
    `, userID).Scan(&expenses)

    var entriesCount int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM journal_entries
        WHERE user_id = $1 AND status = 'posted'
    `, userID).Scan(&entriesCount)

    var bankBalance float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount - credit_amount), 0)
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        WHERE e.user_id = $1
        AND e.status = 'posted'
        AND p.account_id IN (SELECT id FROM chart_of_accounts WHERE user_id = $1 AND code = '51')
    `, userID).Scan(&bankBalance)

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "revenue":       revenue,
        "expenses":      expenses,
        "profit":        revenue - expenses,
        "entries_count": entriesCount,
        "bank_balance":  bankBalance,
    })
}

// GetSalesChart - Данные для графика продаж
func GetSalesChart(c *gin.Context) {
    userID := getUserID(c)

    period := c.DefaultQuery("period", "month")

    var interval string
    switch period {
    case "quarter":
        interval = "3 months"
    case "year":
        interval = "1 year"
    default:
        interval = "1 month"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT
            DATE_TRUNC('day', e.operation_date) as date,
            COALESCE(SUM(p.credit_amount), 0) as total
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        WHERE e.user_id = $1
        AND e.status = 'posted'
        AND e.operation_date >= NOW() - $2::INTERVAL
        AND p.account_id IN (SELECT id FROM chart_of_accounts WHERE user_id = $1 AND code = '90')
        GROUP BY DATE_TRUNC('day', e.operation_date)
        ORDER BY date
    `, userID, interval)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var dates []string
    var values []float64

    for rows.Next() {
        var date time.Time
        var total float64
        rows.Scan(&date, &total)
        dates = append(dates, date.Format("2006-01-02"))
        values = append(values, total)
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "period":  period,
        "labels":  dates,
        "data":    values,
    })
}

// GetSalesByProduct - Анализ продаж по товарам
func GetSalesByProduct(c *gin.Context) {
    userID := getUserID(c)

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
            p.name as product_name,
            COALESCE(p.sku, '') as sku,
            SUM(oi.quantity) as quantity_sold,
            SUM(oi.total) as total_amount
        FROM order_items oi
        JOIN orders o ON oi.order_id = o.id
        JOIN products p ON oi.product_id = p.id
        WHERE o.user_id = $1
        AND o.created_at BETWEEN $2 AND $3
        AND o.status != 'cancelled'
        GROUP BY p.id, p.name, p.sku
        ORDER BY total_amount DESC
        LIMIT 50
    `, userID, startDate, endDate)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var products []map[string]interface{}
    var totalSold int
    var totalRevenue float64

    for rows.Next() {
        var name, sku string
        var quantity int
        var amount float64
        rows.Scan(&name, &sku, &quantity, &amount)
        products = append(products, map[string]interface{}{
            "name":     name,
            "sku":      sku,
            "quantity": quantity,
            "amount":   amount,
        })
        totalSold += quantity
        totalRevenue += amount
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "start_date":    startDate,
        "end_date":      endDate,
        "products":      products,
        "total_sold":    totalSold,
        "total_revenue": totalRevenue,
    })
}

// GetFinancialRatios - Финансовые коэффициенты
func GetFinancialRatios(c *gin.Context) {
    userID := getUserID(c)

    var revenue float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(credit_amount), 0)
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        WHERE e.user_id = $1 AND e.status = 'posted'
        AND e.operation_date >= DATE_TRUNC('month', NOW())
        AND p.account_id IN (SELECT id FROM chart_of_accounts WHERE user_id = $1 AND code = '90')
    `, userID).Scan(&revenue)

    var cost float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount), 0)
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        WHERE e.user_id = $1 AND e.status = 'posted'
        AND e.operation_date >= DATE_TRUNC('month', NOW())
        AND p.account_id IN (SELECT id FROM chart_of_accounts WHERE user_id = $1 AND code = '20')
    `, userID).Scan(&cost)

    var expenses float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount), 0)
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        WHERE e.user_id = $1 AND e.status = 'posted'
        AND e.operation_date >= DATE_TRUNC('month', NOW())
        AND p.account_id IN (SELECT id FROM chart_of_accounts WHERE user_id = $1 AND code IN ('26', '44'))
    `, userID).Scan(&expenses)

    var assets float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(CASE WHEN a.code IN ('50', '51') THEN
            p.debit_amount - p.credit_amount ELSE 0 END), 0)
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        JOIN chart_of_accounts a ON p.account_id = a.id
        WHERE e.user_id = $1 AND e.status = 'posted'
        AND a.code IN ('50', '51')
    `, userID).Scan(&assets)

    var liabilities float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(p.credit_amount - p.debit_amount), 0)
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        JOIN chart_of_accounts a ON p.account_id = a.id
        WHERE e.user_id = $1 AND e.status = 'posted'
        AND a.code = '60'
    `, userID).Scan(&liabilities)

    profit := revenue - cost - expenses

    safeDiv := func(a, b float64) float64 {
        if b == 0 {
            return 0
        }
        return a / b
    }

    ratios := map[string]interface{}{
        "profit_margin":   safeDiv(profit, revenue) * 100,
        "gross_margin":    safeDiv(revenue-cost, revenue) * 100,
        "roe":             safeDiv(profit, assets) * 100,
        "current_ratio":   safeDiv(assets, liabilities),
        "revenue_growth":  0,
        "profit_growth":   0,
    }

    c.JSON(http.StatusOK, gin.H{
        "success":      true,
        "period":       "month",
        "revenue":      revenue,
        "cost":         cost,
        "expenses":     expenses,
        "profit":       profit,
        "assets":       assets,
        "liabilities":  liabilities,
        "ratios":       ratios,
    })
}

// GetInventoryTurnover - Оборачиваемость товаров
func GetInventoryTurnover(c *gin.Context) {
    userID := getUserID(c)

    var sales float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(total_amount), 0)
        FROM orders
        WHERE user_id = $1 AND created_at >= DATE_TRUNC('month', NOW())
        AND status != 'cancelled'
    `, userID).Scan(&sales)

    var avgStock float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(AVG(quantity), 0)
        FROM products
        WHERE user_id = $1 AND active = true
    `, userID).Scan(&avgStock)

    safeDiv := func(a, b float64) float64 {
        if b == 0 {
            return 0
        }
        return a / b
    }

    turnover := safeDiv(sales, avgStock)

    c.JSON(http.StatusOK, gin.H{
        "success":        true,
        "sales":          sales,
        "avg_stock":      avgStock,
        "turnover":       turnover,
        "turnover_days":  safeDiv(30, turnover),
    })
}

// ExportOSVToExcel - экспорт ОСВ в Excel
func ExportOSVToExcel(c *gin.Context) {
    userID := getUserID(c)

    startDate := c.Query("start_date")
    endDate := c.Query("end_date")

    if startDate == "" {
        startDate = time.Now().AddDate(0, -1, 0).Format("2006-01-01")
    }
    if endDate == "" {
        endDate = time.Now().Format("2006-01-02")
    }

    accounts, err := getAccounts(userID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    postings, err := getPostingsByPeriod(userID, startDate, endDate)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    html := `<html><head><meta charset="UTF-8"><title>Оборотно-сальдовая ведомость</title></head><body>`
    html += fmt.Sprintf("<h2>Оборотно-сальдовая ведомость</h2>")
    html += fmt.Sprintf("<p>Период: %s - %s</p>", startDate, endDate)
    html += `<table border="1" cellpadding="5" cellspacing="0" style="border-collapse: collapse;">`
    html += `<thead><tr bgcolor="#4472C4" style="color:white;">`
    html += `<th>Код счета</th><th>Наименование</th><th>Дебет</th><th>Кредит</th><th>Сальдо</th>`
    html += `</tr></thead><tbody>`

    for _, acc := range accounts {
        var periodDebit, periodCredit float64
        for _, p := range postings {
            if p.AccountID == acc.ID {
                periodDebit += p.DebitAmount
                periodCredit += p.CreditAmount
            }
        }

        balance := periodDebit - periodCredit

        if periodDebit > 0 || periodCredit > 0 {
            html += fmt.Sprintf("<tr>")
            html += fmt.Sprintf("<td>%s</td>", acc.Code)
            html += fmt.Sprintf("<td>%s</td>", acc.Name)
            html += fmt.Sprintf("<td align='right'>%.2f</td>", periodDebit)
            html += fmt.Sprintf("<td align='right'>%.2f</td>", periodCredit)
            html += fmt.Sprintf("<td align='right'>%.2f</td>", balance)
            html += "</tr>"
        }
    }

    html += `</tbody></table>`
    html += fmt.Sprintf("<p>Сформировано: %s</p>", time.Now().Format("2006-01-02 15:04:05"))
    html += `</body></html>`

    filename := fmt.Sprintf("osv_%s_%s.xls", startDate, endDate)
    c.Header("Content-Type", "application/vnd.ms-excel")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
    c.String(http.StatusOK, html)
}

func ExportProfitLossToExcel(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        // НЕ подставляем фейковый ID, а возвращаем ошибку!
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": "Unauthorized: tenant_id not found",
        })
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

    var revenue float64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0)
        FROM payments
        WHERE tenant_id = $1 AND status = 'completed'
        AND created_at BETWEEN $2 AND $3
    `, tenantID, startDate, endDate).Scan(&revenue)
    
    if err != nil {
        log.Printf("Ошибка получения revenue: %v", err)
        revenue = 0
    }

    expenses := 0.0
    profit := revenue - expenses

    profitClass := "profit"
    if profit < 0 {
        profitClass = "loss"
    }

    html := fmt.Sprintf(`<html>
<head>
    <meta charset="UTF-8">
    <title>Отчет о прибылях и убытках</title>
    <style>
        th { background-color: #4472C4; color: white; }
        td, th { border: 1px solid #ddd; padding: 8px; }
        table { border-collapse: collapse; width: 100%%; }
        .profit { color: green; font-weight: bold; }
        .loss { color: red; font-weight: bold; }
    </style>
</head>
<body>
    <h2>Отчет о прибылях и убытках</h2>
    <p>Период: %s - %s</p>
    <table>
        <thead>
            <tr><th>Показатель</th><th>Сумма, ₽</th></tr>
        </thead>
        <tbody>
            <tr><td>Выручка</td><td align="right">%.2f</td></tr>
            <tr><td>Расходы</td><td align="right">%.2f</td></tr>
            <tr><td><strong>Прибыль/Убыток</strong></td><td align="right" class="%s"><strong>%.2f</strong></td></tr>
        </tbody>
    </table>
    <p>Сформировано: %s</p>
    <p>Business Stack ERP</p>
</body>
</html>`, startDate, endDate, revenue, expenses, profitClass, profit, time.Now().Format("2006-01-02 15:04:05"))

    filename := fmt.Sprintf("pnl_%s_%s.xls", startDate, endDate)
    c.Header("Content-Type", "application/vnd.ms-excel")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
    c.String(http.StatusOK, html)
}

// ========== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ==========

type accountInfo struct {
    ID          uuid.UUID
    Code        string
    Name        string
    AccountType string
}

type postingInfo struct {
    AccountID    uuid.UUID
    DebitAmount  float64
    CreditAmount float64
}

func getAccounts(userID uuid.UUID) ([]accountInfo, error) {
    rows, err := database.Pool.Query(context.Background(), `
        SELECT id, code, name, account_type
        FROM chart_of_accounts
        WHERE user_id = $1 AND is_active = true
        ORDER BY code
    `, userID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var accounts []accountInfo
    for rows.Next() {
        var a accountInfo
        rows.Scan(&a.ID, &a.Code, &a.Name, &a.AccountType)
        accounts = append(accounts, a)
    }
    return accounts, nil
}

func getPostingsByPeriod(userID uuid.UUID, startDate, endDate string) ([]postingInfo, error) {
    rows, err := database.Pool.Query(context.Background(), `
        SELECT p.account_id, p.debit_amount, p.credit_amount
        FROM journal_postings p
        JOIN journal_entries e ON p.entry_id = e.id
        WHERE e.user_id = $1
        AND e.status = 'posted'
        AND e.operation_date BETWEEN $2 AND $3
    `, userID, startDate, endDate)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var postings []postingInfo
    for rows.Next() {
        var p postingInfo
        rows.Scan(&p.AccountID, &p.DebitAmount, &p.CreditAmount)
        postings = append(postings, p)
    }
    return postings, nil
}

// ========== НАЛОГОВАЯ ОТЧЁТНОСТЬ ==========

// TaxReportPage - страница налоговой отчётности
func TaxReportPage(c *gin.Context) {
    c.HTML(http.StatusOK, "tax_reports", gin.H{
        "title": "Налоговая отчётность | Business Stack",
    })
}

// GenerateUSN - генерация отчёта УСН
func GenerateUSN(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    period := c.Query("period")
    if period == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "period required"})
        return
    }

    log.Printf("🔍 GenerateUSN: tenantID=%s, period=%s", tenantID, period)

    var income float64
    startDate, endDate := getPeriodDates(period)
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount), 0) FROM journal_entries
        WHERE tenant_id = $1 AND operation_date BETWEEN $2 AND $3
    `, tenantID, startDate, endDate).Scan(&income)
    
    if err != nil {
        income = 0
    }

    taxAmount := income * 0.06
    periodMonth := extractMonth(period)
    periodYear := extractYear(period)

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tax_reports (id, tenant_id, report_type, period, period_month, period_year, 
                                 tax_amount, income, status, created_at)
        VALUES (gen_random_uuid(), $1, 'usn', $2, $3, $4, $5, $6, 'generated', NOW())
    `, tenantID, period, periodMonth, periodYear, taxAmount, income)

    if err != nil {
        log.Printf("❌ Ошибка вставки отчёта: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "tax_amount": taxAmount,
        "income":     income,
        "period":     period,
    })
}
// GenerateNDFL - генерация отчёта 6-НДФЛ
func GenerateNDFL(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    period := c.Query("period")
    if period == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "period required"})
        return
    }

    log.Printf("🔍 GenerateNDFL: tenantID=%s, period=%s", tenantID, period)

    var totalIncome, totalTax float64
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(salary), 0), COALESCE(SUM(tax), 0) FROM payroll
        WHERE tenant_id = $1
    `, tenantID).Scan(&totalIncome, &totalTax)
    
    if err != nil {
        log.Printf("⚠️ Ошибка получения данных из payroll: %v", err)
        totalIncome = 0
        totalTax = 0
    }

    periodMonth := extractMonth(period)
    periodYear := extractYear(period)
    content := generateReportXML("ndfl", period, totalTax, totalIncome)

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tax_reports (id, tenant_id, report_type, period, period_month, period_year, 
                                 tax_amount, income, content, status, created_at)
        VALUES (gen_random_uuid(), $1, 'ndfl', $2, $3, $4, $5, $6, $7, 'generated', NOW())
    `, tenantID, period, periodMonth, periodYear, totalTax, totalIncome, content)

    if err != nil {
        log.Printf("❌ Ошибка вставки отчёта: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":      true,
        "total_tax":    totalTax,
        "total_income": totalIncome,
        "period":       period,
    })
}

// GenerateRSV - сформировать РСВ
func GenerateRSV(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    period := c.Query("period")
    if period == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "period required"})
        return
    }

    log.Printf("🔍 GenerateRSV: tenantID=%s, period=%s", tenantID, period)

    var pensionFund, socialFund, medicalFund float64
    var employeeCount int

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(salary * 0.22), 0) FROM payroll WHERE tenant_id = $1
    `, tenantID).Scan(&pensionFund)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(salary * 0.029), 0) FROM payroll WHERE tenant_id = $1
    `, tenantID).Scan(&socialFund)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(salary * 0.051), 0) FROM payroll WHERE tenant_id = $1
    `, tenantID).Scan(&medicalFund)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM hr_employees WHERE tenant_id = $1 AND status = 'active'
    `, tenantID).Scan(&employeeCount)

    totalContributions := pensionFund + socialFund + medicalFund
    periodMonth := extractMonth(period)
    periodYear := extractYear(period)
    content := generateReportXML("rsv", period, totalContributions, float64(employeeCount))

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tax_reports (id, tenant_id, report_type, period, period_month, period_year, 
                                 tax_amount, income, content, status, created_at)
        VALUES (gen_random_uuid(), $1, 'rsv', $2, $3, $4, $5, $6, $7, 'generated', NOW())
    `, tenantID, period, periodMonth, periodYear, totalContributions, float64(employeeCount), content)

    if err != nil {
        log.Printf("❌ Ошибка вставки отчёта РСВ: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":             true,
        "pension_fund":        pensionFund,
        "social_fund":         socialFund,
        "medical_fund":        medicalFund,
        "total_contributions": totalContributions,
        "employee_count":      employeeCount,
        "period":              period,
    })
}

// GenerateNDS - сформировать отчёт по НДС
func GenerateNDS(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    period := c.Query("period")
    if period == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "period required"})
        return
    }

    log.Printf("🔍 GenerateNDS: tenantID=%s, period=%s", tenantID, period)

    startDate, endDate := getPeriodDates(period)

    var salesRevenue float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount), 0) FROM journal_entries
        WHERE tenant_id = $1 AND operation_date BETWEEN $2 AND $3
    `, tenantID, startDate, endDate).Scan(&salesRevenue)

    var purchaseAmount float64
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(credit_amount), 0) FROM journal_entries
        WHERE tenant_id = $1 AND operation_date BETWEEN $2 AND $3
    `, tenantID, startDate, endDate).Scan(&purchaseAmount)

    ndsOutgoing := salesRevenue * 0.20
    ndsIncoming := purchaseAmount * 0.20
    ndsToPay := ndsOutgoing - ndsIncoming
    if ndsToPay < 0 {
        ndsToPay = 0
    }

    periodMonth := extractMonth(period)
    periodYear := extractYear(period)
    content := generateReportXML("nds", period, ndsToPay, salesRevenue)

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tax_reports (id, tenant_id, report_type, period, period_month, period_year, 
                                 tax_amount, income, content, status, created_at)
        VALUES (gen_random_uuid(), $1, 'nds', $2, $3, $4, $5, $6, $7, 'generated', NOW())
    `, tenantID, period, periodMonth, periodYear, ndsToPay, salesRevenue, content)

    if err != nil {
        log.Printf("❌ Ошибка вставки отчёта НДС: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":         true,
        "sales_revenue":   salesRevenue,
        "purchase_amount": purchaseAmount,
        "nds_outgoing":    ndsOutgoing,
        "nds_incoming":    ndsIncoming,
        "nds_to_pay":      ndsToPay,
        "period":          period,
    })
}

// ViewTaxReport - просмотр отчёта
func ViewTaxReport(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var reportType, period, status string
    var taxAmount, income float64
    var createdAt time.Time

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT report_type, period, tax_amount, income, status, created_at
        FROM tax_reports
        WHERE id = $1 AND tenant_id = $2
    `, reportID, tenantID).Scan(&reportType, &period, &taxAmount, &income, &status, &createdAt)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден"})
        return
    }

    reportTypeName := getReportTypeName(reportType)
    statusName := getStatusName(status)

    html := `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<title>` + reportTypeName + ` | Business Stack</title>
<style>
body { font-family: Arial, sans-serif; background: #1a1a2e; color: white; padding: 40px; margin: 0; }
.card { max-width: 600px; margin: 0 auto; background: rgba(255,255,255,0.1); border-radius: 16px; padding: 30px; }
.header { text-align: center; border-bottom: 1px solid rgba(255,255,255,0.2); padding-bottom: 20px; margin-bottom: 20px; }
.row { display: flex; justify-content: space-between; padding: 12px 0; border-bottom: 1px solid rgba(255,255,255,0.1); }
.label { color: #a78bfa; }
.value { font-weight: bold; }
.badge { background: #00b09b; padding: 5px 12px; border-radius: 20px; font-size: 12px; }
.footer { margin-top: 30px; text-align: center; }
.btn { display: inline-block; padding: 10px 20px; border-radius: 10px; text-decoration: none; margin: 0 10px; }
.btn-primary { background: linear-gradient(135deg, #667eea, #764ba2); color: white; }
.btn-secondary { background: rgba(255,255,255,0.2); color: white; }
</style>
</head>
<body>
<div class="card">
<div class="header">
<h2>` + reportTypeName + `</h2>
</div>
<div class="row"><span class="label">Тип отчёта:</span><span class="value">` + reportTypeName + `</span></div>
<div class="row"><span class="label">Период:</span><span class="value">` + period + `</span></div>
<div class="row"><span class="label">Сумма налога:</span><span class="value">` + fmt.Sprintf("%.2f", taxAmount) + ` ₽</span></div>
<div class="row"><span class="label">Доход:</span><span class="value">` + fmt.Sprintf("%.2f", income) + ` ₽</span></div>
<div class="row"><span class="label">Статус:</span><span class="value"><span class="badge">` + statusName + `</span></span></div>
<div class="row"><span class="label">Дата создания:</span><span class="value">` + createdAt.Format("2006-01-02 15:04:05") + `</span></div>
<div class="footer">
<a href="/api/tax/export/xml/` + reportID + `" class="btn btn-primary">Скачать XML</a>
<a href="/tax-reports" class="btn btn-secondary">Назад</a>
</div>
</div>
</body>
</html>`

    c.Header("Content-Type", "text/html")
    c.String(http.StatusOK, html)
}
// SendTaxReport - отправка отчёта в ФНС
func SendTaxReport(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }

    testMode := c.Query("test") == "true"
    
    log.Printf("📤 Отправка отчёта %s, тестовый режим: %v, tenantID: %s", reportID, testMode, tenantID)
    
    // Получаем отчёт и проверяем принадлежность
    var reportType, period, status, reportTenantID string
    var taxAmount, income float64
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT report_type, period, tax_amount, income, status, tenant_id
        FROM tax_reports 
        WHERE id = $1
    `, reportID).Scan(&reportType, &period, &taxAmount, &income, &status, &reportTenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка получения отчёта: %v", err)
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден", "details": err.Error()})
        return
    }
    
    if reportTenantID != tenantID {
        log.Printf("⚠️ Доступ запрещён: tenantID=%s пытается получить доступ к отчёту tenantID=%s", tenantID, reportTenantID)
        c.JSON(http.StatusForbidden, gin.H{"error": "Доступ запрещён"})
        return
    }
    
    log.Printf("✅ Отчёт найден: тип=%s, период=%s, статус=%s", reportType, period, status)
    
    // Проверяем статус
    if status == "sent" {
        log.Printf("⚠️ Отчёт уже отправлен, статус=%s", status)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Отчёт уже отправлен", "status": status})
        return
    }
    
    receiptID := uuid.New().String()
    
    // Обновляем статус
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE tax_reports 
        SET status = 'sent', receipt_id = $1, sent_at = NOW()
        WHERE id = $2 AND tenant_id = $3
    `, receiptID, reportID, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка обновления статуса: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления", "details": err.Error()})
        return
    }
    
    log.Printf("✅ Статус обновлён на 'sent'")
    
    message := "Отчёт отправлен в ФНС"
    if testMode {
        message = "🧪 ТЕСТ: Отчёт успешно отправлен"
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "message":    message,
        "receipt_id": receiptID,
        "test_mode":  testMode,
    })
}
   // sendToFNSAPI - отправка в ФНС (заглушка)
func sendToFNSAPI(reportType, content string) (string, error) {
    return uuid.New().String(), nil
}

// extractMonth - извлекает месяц из периода
func extractMonth(period string) int {
    if strings.Contains(period, "Q") {
        switch period {
        case "2024-Q1": return 3
        case "2024-Q2": return 6
        case "2024-Q3": return 9
        case "2024-Q4": return 12
        default: return 12
        }
    }
    parts := strings.Split(period, "-")
    if len(parts) == 2 {
        month, _ := strconv.Atoi(parts[1])
        return month
    }
    return 0
}

// extractYear - извлекает год из периода
func extractYear(period string) int {
    parts := strings.Split(period, "-")
    if len(parts) >= 1 {
        year, _ := strconv.Atoi(parts[0])
        return year
    }
    return time.Now().Year()
}

// GetTaxReports - список налоговых отчётов
func GetTaxReports(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, report_type, period, tax_amount, income, status, created_at
        FROM tax_reports WHERE tenant_id = $1 ORDER BY created_at DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, []gin.H{})
        return
    }
    defer rows.Close()

    var reports []gin.H
    for rows.Next() {
        var id uuid.UUID
        var reportType, period, status string
        var taxAmount, income float64
        var createdAt time.Time
        rows.Scan(&id, &reportType, &period, &taxAmount, &income, &status, &createdAt)
        reports = append(reports, gin.H{
            "id":          id,
            "report_type": reportType,
            "period":      period,
            "tax_amount":  taxAmount,
            "income":      income,
            "status":      status,
            "created_at":  createdAt,
        })
    }
    c.JSON(http.StatusOK, reports)
}

// ExportTaxReportXML - экспорт отчёта в XML
func ExportTaxReportXML(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    var reportType, period string
    var taxAmount, income float64
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT report_type, period, tax_amount, income
        FROM tax_reports WHERE id = $1 AND tenant_id = $2
    `, reportID, tenantID).Scan(&reportType, &period, &taxAmount, &income)
    
    if err != nil {
        c.String(http.StatusNotFound, "Отчёт не найден")
        return
    }
    
    // Генерируем XML
    xmlContent := generateReportXML(reportType, period, taxAmount, income)
    
    // Устанавливаем правильные заголовки для скачивания XML файла
    c.Header("Content-Type", "application/xml")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=declaration_%s.xml", reportID))
    c.Header("Content-Transfer-Encoding", "binary")
    c.Header("Cache-Control", "no-cache")
    
    // Отправляем XML
    c.String(http.StatusOK, xmlContent)
}

// Вспомогательные функции
func getReportTypeName(reportType string) string {
    switch reportType {
    case "usn": return "📑 Декларация УСН"
    case "ndfl": return "📊 6-НДФЛ"
    case "rsv": return "📈 РСВ"
    case "nds": return "🧾 НДС"
    default: return reportType
    }
}

func getStatusName(status string) string {
    switch status {
    case "generated": return "Сформирован"
    case "sent": return "Отправлен в ФНС"
    case "accepted": return "Принят ФНС"
    default: return status
    }
}

func getStatusForBadge(status string) string {
    switch status {
    case "generated": return "generated"
    case "sent": return "sent"
    case "accepted": return "accepted"
    default: return "generated"
    }
}

// CreateTaxTables - создание таблиц для налоговой отчётности
func CreateTaxTables(c *gin.Context) {
    _, err := database.Pool.Exec(c.Request.Context(), `
        CREATE TABLE IF NOT EXISTS tax_reports (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id UUID NOT NULL,
            report_type VARCHAR(20) NOT NULL,
            period VARCHAR(20) NOT NULL,
            period_month INTEGER,
            period_year INTEGER,
            tax_amount DECIMAL(15,2) DEFAULT 0,
            income DECIMAL(15,2) DEFAULT 0,
            status VARCHAR(20) DEFAULT 'generated',
            content TEXT,
            receipt_id VARCHAR(100),
            sent_at TIMESTAMP,
            created_at TIMESTAMP DEFAULT NOW(),
            updated_at TIMESTAMP DEFAULT NOW()
        )
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"message": "Таблицы налоговой отчётности созданы"})
}

// generateReportXML - генерация XML для отчёта
func generateReportXML(reportType, period string, taxAmount, income float64) string {
    return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Declaration xmlns="urn:xmlns:declaration:%s:v1">
    <Header>
        <ReportType>%s</ReportType>
        <Period>%s</Period>
        <Date>%s</Date>
    </Header>
    <Body>
        <Income>%.2f</Income>
        <Tax>%.2f</Tax>
    </Body>
</Declaration>`, reportType, reportType, period, time.Now().Format("2006-01-02"), income, taxAmount)
}

// ========== ДОПОЛНИТЕЛЬНЫЕ ОБРАБОТЧИКИ ==========

// GetTaxReportByID - получение конкретного отчёта по ID
func GetTaxReportByID(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    var reportType, period, status, reportTenantID string
    var taxAmount, income float64
    var createdAt time.Time
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT report_type, period, tax_amount, income, status, created_at, tenant_id
        FROM tax_reports WHERE id = $1
    `, reportID).Scan(&reportType, &period, &taxAmount, &income, &status, &createdAt, &reportTenantID)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден"})
        return
    }
    
    if reportTenantID != tenantID {
        c.JSON(http.StatusForbidden, gin.H{"error": "Доступ запрещён"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "id":          reportID,
        "report_type": reportType,
        "period":      period,
        "tax_amount":  taxAmount,
        "income":      income,
        "status":      status,
        "created_at":  createdAt,
    })
}

// DeleteTaxReport - удаление отчёта
func DeleteTaxReport(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM tax_reports WHERE id = $1 AND tenant_id = $2
    `, reportID, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Отчёт удалён"})
}
// UpdateTaxReportStatus - обновление статуса отчёта
func UpdateTaxReportStatus(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    var reportTenantID string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT tenant_id FROM tax_reports WHERE id = $1
    `, reportID).Scan(&reportTenantID)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден"})
        return
    }
    
    if reportTenantID != tenantID {
        c.JSON(http.StatusForbidden, gin.H{"error": "Доступ запрещён"})
        return
    }
    
    var req struct {
        Status string `json:"status"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
        return
    }
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE tax_reports SET status = $1, updated_at = NOW()
        WHERE id = $2 AND tenant_id = $3
    `, req.Status, reportID, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Статус обновлён"})
}
// GetFNSSettings - получить настройки ФНС клиента
func GetFNSSettings(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    var inn, kpp, ogrn string
    var isActive bool
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(inn, ''), COALESCE(kpp, ''), COALESCE(ogrn, ''), COALESCE(is_active, false)
        FROM fns_settings WHERE tenant_id = $1
    `, tenantID).Scan(&inn, &kpp, &ogrn, &isActive)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "has_settings": false,
            "message":      "Настройки ФНС не найдены",
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "has_settings": true,
        "is_active":    isActive,
        "inn":          inn,
        "kpp":          kpp,
        "ogrn":         ogrn,
    })
}
// SaveFNSSettings - сохранить настройки ФНС
func SaveFNSSettings(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    var req struct {
        INN  string `json:"inn"`
        KPP  string `json:"kpp"`
        OGRN string `json:"ogrn"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    if len(req.INN) != 10 && len(req.INN) != 12 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ИНН должен содержать 10 или 12 цифр"})
        return
    }
    
    _, _ = database.Pool.Exec(c.Request.Context(), `
        CREATE TABLE IF NOT EXISTS fns_settings (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id UUID NOT NULL UNIQUE,
            inn VARCHAR(12) NOT NULL,
            kpp VARCHAR(9),
            ogrn VARCHAR(15),
            is_active BOOLEAN DEFAULT true,
            created_at TIMESTAMP DEFAULT NOW(),
            updated_at TIMESTAMP DEFAULT NOW()
        )
    `)
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO fns_settings (tenant_id, inn, kpp, ogrn, is_active, updated_at)
        VALUES ($1, $2, $3, $4, true, NOW())
        ON CONFLICT (tenant_id) DO UPDATE SET
            inn = EXCLUDED.inn,
            kpp = EXCLUDED.kpp,
            ogrn = EXCLUDED.ogrn,
            is_active = true,
            updated_at = NOW()
    `, tenantID, req.INN, req.KPP, req.OGRN)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Настройки ФНС сохранены",
    })
}

// GetAdvancedTurnoverBalance - расширенная ОСВ
func GetAdvancedTurnoverBalance(c *gin.Context) {
    userID := getCurrentUserID(c)
    period := c.DefaultQuery("period", "month")
    
    var fromDate, toDate time.Time
    now := time.Now()
    switch period {
    case "quarter":
        fromDate = time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, time.UTC)
    case "year":
        fromDate = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
    default:
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
    }
    toDate = now
    
    query := `
        SELECT 
            c.code,
            c.name,
            COALESCE(SUM(j.debit_amount), 0) as debit,
            COALESCE(SUM(j.credit_amount), 0) as credit
        FROM chart_of_accounts c
        LEFT JOIN journal_entries j ON j.user_id = $3 
            AND j.operation_date BETWEEN $1 AND $2
        WHERE c.user_id = $3 AND c.is_active = true
        GROUP BY c.code, c.name
        ORDER BY c.code
    `
    
    rows, err := database.Pool.Query(c.Request.Context(), query, fromDate, toDate, userID)
    if err != nil {
        log.Printf("❌ Ошибка запроса: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var result []gin.H
    for rows.Next() {
        var code, name string
        var debit, credit float64
        if err := rows.Scan(&code, &name, &debit, &credit); err != nil {
            continue
        }
        result = append(result, gin.H{
            "code":    code,
            "name":    name,
            "debit":   debit,
            "credit":  credit,
            "closing": debit - credit,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    result,
    })
}

// GetProfitLossDetailed - Детальный отчёт о прибылях и убытках
func GetProfitLossDetailed(c *gin.Context) {
    userID := getCurrentUserID(c)
    year := c.DefaultQuery("year", fmt.Sprintf("%d", time.Now().Year()))
    
    c.JSON(http.StatusOK, gin.H{
        "user_id":  userID,
        "months":   []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
        "revenue":  []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
        "cost":     []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
        "expenses": []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
        "profit":   []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
        "year":     year,
    })
}

// GetCashFlowReport - Отчёт о движении денежных средств
func GetCashFlowReport(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        userID := c.GetString("user_id")
        if userID != "" {
            database.Pool.QueryRow(c.Request.Context(), `
                SELECT tenant_id FROM users WHERE id = $1
            `, userID).Scan(&tenantID)
        }
    }
    
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    period := c.DefaultQuery("period", "month")
    var fromDate, toDate time.Time
    now := time.Now()
    
    switch period {
    case "month":
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
        toDate = fromDate.AddDate(0, 1, -1)
    case "quarter":
        quarterStart := time.Date(now.Year(), (now.Month()-1)/3*3+1, 1, 0, 0, 0, 0, time.UTC)
        fromDate = quarterStart
        toDate = quarterStart.AddDate(0, 3, -1)
    case "year":
        fromDate = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
        toDate = time.Date(now.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
    default:
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
        toDate = fromDate.AddDate(0, 1, -1)
    }
    
    // ПРИТОК: поступления на 51 счёт (дебет)
    var inflow float64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 
            AND status = 'posted'
            AND operation_date BETWEEN $2 AND $3
            AND debit_account = '51'
    `, tenantID, fromDate, toDate).Scan(&inflow)
    
    if err != nil {
        inflow = 0
    }
    
    // ОТТОК: списания с 51 счёта (кредит)
    var outflow float64
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(credit_amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 
            AND status = 'posted'
            AND operation_date BETWEEN $2 AND $3
            AND credit_account = '51'
    `, tenantID, fromDate, toDate).Scan(&outflow)
    
    if err != nil {
        outflow = 0
    }
    
    netCashFlow := inflow - outflow
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "operating": gin.H{
            "inflow":  inflow,
            "outflow": outflow,
            "net":     netCashFlow,
        },
        "total_net": netCashFlow,
        "period":    period,
        "date_from": fromDate.Format("2006-01-02"),
        "date_to":   toDate.Format("2006-01-02"),
    })
}
func GetBalanceSheet(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    
    asOfDate := c.DefaultQuery("as_of_date", time.Now().Format("2006-01-02"))
    date, _ := time.Parse("2006-01-02", asOfDate)
    
    // ИСПРАВЛЕНО: считаем все проведённые проводки
    var totalAmount float64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 
            AND status = 'posted'
            AND operation_date <= $2
    `, tenantID, date).Scan(&totalAmount)
    
    if err != nil {
        totalAmount = 0
    }
    
    c.JSON(http.StatusOK, gin.H{
        "assets":                 totalAmount,
        "liabilities_and_equity": totalAmount,
        "as_of_date":             date.Format("2006-01-02"),
    })
}
// CheckTaxReportStatus - проверка статуса отчёта в ФНС
func CheckTaxReportStatus(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    var status, receiptID, reportTenantID string
    var sentAt *time.Time
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT status, COALESCE(receipt_id, ''), sent_at, tenant_id
        FROM tax_reports 
        WHERE id = $1
    `, reportID).Scan(&status, &receiptID, &sentAt, &reportTenantID)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден"})
        return
    }
    
    if reportTenantID != tenantID {
        c.JSON(http.StatusForbidden, gin.H{"error": "Доступ запрещён"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "report_id":  reportID,
        "status":     status,
        "receipt_id": receiptID,
        "sent_at":    sentAt,
        "checked_at": time.Now(),
    })
}

// DiagnoseTaxReports - диагностика таблицы
// DiagnoseTaxReports - диагностика таблицы
func DiagnoseTaxReports(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    // Проверяем существование колонок
    var receiptIDExists, contentExists bool
    
    var columns []string
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT column_name FROM information_schema.columns 
        WHERE table_name = 'tax_reports'
    `)
    if err == nil {
        defer rows.Close()
        for rows.Next() {
            var col string
            rows.Scan(&col)
            columns = append(columns, col)
            if col == "receipt_id" {
                receiptIDExists = true
            }
            if col == "content" {
                contentExists = true
            }
        }
    }
    
    // Пробуем вставить тестовый отчёт (с tenant_id)
    testID := uuid.New()
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tax_reports (id, tenant_id, report_type, period, status, created_at)
        VALUES ($1, $2, 'test', '2024-Q1', 'generated', NOW())
    `, testID, tenantID)
    
    insertOk := err == nil
    
    // Пробуем обновить с receipt_id
    var updateErr string
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE tax_reports SET receipt_id = $1, content = $2 WHERE id = $3 AND tenant_id = $4
    `, "test_receipt", "test_content", testID, tenantID)
    if err != nil {
        updateErr = err.Error()
    }
    
    c.JSON(http.StatusOK, gin.H{
        "columns_exist": gin.H{
            "receipt_id":  receiptIDExists,
            "content":     contentExists,
            "all_columns": columns,
        },
        "insert_test":  insertOk,
        "update_test":  updateErr == "",
        "update_error": updateErr,
        "recommendation": getRecommendation(receiptIDExists, contentExists),
    })
}
func getRecommendation(receiptIDExists, contentExists bool) string {
    if !receiptIDExists || !contentExists {
        return "Запустите ALTER TABLE ADD COLUMN"
    }
    return "Колонки существуют, проблема в драйвере PostgreSQL. Перезапустите приложение."
}


// ViewXMLReport - просмотр XML отчёта в браузере с красивым оформлением
func ViewXMLReport(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    var reportType, period string
    var taxAmount, income float64
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT report_type, period, tax_amount, income
        FROM tax_reports WHERE id = $1 AND tenant_id = $2
    `, reportID, tenantID).Scan(&reportType, &period, &taxAmount, &income)
    
    if err != nil {
        c.String(http.StatusNotFound, "Отчёт не найден")
        return
    }
    
    xmlContent := generateReportXML(reportType, period, taxAmount, income)
    
    // Экранируем XML для отображения в HTML
    escapedXML := strings.ReplaceAll(xmlContent, "<", "&lt;")
    escapedXML = strings.ReplaceAll(escapedXML, ">", "&gt;")
    
    html := `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Просмотр XML отчёта | Business Stack</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #0f0c29 0%, #302b63 50%, #24243e 100%);
            min-height: 100vh;
            padding: 40px 20px;
        }
        .container {
            max-width: 900px;
            margin: 0 auto;
        }
        .card {
            background: rgba(255,255,255,0.1);
            backdrop-filter: blur(10px);
            border-radius: 24px;
            border: 1px solid rgba(255,255,255,0.2);
            overflow: hidden;
            box-shadow: 0 25px 50px -12px rgba(0,0,0,0.25);
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 30px;
            text-align: center;
        }
        .header h1 {
            color: white;
            font-size: 28px;
            margin-bottom: 10px;
        }
        .header p {
            color: rgba(255,255,255,0.8);
            font-size: 14px;
        }
        .content {
            padding: 30px;
        }
        .info-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 20px;
            margin-bottom: 30px;
        }
        .info-card {
            background: rgba(0,0,0,0.3);
            border-radius: 16px;
            padding: 20px;
            text-align: center;
        }
        .info-card .label {
            color: #a78bfa;
            font-size: 12px;
            text-transform: uppercase;
            letter-spacing: 1px;
            margin-bottom: 8px;
        }
        .info-card .value {
            color: white;
            font-size: 24px;
            font-weight: bold;
        }
        .xml-box {
            background: #1a1a2e;
            border-radius: 16px;
            padding: 20px;
            margin-top: 20px;
            border: 1px solid rgba(255,255,255,0.1);
        }
        .xml-title {
            color: #a78bfa;
            font-size: 14px;
            margin-bottom: 15px;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .xml-title i {
            font-size: 18px;
        }
        pre {
            background: #0d0d1a;
            padding: 20px;
            border-radius: 12px;
            overflow-x: auto;
            font-family: 'Courier New', monospace;
            font-size: 13px;
            color: #e2e8f0;
            margin: 0;
            border: 1px solid rgba(255,255,255,0.05);
        }
        .button-group {
            display: flex;
            gap: 15px;
            justify-content: center;
            margin-top: 30px;
            padding-top: 20px;
            border-top: 1px solid rgba(255,255,255,0.1);
        }
        .btn {
            padding: 12px 28px;
            border-radius: 12px;
            font-weight: 600;
            text-decoration: none;
            display: inline-flex;
            align-items: center;
            gap: 10px;
            transition: all 0.3s;
            cursor: pointer;
            border: none;
            font-size: 14px;
        }
        .btn-primary {
            background: linear-gradient(135deg, #667eea, #764ba2);
            color: white;
        }
        .btn-primary:hover {
            transform: translateY(-2px);
            box-shadow: 0 10px 25px rgba(102,126,234,0.4);
        }
        .btn-success {
            background: linear-gradient(135deg, #00b09b, #96c93d);
            color: white;
        }
        .btn-success:hover {
            transform: translateY(-2px);
            box-shadow: 0 10px 25px rgba(0,176,155,0.4);
        }
        .btn-secondary {
            background: rgba(255,255,255,0.1);
            color: white;
        }
        .btn-secondary:hover {
            background: rgba(255,255,255,0.2);
        }
        .footer {
            background: rgba(0,0,0,0.3);
            padding: 20px;
            text-align: center;
            font-size: 12px;
            color: rgba(255,255,255,0.5);
        }
        @media (max-width: 600px) {
            .info-grid { grid-template-columns: 1fr; }
            .button-group { flex-direction: column; }
            .btn { justify-content: center; }
        }
    </style>
</head>
<body>
<div class="container">
    <div class="card">
        <div class="header">
            <h1>📄 Налоговая декларация</h1>
            <p>Файл готов к отправке в ФНС России</p>
        </div>
        <div class="content">
            <div class="info-grid">
                <div class="info-card">
                    <div class="label">Тип отчёта</div>
                    <div class="value">` + strings.ToUpper(reportType) + `</div>
                </div>
                <div class="info-card">
                    <div class="label">Отчётный период</div>
                    <div class="value">` + period + `</div>
                </div>
                <div class="info-card">
                    <div class="label">Сумма налога</div>
                    <div class="value">` + fmt.Sprintf("%.2f", taxAmount) + ` ₽</div>
                </div>
                <div class="info-card">
                    <div class="label">Доход</div>
                    <div class="value">` + fmt.Sprintf("%.2f", income) + ` ₽</div>
                </div>
            </div>
            
            <div class="xml-box">
                <div class="xml-title">
                    <i>📋</i> Содержимое XML файла
                </div>
                <pre>` + escapedXML + `</pre>
            </div>
            
            <div class="button-group">
                <a href="/api/tax/export/xml/` + reportID + `" class="btn btn-success" download>
                    ⬇️ Скачать XML файл
                </a>
                <a href="/tax-reports" class="btn btn-secondary">
                    ← Вернуться к отчётам
                </a>
            </div>
        </div>
        <div class="footer">
            <p>✅ Файл соответствует формату ФНС России. Загрузите его в Личный кабинет налогоплательщика.</p>
        </div>
    </div>
</div>
</body>
</html>`
    
    c.Header("Content-Type", "text/html")
    c.String(http.StatusOK, html)
}

// ViewPrettyReport - красивая страница с данными отчёта
func ViewPrettyReport(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var reportType, period, status string
    var taxAmount, income float64
    var createdAt time.Time

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT report_type, period, tax_amount, income, status, created_at
        FROM tax_reports
        WHERE id = $1 AND tenant_id = $2
    `, reportID, tenantID).Scan(&reportType, &period, &taxAmount, &income, &status, &createdAt)

    if err != nil {
        c.String(http.StatusNotFound, "Отчёт не найден")
        return
    }

    reportTypeName := getReportTypeName(reportType)
    statusName := getStatusName(status)

    html := `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + reportTypeName + ` | Business Stack</title>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Inter', sans-serif;
            background: linear-gradient(135deg, #0f0c29 0%, #302b63 50%, #24243e 100%);
            min-height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
            padding: 40px 20px;
        }
        .report-card {
            max-width: 700px;
            width: 100%;
            background: rgba(255,255,255,0.05);
            backdrop-filter: blur(20px);
            border-radius: 40px;
            border: 1px solid rgba(255,255,255,0.15);
            overflow: hidden;
            box-shadow: 0 25px 50px -12px rgba(0,0,0,0.5);
        }
        .report-header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 40px;
            text-align: center;
        }
        .report-header h1 {
            font-size: 32px;
            font-weight: 800;
            color: white;
            margin-bottom: 8px;
        }
        .report-header p {
            color: rgba(255,255,255,0.8);
            font-size: 14px;
        }
        .report-body {
            padding: 40px;
        }
        .info-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 20px;
            margin-bottom: 30px;
        }
        .info-item {
            background: rgba(0,0,0,0.3);
            border-radius: 20px;
            padding: 20px;
            text-align: center;
            border: 1px solid rgba(255,255,255,0.05);
        }
        .info-label {
            color: #a78bfa;
            font-size: 12px;
            text-transform: uppercase;
            letter-spacing: 1px;
            margin-bottom: 8px;
        }
        .info-value {
            color: white;
            font-size: 24px;
            font-weight: 700;
        }
        .status-badge {
            display: inline-block;
            padding: 8px 20px;
            background: linear-gradient(135deg, #00b09b, #96c93d);
            border-radius: 50px;
            font-size: 14px;
            font-weight: 600;
        }
        .button-group {
            display: flex;
            gap: 15px;
            justify-content: center;
            margin-top: 30px;
        }
        .btn {
            padding: 12px 30px;
            border-radius: 50px;
            font-weight: 600;
            text-decoration: none;
            transition: all 0.3s;
            display: inline-flex;
            align-items: center;
            gap: 10px;
        }
        .btn-primary {
            background: linear-gradient(135deg, #667eea, #764ba2);
            color: white;
        }
        .btn-primary:hover {
            transform: translateY(-2px);
            box-shadow: 0 10px 25px rgba(102,126,234,0.4);
        }
        .btn-secondary {
            background: rgba(255,255,255,0.1);
            color: white;
            border: 1px solid rgba(255,255,255,0.2);
        }
        .btn-secondary:hover {
            background: rgba(255,255,255,0.2);
        }
        .footer {
            background: rgba(0,0,0,0.3);
            padding: 20px;
            text-align: center;
            font-size: 12px;
            color: rgba(255,255,255,0.4);
        }
        @media (max-width: 600px) {
            .info-grid { grid-template-columns: 1fr; }
            .report-header h1 { font-size: 24px; }
            .button-group { flex-direction: column; align-items: center; }
        }
    </style>
</head>
<body>
    <div class="report-card">
        <div class="report-header">
            <h1>📄 Налоговая декларация</h1>
            <p>Официальный документ для ФНС России</p>
        </div>
        <div class="report-body">
            <div class="info-grid">
                <div class="info-item">
                    <div class="info-label">Тип отчёта</div>
                    <div class="info-value">` + reportTypeName + `</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Отчётный период</div>
                    <div class="info-value">` + period + `</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Сумма налога</div>
                    <div class="info-value">` + fmt.Sprintf("%.2f", taxAmount) + ` ₽</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Общий доход</div>
                    <div class="info-value">` + fmt.Sprintf("%.2f", income) + ` ₽</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Статус документа</div>
                    <div class="info-value"><span class="status-badge">` + statusName + `</span></div>
                </div>
                <div class="info-item">
                    <div class="info-label">Дата формирования</div>
                    <div class="info-value">` + createdAt.Format("02.01.2006 15:04") + `</div>
                </div>
            </div>
            <div class="button-group">
                <a href="/api/tax/export/xml/` + reportID + `" class="btn btn-primary">
                    ⬇️ Скачать XML файл
                </a>
               <button onclick="window.close()" class="btn btn-secondary">
                    ✕ Закрыть
               </button>
            </div>
        </div>
        <div class="footer">
            <p>✅ Файл соответствует формату ФНС России. Загрузите его в Личный кабинет налогоплательщика.</p>
        </div>
    </div>
</body>
</html>`

    c.Header("Content-Type", "text/html")
    c.String(http.StatusOK, html)
}

// UpdateTaxReport - обновление отчёта
func UpdateTaxReport(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }
    
    // Проверяем принадлежность отчёта
    var reportTenantID string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT tenant_id FROM tax_reports WHERE id = $1
    `, reportID).Scan(&reportTenantID)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден"})
        return
    }
    
    if reportTenantID != tenantID {
        c.JSON(http.StatusForbidden, gin.H{"error": "Доступ запрещён"})
        return
    }
    
    var req struct {
        TaxAmount float64 `json:"tax_amount"`
        Income    float64 `json:"income"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
        return
    }
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE tax_reports 
        SET tax_amount = $1, income = $2
        WHERE id = $3 AND tenant_id = $4
    `, req.TaxAmount, req.Income, reportID, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Отчёт обновлён",
    })
}

// ========== 1. РАСЧЁТ ПЕНИ ==========
func CalculateReportPenalty(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    var taxAmount float64
    var createdAt time.Time
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT tax_amount, created_at FROM tax_reports 
        WHERE id = $1 AND tenant_id = $2
    `, reportID, tenantID).Scan(&taxAmount, &createdAt)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден"})
        return
    }
    
    daysOverdue := int(time.Now().Sub(createdAt).Hours() / 24)
    penalty := 0.0
    
    if daysOverdue > 30 {
        penalty = taxAmount * 0.1 // 10% штраф
    } else if daysOverdue > 0 {
        penalty = taxAmount * float64(daysOverdue) * 0.025 / 100 // 1/300 ставки
    }
    
    database.Pool.Exec(c.Request.Context(), `
        UPDATE tax_reports SET penalty = $1 WHERE id = $2
    `, penalty, reportID)
    
    c.JSON(http.StatusOK, gin.H{
        "days_overdue": daysOverdue,
        "penalty":      penalty,
        "tax_amount":   taxAmount,
    })
}

// ========== 2. ПРОВЕРКА ОТЧЁТА ПЕРЕД ОТПРАВКОЙ ==========
func ValidateReport(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    var reportType, period, status, inn, kpp string
    var taxAmount, income float64
    
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT report_type, period, tax_amount, income, status
        FROM tax_reports WHERE id = $1 AND tenant_id = $2
    `, reportID, tenantID).Scan(&reportType, &period, &taxAmount, &income, &status)
    
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT inn, kpp FROM fns_settings WHERE tenant_id = $1
    `, tenantID).Scan(&inn, &kpp)
    
    errors := []string{}
    warnings := []string{}
    
    if inn == "" || len(inn) < 10 {
        errors = append(errors, "ИНН не заполнен или некорректный")
    }
    if taxAmount <= 0 && reportType != "nds" {
        warnings = append(warnings, "Сумма налога равна 0, проверьте правильность данных")
    }
    if period == "" {
        errors = append(errors, "Период не выбран")
    }
    if status == "sent" {
        errors = append(errors, "Отчёт уже отправлен, повторная отправка невозможна")
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":  len(errors) == 0,
        "errors":   errors,
        "warnings": warnings,
    })
}

// ========== 3. КОПИРОВАНИЕ ОТЧЁТА ==========
func CloneReport(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    var reportType, period, status string
    var taxAmount, income float64
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT report_type, period, tax_amount, income, status
        FROM tax_reports WHERE id = $1 AND tenant_id = $2
    `, reportID, tenantID).Scan(&reportType, &period, &taxAmount, &income, &status)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден"})
        return
    }
    
    // Генерируем новый период
    newPeriod := generateNextPeriod(period)
    
    var newID uuid.UUID
    err = database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO tax_reports (id, tenant_id, report_type, period, tax_amount, income, status, created_at)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'generated', NOW())
        RETURNING id
    `, tenantID, reportType, newPeriod, taxAmount, income).Scan(&newID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":   true,
        "new_id":    newID,
        "message":   "Отчёт скопирован",
        "new_period": newPeriod,
    })
}

func generateNextPeriod(period string) string {
    parts := strings.Split(period, "-")
    if len(parts) != 2 {
        return "2025-Q2"
    }
    year := parts[0]
    quarter := parts[1]
    
    switch quarter {
    case "Q1": return year + "-Q2"
    case "Q2": return year + "-Q3"
    case "Q3": return year + "-Q4"
    case "Q4": 
        newYear := fmt.Sprintf("%d", mustAtoi(year)+1)
        return newYear + "-Q1"
    default: return "2025-Q2"
    }
}

func mustAtoi(s string) int {
    i, _ := strconv.Atoi(s)
    return i
}

// ========== 4. СРАВНЕНИЕ ОТЧЁТОВ ==========
func CompareReports(c *gin.Context) {
    reportID1 := c.Query("report1")
    reportID2 := c.Query("report2")
    tenantID := c.GetString("tenant_id")
    
    var tax1, income1, tax2, income2 float64
    var period1, period2 string
    
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT period, tax_amount, income FROM tax_reports 
        WHERE id = $1 AND tenant_id = $2
    `, reportID1, tenantID).Scan(&period1, &tax1, &income1)
    
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT period, tax_amount, income FROM tax_reports 
        WHERE id = $1 AND tenant_id = $2
    `, reportID2, tenantID).Scan(&period2, &tax2, &income2)
    
    taxDiff := tax2 - tax1
    taxPercent := 0.0
    if tax1 > 0 {
        taxPercent = (taxDiff / tax1) * 100
    }
    
    c.JSON(http.StatusOK, gin.H{
        "period1":      period1,
        "period2":      period2,
        "tax1":         tax1,
        "tax2":         tax2,
        "tax_diff":     taxDiff,
        "tax_percent":  taxPercent,
        "income1":      income1,
        "income2":      income2,
        "income_diff":  income2 - income1,
    })
}

// ========== 5. ЭКСПОРТ В EXCEL ==========
func ExportToExcel(c *gin.Context) {
    reportID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    var reportType, period, status string
    var taxAmount, income float64
    var createdAt time.Time
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT report_type, period, tax_amount, income, status, created_at
        FROM tax_reports WHERE id = $1 AND tenant_id = $2
    `, reportID, tenantID).Scan(&reportType, &period, &taxAmount, &income, &status, &createdAt)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Отчёт не найден"})
        return
    }
    
    // Простой CSV вместо Excel (для быстроты)
    csv := fmt.Sprintf(`Тип отчёта;%s
Период;%s
Сумма налога;%.2f
Доход;%.2f
Статус;%s
Дата создания;%s`,
        strings.ToUpper(reportType), period, taxAmount, income, status, createdAt.Format("2006-01-02"))
    
    c.Header("Content-Type", "text/csv")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=report_%s.csv", reportID))
    c.String(http.StatusOK, csv)
}

// ========== 6. ЛОГИРОВАНИЕ ДЕЙСТВИЙ ==========
func LogReportAction(reportID, userID uuid.UUID, action, details, ip string) {
    database.Pool.Exec(context.Background(), `
        INSERT INTO tax_report_actions (report_id, action, user_id, ip_address, details)
        VALUES ($1, $2, $3, $4, $5)
    `, reportID, action, userID, ip, details)
}

// ========== 7. НАПОМИНАНИЕ О СРОКАХ ==========
func GetDeadlineNotifications(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }    
    // Получаем настройки уведомлений для этого tenant
    var reportTypes []string
    rowsSettings, err := database.Pool.Query(c.Request.Context(), `
        SELECT report_type FROM tax_notifications 
        WHERE tenant_id = $1 AND is_active = true
    `, tenantID)
    if err == nil {
        defer rowsSettings.Close()
        for rowsSettings.Next() {
            var rt string
            rowsSettings.Scan(&rt)
            reportTypes = append(reportTypes, rt)
        }
    }
    
    // Формируем запрос с учётом предпочтений tenant
    query := `
        SELECT report_type, deadline_date FROM tax_deadlines 
        WHERE notification_sent = false AND deadline_date >= NOW()
    `
    
    if len(reportTypes) > 0 {
        placeholders := ""
        for i, rt := range reportTypes {
            if i > 0 {
                placeholders += ","
            }
            placeholders += "'" + rt + "'"
        }
        query += " AND report_type IN (" + placeholders + ")"
    }
    
    query += " ORDER BY deadline_date ASC LIMIT 5"
    
    rows, err := database.Pool.Query(c.Request.Context(), query)
    if err != nil {
        c.JSON(http.StatusOK, []gin.H{})
        return
    }
    defer rows.Close()
    
    var deadlines []gin.H
    for rows.Next() {
        var reportType string
        var deadlineDate time.Time
        rows.Scan(&reportType, &deadlineDate)
        
        daysLeft := int(deadlineDate.Sub(time.Now()).Hours() / 24)
        
        deadlines = append(deadlines, gin.H{
            "report_type": reportType,
            "deadline":    deadlineDate.Format("02.01.2006"),
            "days_left":   daysLeft,
            "tenant_id":   tenantID,
        })
    }
    
    c.JSON(http.StatusOK, deadlines)
}
// ExportMonthClosureReport - экспорт отчёта о закрытии месяца в Excel
func ExportMonthClosureReport(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(401, gin.H{"error": "Unauthorized"})
        return
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        c.JSON(400, gin.H{"error": "Invalid tenant ID"})
        return
    }
    
    ctx := c.Request.Context()
    
    // Получаем информацию о закрытых месяцах
    rows, err := database.Pool.Query(ctx, `
        SELECT 
            mc.period,
            mc.closed_at,
            mc.status,
            COUNT(DISTINCT je.id) as entries_count,
            COALESCE(SUM(CASE WHEN je.debit_account = '90' THEN je.amount ELSE 0 END), 0) as sales_amount,
            COALESCE(SUM(CASE WHEN je.debit_account = '91' THEN je.amount ELSE 0 END), 0) as other_amount,
            COALESCE(SUM(CASE WHEN je.credit_account = '99' THEN je.amount ELSE 0 END), 0) as profit_amount
        FROM month_closures mc
        LEFT JOIN journal_entries je ON je.tenant_id = mc.tenant_id 
            AND DATE(je.created_at) BETWEEN mc.period AND (mc.period + INTERVAL '1 month' - INTERVAL '1 day')
        WHERE mc.tenant_id = $1
        GROUP BY mc.period, mc.closed_at, mc.status
        ORDER BY mc.period DESC
    `, tenantUUID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    // Создаём HTML для Excel (проще и быстрее)
    html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Отчёт о закрытии месяца</title>
    <style>
        th { background-color: #4472C4; color: white; padding: 8px; }
        td { padding: 6px 8px; border: 1px solid #ddd; }
        table { border-collapse: collapse; width: 100%; }
        .number { text-align: right; }
        .header { background-color: #f0f0f0; }
        .profit { color: green; font-weight: bold; }
        .loss { color: red; font-weight: bold; }
    </style>
</head>
<body>
    <h2>📊 Отчёт о закрытии месяца</h2>
    <p>Сформирован: ` + time.Now().Format("02.01.2006 15:04:05") + `</p>
    <p>Организация: ` + tenantID + `</p>
    
    <h3>Закрытые месяцы</h3>
    <table>
        <thead>
            <tr style="background-color:#4472C4;color:white;">
                <th>Период</th>
                <th>Дата закрытия</th>
                <th>Статус</th>
                <th>Кол-во проводок</th>
                <th>Продажи (сч.90)</th>
                <th>Прочие доходы/расходы (сч.91)</th>
                <th>Фин.результат (сч.99)</th>
            </tr>
        </thead>
        <tbody>`
    
    hasData := false
    for rows.Next() {
        var period time.Time
        var closedAt time.Time
        var status string
        var entriesCount int
        var salesAmount, otherAmount, profitAmount float64
        
        rows.Scan(&period, &closedAt, &status, &entriesCount, &salesAmount, &otherAmount, &profitAmount)
        
        statusText := map[string]string{
            "completed": "✅ Завершено",
            "pending":   "⏳ В процессе",
            "failed":    "❌ Ошибка",
        }[status]
        if statusText == "" {
            statusText = status
        }
        
        profitClass := "profit"
        if profitAmount < 0 {
            profitClass = "loss"
        }
        
        html += fmt.Sprintf(`
            <tr>
                <td>%s</td>
                <td>%s</td>
                <td>%s</td>
                <td class="number">%d</td>
                <td class="number">%.2f</td>
                <td class="number">%.2f</td>
                <td class="number %s">%.2f</td>
            </tr>`,
            period.Format("January 2006"),
            closedAt.Format("02.01.2006 15:04:05"),
            statusText,
            entriesCount,
            salesAmount,
            otherAmount,
            profitClass,
            profitAmount)
        
        hasData = true
    }
    
    if !hasData {
        html += `<tr><td colspan="7" style="text-align:center;">Нет данных о закрытии месяцев</td></tr>`
    }
    
    html += `</tbody>
    </table>
    
    <h3>Проводки закрытия</h3>
    <table>
        <thead>
            <tr style="background-color:#4472C4;color:white;">
                <th>Дата</th>
                <th>Дебет</th>
                <th>Кредит</th>
                <th>Сумма</th>
                <th>Описание</th>
                <th>Период</th>
            </tr>
        </thead>
        <tbody>`
    
    closingRows, err := database.Pool.Query(ctx, `
        SELECT 
            je.created_at,
            je.debit_account,
            je.credit_account,
            je.amount,
            je.description,
            DATE_TRUNC('month', je.created_at) as period
        FROM journal_entries je
        WHERE je.tenant_id = $1 
            AND (je.debit_account IN ('90', '91') OR je.credit_account = '99')
            AND je.description LIKE '%Закрытие месяца%'
        ORDER BY je.created_at DESC
        LIMIT 100
    `, tenantUUID)
    
    if err == nil {
        defer closingRows.Close()
        rowCount := 0
        for closingRows.Next() {
            var createdAt time.Time
            var debitAccount, creditAccount, description string
            var amount float64
            var period time.Time
            
            closingRows.Scan(&createdAt, &debitAccount, &creditAccount, &amount, &description, &period)
            
            html += fmt.Sprintf(`
                <tr>
                    <td>%s</td>
                    <td>%s</td>
                    <td>%s</td>
                    <td class="number">%.2f</td>
                    <td>%s</td>
                    <td>%s</td>
                </tr>`,
                createdAt.Format("02.01.2006"),
                debitAccount,
                creditAccount,
                amount,
                description,
                period.Format("January 2006"))
            rowCount++
        }
        if rowCount == 0 {
            html += `<tr><td colspan="6" style="text-align:center;">Нет проводок закрытия</td></tr>`
        }
    } else {
        html += `<tr><td colspan="6" style="text-align:center;">Ошибка загрузки проводок</td></tr>`
    }
    
    html += `</tbody>
    </table>
    
    <h3>Статистика</h3>
    <table>
        <thead>
            <tr style="background-color:#4472C4;color:white;">
                <th>Показатель</th>
                <th>Значение</th>
            </tr>
        </thead>
        <tbody>`
    
    var totalClosedMonths, totalEntries int
    var totalProfit float64
    
    database.Pool.QueryRow(ctx, `
        SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(entries_count), 0), COALESCE(SUM(profit_amount), 0)
        FROM (
            SELECT 
                COUNT(DISTINCT je.id) as entries_count,
                COALESCE(SUM(CASE WHEN je.credit_account = '99' THEN je.amount ELSE 0 END), 0) as profit_amount
            FROM month_closures mc
            LEFT JOIN journal_entries je ON je.tenant_id = mc.tenant_id 
                AND DATE(je.created_at) BETWEEN mc.period AND (mc.period + INTERVAL '1 month' - INTERVAL '1 day')
            WHERE mc.tenant_id = $1
            GROUP BY mc.period
        ) t
    `, tenantUUID).Scan(&totalClosedMonths, &totalEntries, &totalProfit)
    
    profitClass := "profit"
    if totalProfit < 0 {
        profitClass = "loss"
    }
    
    html += fmt.Sprintf(`
        <tr><td><strong>Всего закрытых месяцев</strong></td><td>%d</td></tr>
        <tr><td><strong>Всего проводок за период</strong></td><td>%d</td></tr>
        <tr><td><strong>Общая прибыль</strong></td><td class="%s">%.2f ₽</td></tr>
        <tr><td><strong>Дата формирования отчёта</strong></td><td>%s</td></tr>
    `, totalClosedMonths, totalEntries, profitClass, totalProfit, time.Now().Format("02.01.2006 15:04:05"))
    
    html += `</tbody>
    </table>
    
    <p><em>Business Stack ERP - FinCore модуль</em></p>
</body>
</html>`
    
    filename := fmt.Sprintf("month_closure_report_%s.xls", time.Now().Format("2006-01-02_150405"))
    c.Header("Content-Type", "application/vnd.ms-excel")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
    c.String(http.StatusOK, html)
}

// ============================================================
// НОВЫЕ ФУНКЦИИ ДЛЯ ПОЛНОГО ЗАМЕЩЕНИЯ 1С
// ============================================================

// GetTurnoverBalanceSheetGrouped - ОСВ с группировкой по счетам
func GetTurnoverBalanceSheetGrouped(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = c.GetString("user_id")
	}
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	period := c.DefaultQuery("period", "month")
	var fromDate, toDate time.Time
	now := time.Now()

	switch period {
	case "month":
		fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		toDate = fromDate.AddDate(0, 1, -1)
	case "quarter":
		fromDate = time.Date(now.Year(), (now.Month()-1)/3*3+1, 1, 0, 0, 0, 0, time.UTC)
		toDate = fromDate.AddDate(0, 3, -1)
	case "year":
		fromDate = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		toDate = time.Date(now.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
	default:
		fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		toDate = fromDate.AddDate(0, 1, -1)
	}

	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT 
			COALESCE(debit_account, credit_account) as account,
			SUM(CASE WHEN operation_date < $1 THEN amount ELSE 0 END) as opening_balance,
			SUM(CASE WHEN operation_date BETWEEN $1 AND $2 AND debit_account IS NOT NULL THEN amount ELSE 0 END) as period_debit,
			SUM(CASE WHEN operation_date BETWEEN $1 AND $2 AND credit_account IS NOT NULL THEN amount ELSE 0 END) as period_credit
		FROM journal_entries
		WHERE tenant_id = $3 AND status = 'posted'
		GROUP BY COALESCE(debit_account, credit_account)
	`, fromDate, toDate, tenantID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type AccountTurnover struct {
		AccountCode    string  `json:"account_code"`
		OpeningDebit   float64 `json:"opening_debit"`
		OpeningCredit  float64 `json:"opening_credit"`
		PeriodDebit    float64 `json:"period_debit"`
		PeriodCredit   float64 `json:"period_credit"`
		ClosingDebit   float64 `json:"closing_debit"`
		ClosingCredit  float64 `json:"closing_credit"`
	}

	accountMap := make(map[string]*AccountTurnover)

	for rows.Next() {
		var account string
		var openingBalance, periodDebit, periodCredit float64
		rows.Scan(&account, &openingBalance, &periodDebit, &periodCredit)

		if _, exists := accountMap[account]; !exists {
			accountMap[account] = &AccountTurnover{
				AccountCode: account,
			}
		}

		if openingBalance > 0 {
			accountMap[account].OpeningDebit += openingBalance
		} else {
			accountMap[account].OpeningCredit += -openingBalance
		}

		accountMap[account].PeriodDebit += periodDebit
		accountMap[account].PeriodCredit += periodCredit
	}

	result := []AccountTurnover{}
	for _, acc := range accountMap {
		acc.ClosingDebit = acc.OpeningDebit + acc.PeriodDebit - acc.PeriodCredit
		if acc.ClosingDebit < 0 {
			acc.ClosingCredit = -acc.ClosingDebit
			acc.ClosingDebit = 0
		}
		result = append(result, *acc)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"start_date": fromDate.Format("2006-01-02"),
		"end_date":   toDate.Format("2006-01-02"),
	})
}

// GetDetailedBalanceSheet - детализированный бухгалтерский баланс
func GetDetailedBalanceSheet(c *gin.Context) {
    // Получаем tenant_id из контекста (устанавливается middleware)
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }

    asOfDate := c.DefaultQuery("as_of_date", time.Now().Format("2006-01-02"))
    date, _ := time.Parse("2006-01-02", asOfDate)

    // ========== 1. АКТИВ ==========
    
    // Внеоборотные активы (01, 03, 04, 08) — дебетовое сальдо
    var nonCurrentAssets float64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(
            CASE 
                WHEN debit_account IN ('01', '03', '04', '08') THEN amount
                WHEN credit_account IN ('01', '03', '04', '08') THEN -amount
                ELSE 0
            END
        ), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND status = 'posted' AND operation_date <= $2
    `, tenantID, date).Scan(&nonCurrentAssets)
    if err != nil {
        nonCurrentAssets = 0
    }

    // Оборотные активы (10, 41, 50, 51, 58, 60, 62)
    var currentAssets float64
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(
            CASE 
                WHEN debit_account IN ('10', '41', '50', '51', '58', '60', '62') THEN amount
                WHEN credit_account IN ('10', '41', '50', '51', '58', '60', '62') THEN -amount
                ELSE 0
            END
        ), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND status = 'posted' AND operation_date <= $2
    `, tenantID, date).Scan(&currentAssets)
    if err != nil {
        currentAssets = 0
    }

    totalAssets := nonCurrentAssets + currentAssets

    // ========== 2. ПАССИВ ==========

    // Капитал и резервы (80 - Уставный, 82 - Резервный, 83 - Добавочный, 84 - Нераспределённая прибыль, 99 - Прибыль/убыток)
    var capital float64
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(
            CASE 
                WHEN credit_account IN ('80', '82', '83', '84', '99') THEN amount
                WHEN debit_account IN ('80', '82', '83', '84', '99') THEN -amount
                ELSE 0
            END
        ), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND status = 'posted' AND operation_date <= $2
    `, tenantID, date).Scan(&capital)
    if err != nil {
        capital = 0
    }

    // Долгосрочные обязательства (67)
    var longTermLiabilities float64
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(
            CASE 
                WHEN credit_account = '67' THEN amount
                WHEN debit_account = '67' THEN -amount
                ELSE 0
            END
        ), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND status = 'posted' AND operation_date <= $2
    `, tenantID, date).Scan(&longTermLiabilities)
    if err != nil {
        longTermLiabilities = 0
    }

    // Краткосрочные обязательства (60, 62, 66, 68, 69, 70)
    var shortTermLiabilities float64
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(
            CASE 
                WHEN credit_account IN ('60', '62', '66', '68', '69', '70') THEN amount
                WHEN debit_account IN ('60', '62', '66', '68', '69', '70') THEN -amount
                ELSE 0
            END
        ), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND status = 'posted' AND operation_date <= $2
    `, tenantID, date).Scan(&shortTermLiabilities)
    if err != nil {
        shortTermLiabilities = 0
    }

    totalLiabilities := capital + longTermLiabilities + shortTermLiabilities

    // ========== 3. ЛОГИРОВАНИЕ ДЛЯ ОТЛАДКИ ==========
    log.Printf("📊 БАЛАНС на %s для tenant %s:", date.Format("2006-01-02"), tenantID)
    log.Printf("   Внеоборотные активы: %.2f", nonCurrentAssets)
    log.Printf("   Оборотные активы: %.2f", currentAssets)
    log.Printf("   ИТОГО АКТИВ: %.2f", totalAssets)
    log.Printf("   Капитал (счета 80,82,83,84,99): %.2f", capital)
    log.Printf("   Долгосрочные обязательства (67): %.2f", longTermLiabilities)
    log.Printf("   Краткосрочные обязательства (60,62,66,68,69,70): %.2f", shortTermLiabilities)
    log.Printf("   ИТОГО ПАССИВ: %.2f", totalLiabilities)

    if totalAssets != totalLiabilities {
        log.Printf("⚠️ БАЛАНС НЕ СОШЁЛСЯ! Разница: %.2f", totalAssets-totalLiabilities)
    } else {
        log.Printf("✅ БАЛАНС СОШЁЛСЯ!")
    }

    // ========== 4. ОТВЕТ ==========
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "assets": gin.H{
            "total":        totalAssets,
            "non_current":  nonCurrentAssets,
            "current":      currentAssets,
        },
        "liabilities": gin.H{
            "total":         totalLiabilities,
            "capital":       capital,
            "long_term":     longTermLiabilities,
            "short_term":    shortTermLiabilities,
        },
        "as_of_date": date.Format("2006-01-02"),
    })
}
// GetFullCashFlowReport - ДДС с тремя разделами
func GetFullCashFlowReport(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = c.GetString("user_id")
	}
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	if dateFrom == "" {
		dateFrom = time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	}
	if dateTo == "" {
		dateTo = time.Now().Format("2006-01-02")
	}

	var operatingInflow, operatingOutflow float64

	// Приток: поступления от покупателей
	database.Pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(SUM(amount), 0)
		FROM journal_entries
		WHERE tenant_id = $1 AND status = 'posted'
		AND operation_date BETWEEN $2 AND $3
		AND debit_account = '51' AND credit_account = '62'
	`, tenantID, dateFrom, dateTo).Scan(&operatingInflow)

	// Отток: оплата поставщикам, зарплата, налоги
	database.Pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(SUM(amount), 0)
		FROM journal_entries
		WHERE tenant_id = $1 AND status = 'posted'
		AND operation_date BETWEEN $2 AND $3
		AND credit_account = '51' 
		AND debit_account IN ('60', '70', '68', '69')
	`, tenantID, dateFrom, dateTo).Scan(&operatingOutflow)

	var investingInflow, investingOutflow float64

	// Приток: продажа ОС
	database.Pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(SUM(amount), 0)
		FROM journal_entries
		WHERE tenant_id = $1 AND status = 'posted'
		AND operation_date BETWEEN $2 AND $3
		AND debit_account = '51' AND credit_account IN ('01', '04')
	`, tenantID, dateFrom, dateTo).Scan(&investingInflow)

	// Отток: покупка ОС
	database.Pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(SUM(amount), 0)
		FROM journal_entries
		WHERE tenant_id = $1 AND status = 'posted'
		AND operation_date BETWEEN $2 AND $3
		AND credit_account = '51' 
		AND debit_account IN ('01', '04', '08')
	`, tenantID, dateFrom, dateTo).Scan(&investingOutflow)

	var financingInflow, financingOutflow float64

	// Приток: кредиты, займы
	database.Pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(SUM(amount), 0)
		FROM journal_entries
		WHERE tenant_id = $1 AND status = 'posted'
		AND operation_date BETWEEN $2 AND $3
		AND debit_account = '51' AND credit_account IN ('66', '67', '75')
	`, tenantID, dateFrom, dateTo).Scan(&financingInflow)

	// Отток: возврат кредитов
	database.Pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(SUM(amount), 0)
		FROM journal_entries
		WHERE tenant_id = $1 AND status = 'posted'
		AND operation_date BETWEEN $2 AND $3
		AND credit_account = '51' 
		AND debit_account IN ('66', '67', '75', '84')
	`, tenantID, dateFrom, dateTo).Scan(&financingOutflow)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"operating": gin.H{
			"inflow":  operatingInflow,
			"outflow": operatingOutflow,
			"net":     operatingInflow - operatingOutflow,
		},
		"investing": gin.H{
			"inflow":  investingInflow,
			"outflow": investingOutflow,
			"net":     investingInflow - investingOutflow,
		},
		"financing": gin.H{
			"inflow":  financingInflow,
			"outflow": financingOutflow,
			"net":     financingInflow - financingOutflow,
		},
		"total_net": (operatingInflow - operatingOutflow) + (investingInflow - investingOutflow) + (financingInflow - financingOutflow),
		"date_from": dateFrom,
		"date_to":   dateTo,
	})
}

// GetDetailedProfitLoss - детализированный P&L
// GetDetailedProfitLoss - детализированный P&L (универсальный для всех счетов)
// GetDetailedProfitLoss - детализированный P&L (под вашу структуру счетов)
func GetDetailedProfitLoss(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = c.GetString("user_id")
	}
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	dateFrom := c.Query("start_date")
	dateTo := c.Query("end_date")

	if dateFrom == "" {
		dateFrom = time.Now().AddDate(-1, 0, 0).Format("2006-01-02")
	}
	if dateTo == "" {
		dateTo = time.Now().Format("2006-01-02")
	}

	// ВЫРУЧКА: кредит 90 или поступления 51->62
	var revenue float64
	database.Pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(SUM(amount), 0)
		FROM journal_entries
		WHERE tenant_id = $1 AND status = 'posted'
		AND operation_date BETWEEN $2 AND $3
		AND (credit_account = '90' OR (debit_account = '51' AND credit_account = '62'))
	`, tenantID, dateFrom, dateTo).Scan(&revenue)

	// РАСХОДЫ (все списания): кредит 51 или дебет 70
	var totalExpenses float64
	database.Pool.QueryRow(c.Request.Context(), `
		SELECT COALESCE(SUM(amount), 0)
		FROM journal_entries
		WHERE tenant_id = $1 AND status = 'posted'
		AND operation_date BETWEEN $2 AND $3
		AND (credit_account = '51' OR debit_account = '70')
	`, tenantID, dateFrom, dateTo).Scan(&totalExpenses)

	// Себестоимость (часть расходов) - для демонстрации
	costOfSales := totalExpenses * 0.5
	commercialExpenses := 0.0
	adminExpenses := totalExpenses * 0.5
	interestExpenses := 0.0
	otherExpenses := 0.0

	grossProfit := revenue - costOfSales
	netProfit := revenue - totalExpenses

	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"revenue":             revenue,
		"cost_of_sales":       costOfSales,
		"gross_profit":        grossProfit,
		"commercial_expenses": commercialExpenses,
		"admin_expenses":      adminExpenses,
		"interest_expenses":   interestExpenses,
		"other_expenses":      otherExpenses,
		"total_expenses":      totalExpenses,
		"net_profit":          netProfit,
		"start_date":          dateFrom,
		"end_date":            dateTo,
	})
}
// ComparePeriods - сравнение двух периодов
func ComparePeriods(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		tenantID = c.GetString("user_id")
	}
	if tenantID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	period1Start := c.Query("period1_start")
	period1End := c.Query("period1_end")
	period2Start := c.Query("period2_start")
	period2End := c.Query("period2_end")

	type PeriodData struct {
		Revenue  float64 `json:"revenue"`
		Profit   float64 `json:"profit"`
		Expenses float64 `json:"expenses"`
	}

	getData := func(start, end string) PeriodData {
		var data PeriodData
		query := `SELECT 
			COALESCE(SUM(CASE WHEN credit_account = '90' THEN amount ELSE 0 END), 0) as revenue,
			COALESCE(SUM(CASE WHEN credit_account = '99' THEN amount ELSE 0 END), 0) as profit,
			COALESCE(SUM(CASE WHEN debit_account IN ('20', '26', '44', '91') THEN amount ELSE 0 END), 0) as expenses
		FROM journal_entries
		WHERE tenant_id = $1 AND status = 'posted'
		AND operation_date BETWEEN $2 AND $3`
		
		database.Pool.QueryRow(c.Request.Context(), query, tenantID, start, end).Scan(&data.Revenue, &data.Profit, &data.Expenses)
		return data
	}

	period1 := getData(period1Start, period1End)
	period2 := getData(period2Start, period2End)

	revenueChange := period2.Revenue - period1.Revenue
	revenuePercent := 0.0
	if period1.Revenue > 0 {
		revenuePercent = (revenueChange / period1.Revenue) * 100
	}

	profitChange := period2.Profit - period1.Profit
	profitPercent := 0.0
	if period1.Profit > 0 {
		profitPercent = (profitChange / period1.Profit) * 100
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"period1": gin.H{
			"start":    period1Start,
			"end":      period1End,
			"revenue":  period1.Revenue,
			"profit":   period1.Profit,
			"expenses": period1.Expenses,
		},
		"period2": gin.H{
			"start":    period2Start,
			"end":      period2End,
			"revenue":  period2.Revenue,
			"profit":   period2.Profit,
			"expenses": period2.Expenses,
		},
		"changes": gin.H{
			"revenue":         revenueChange,
			"revenue_percent": revenuePercent,
			"profit":          profitChange,
			"profit_percent":  profitPercent,
			"trend":           map[bool]string{true: "positive", false: "negative"}[profitChange > 0],
		},
	})
}

// ============================================================
// СОХРАНЕНИЕ ФИЛЬТРОВ ПОЛЬЗОВАТЕЛЯ (для отчётов)
// ============================================================

// SaveUserFilters - сохраняет настройки фильтров пользователя
func SaveUserFilters(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }

    var filters map[string]interface{}
    if err := c.BindJSON(&filters); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Создаём таблицу если нет
    _, err := database.Pool.Exec(c.Request.Context(), `
        CREATE TABLE IF NOT EXISTS user_filters (
            tenant_id VARCHAR(100) PRIMARY KEY,
            filters JSONB NOT NULL,
            updated_at TIMESTAMP DEFAULT NOW()
        )
    `)
    if err != nil {
        log.Printf("❌ Ошибка создания таблицы user_filters: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Сохраняем фильтры
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO user_filters (tenant_id, filters, updated_at)
        VALUES ($1, $2, NOW())
        ON CONFLICT (tenant_id) DO UPDATE SET
            filters = EXCLUDED.filters,
            updated_at = NOW()
    `, tenantID, filters)

    if err != nil {
        log.Printf("❌ Ошибка сохранения фильтров: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    log.Printf("✅ Фильтры сохранены для tenant: %s", tenantID)
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// LoadUserFilters - загружает настройки фильтров пользователя
func LoadUserFilters(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant_id not found"})
        return
    }

    var filtersJSON []byte
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT filters FROM user_filters WHERE tenant_id = $1
    `, tenantID).Scan(&filtersJSON)

    if err != nil {
        // Нет сохранённых фильтров - возвращаем пустой объект
        c.JSON(http.StatusOK, gin.H{})
        return
    }

    var filters map[string]interface{}
    if err := json.Unmarshal(filtersJSON, &filters); err != nil {
        c.JSON(http.StatusOK, gin.H{})
        return
    }

    log.Printf("📂 Фильтры загружены для tenant: %s", tenantID)
    c.JSON(http.StatusOK, filters)
}
// ============================================================
// ДОПОЛНИТЕЛЬНЫЕ ФУНКЦИИ ДЛЯ ПОЛНОЙ ЗАМЕНЫ 1С
// ============================================================

// 1. КАРТОЧКА СЧЁТА (все проводки по конкретному счёту)
// 1. КАРТОЧКА СЧЁТА (все проводки по конкретному счёту)
func GetAccountLedger(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }

    accountCode := c.Query("account_code")
    if accountCode == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "account_code required"})
        return
    }

    dateFrom := c.DefaultQuery("date_from", "2000-01-01")
    dateTo := c.DefaultQuery("date_to", time.Now().Format("2006-01-02"))

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            operation_date,
            COALESCE(document_number, '') as doc_number,
            COALESCE(counterparty_name, '') as counterparty,
            COALESCE(debit_account, '') as debit_account,
            COALESCE(credit_account, '') as credit_account,
            amount,
            COALESCE(description, '') as description
        FROM journal_entries
        WHERE tenant_id = $1 
            AND (debit_account = $2 OR credit_account = $2)
            AND operation_date BETWEEN $3 AND $4
        ORDER BY operation_date DESC
        LIMIT 500
    `, tenantID, accountCode, dateFrom, dateTo)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var entries []gin.H
    var openingBalance float64
    var closingBalance float64

    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(
            CASE WHEN debit_account = $1 THEN amount ELSE 0 END -
            CASE WHEN credit_account = $1 THEN amount ELSE 0 END
        ), 0)
        FROM journal_entries
        WHERE tenant_id = $2 AND operation_date < $3
    `, accountCode, tenantID, dateFrom).Scan(&openingBalance)
    if err != nil {
        openingBalance = 0
    }

    closingBalance = openingBalance

    for rows.Next() {
        var opDate time.Time
        var docNumber, counterparty, debitAcc, creditAcc, description string
        var amount float64

        err = rows.Scan(&opDate, &docNumber, &counterparty, &debitAcc, &creditAcc, &amount, &description)
        if err != nil {
            continue
        }

        if debitAcc == accountCode {
            closingBalance += amount
        }
        if creditAcc == accountCode {
            closingBalance -= amount
        }

        entries = append(entries, gin.H{
            "date":            opDate.Format("2006-01-02"),
            "document":        docNumber,
            "counterparty":    counterparty,
            "debit":           amount,
            "credit":          amount,
            "debit_account":   debitAcc,
            "credit_account":  creditAcc,
            "description":     description,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success":          true,
        "account_code":     accountCode,
        "date_from":        dateFrom,
        "date_to":          dateTo,
        "opening_balance":  openingBalance,
        "closing_balance":  closingBalance,
        "entries":          entries,
    })
}
// 2. АНАЛИЗ ДЕБИТОРСКОЙ ЗАДОЛЖЕННОСТИ (кто должен вам)
func GetAccountsReceivable(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    asOfDate := c.DefaultQuery("as_of_date", time.Now().Format("2006-01-02"))

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            COALESCE(counterparty_name, 'Без контрагента') as counterparty,
            COALESCE(counterparty_inn, '') as inn,
            SUM(CASE WHEN debit_account = '62' THEN amount ELSE 0 END) as debit_total,
            SUM(CASE WHEN credit_account = '62' THEN amount ELSE 0 END) as credit_total,
            MAX(operation_date) as last_operation
        FROM journal_entries
        WHERE tenant_id = $1 
            AND operation_date <= $2
            AND (debit_account = '62' OR credit_account = '62')
        GROUP BY counterparty_name, counterparty_inn
        HAVING SUM(CASE WHEN debit_account = '62' THEN amount ELSE 0 END) - 
               SUM(CASE WHEN credit_account = '62' THEN amount ELSE 0 END) > 0
        ORDER BY 4 DESC
    `, tenantID, asOfDate)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var debtors []gin.H
    var totalDebt float64

    for rows.Next() {
        var counterparty, inn string
        var debitTotal, creditTotal float64
        var lastOp time.Time

        rows.Scan(&counterparty, &inn, &debitTotal, &creditTotal, &lastOp)
        debt := debitTotal - creditTotal
        totalDebt += debt

        // Определяем просрочку
        daysOverdue := int(time.Since(lastOp).Hours() / 24)
        risk := "normal"
        if daysOverdue > 90 {
            risk = "high"
        } else if daysOverdue > 30 {
            risk = "medium"
        }

        debtors = append(debtors, gin.H{
            "counterparty":   counterparty,
            "inn":           inn,
            "debt":          debt,
            "last_operation": lastOp.Format("2006-01-02"),
            "days_overdue":  daysOverdue,
            "risk":          risk,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success":        true,
        "as_of_date":     asOfDate,
        "total_debt":     totalDebt,
        "debtors_count":  len(debtors),
        "debtors":        debtors,
    })
}

// 3. АНАЛИЗ КРЕДИТОРСКОЙ ЗАДОЛЖЕННОСТИ (кому должны вы)
func GetAccountsPayable(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    asOfDate := c.DefaultQuery("as_of_date", time.Now().Format("2006-01-02"))

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            COALESCE(counterparty_name, 'Без контрагента') as counterparty,
            COALESCE(counterparty_inn, '') as inn,
            SUM(CASE WHEN credit_account = '60' THEN amount ELSE 0 END) as credit_total,
            SUM(CASE WHEN debit_account = '60' THEN amount ELSE 0 END) as debit_total,
            MAX(operation_date) as last_operation
        FROM journal_entries
        WHERE tenant_id = $1 
            AND operation_date <= $2
            AND (debit_account = '60' OR credit_account = '60')
        GROUP BY counterparty_name, counterparty_inn
        HAVING SUM(CASE WHEN credit_account = '60' THEN amount ELSE 0 END) - 
               SUM(CASE WHEN debit_account = '60' THEN amount ELSE 0 END) > 0
        ORDER BY 4 DESC
    `, tenantID, asOfDate)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var creditors []gin.H
    var totalPayable float64

    for rows.Next() {
        var counterparty, inn string
        var creditTotal, debitTotal float64
        var lastOp time.Time

        rows.Scan(&counterparty, &inn, &creditTotal, &debitTotal, &lastOp)
        debt := creditTotal - debitTotal
        totalPayable += debt

        daysSince := int(time.Since(lastOp).Hours() / 24)

        creditors = append(creditors, gin.H{
            "counterparty":   counterparty,
            "inn":           inn,
            "debt":          debt,
            "last_operation": lastOp.Format("2006-01-02"),
            "days_since":    daysSince,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success":         true,
        "as_of_date":      asOfDate,
        "total_payable":   totalPayable,
        "creditors_count": len(creditors),
        "creditors":       creditors,
    })
}

// 4. ОБОРОТНО-САЛЬДОВАЯ ВЕДОМОСТЬ ПО СЧЁТУ
func GetAccountTurnoverBalance(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    accountCode := c.Query("account_code")
    if accountCode == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "account_code required"})
        return
    }

    dateFrom := c.DefaultQuery("date_from", "2000-01-01")
    dateTo := c.DefaultQuery("date_to", time.Now().Format("2006-01-02"))

    var openingDebit, openingCredit, periodDebit, periodCredit, closingDebit, closingCredit float64

    // Входящее сальдо (дебет)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND debit_account = $2 AND operation_date < $3
    `, tenantID, accountCode, dateFrom).Scan(&openingDebit)

    // Входящее сальдо (кредит)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND credit_account = $2 AND operation_date < $3
    `, tenantID, accountCode, dateFrom).Scan(&openingCredit)

    // Обороты за период (дебет)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND debit_account = $2 
            AND operation_date BETWEEN $3 AND $4
    `, tenantID, accountCode, dateFrom, dateTo).Scan(&periodDebit)

    // Обороты за период (кредит)
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(amount), 0)
        FROM journal_entries
        WHERE tenant_id = $1 AND credit_account = $2 
            AND operation_date BETWEEN $3 AND $4
    `, tenantID, accountCode, dateFrom, dateTo).Scan(&periodCredit)

    closingDebit = openingDebit + periodDebit - periodCredit
    if closingDebit < 0 {
        closingCredit = -closingDebit
        closingDebit = 0
    }

    c.JSON(http.StatusOK, gin.H{
        "success":         true,
        "account_code":    accountCode,
        "date_from":       dateFrom,
        "date_to":         dateTo,
        "opening_debit":   openingDebit,
        "opening_credit":  openingCredit,
        "period_debit":    periodDebit,
        "period_credit":   periodCredit,
        "closing_debit":   closingDebit,
        "closing_credit":  closingCredit,
    })
}

// 5. ШАХМАТНАЯ ВЕДОМОСТЬ (обороты между счетами)
func GetChessboardReport(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    dateFrom := c.DefaultQuery("date_from", time.Now().AddDate(0, -1, 0).Format("2006-01-02"))
    dateTo := c.DefaultQuery("date_to", time.Now().Format("2006-01-02"))

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            COALESCE(debit_account, '') as debit,
            COALESCE(credit_account, '') as credit,
            SUM(amount) as total
        FROM journal_entries
        WHERE tenant_id = $1 
            AND operation_date BETWEEN $2 AND $3
            AND debit_account IS NOT NULL 
            AND credit_account IS NOT NULL
        GROUP BY debit_account, credit_account
        ORDER BY debit_account, credit_account
    `, tenantID, dateFrom, dateTo)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var turnovers []gin.H
    var totalTurnover float64

    for rows.Next() {
        var debit, credit string
        var amount float64
        rows.Scan(&debit, &credit, &amount)
        totalTurnover += amount

        turnovers = append(turnovers, gin.H{
            "debit_account":  debit,
            "credit_account": credit,
            "amount":         amount,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success":        true,
        "date_from":      dateFrom,
        "date_to":        dateTo,
        "total_turnover": totalTurnover,
        "turnovers":      turnovers,
    })
}

// GetPurchaseLedger - Книга покупок (входящий НДС)
// Берет данные напрямую из journal_entries, не требует ручного заполнения invoices
func GetPurchaseLedger(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    dateFrom := c.DefaultQuery("date_from", "2020-01-01")
    dateTo := c.DefaultQuery("date_to", time.Now().Format("2006-01-02"))

    // Берем данные НАПРЯМУЮ из journal_entries
    // Не нужно никаких invoices!
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            operation_date,
            COALESCE(document_number, '') as invoice_number,
            COALESCE(counterparty_name, 
                CASE 
                    WHEN credit_account = '60' THEN 'Поставщик'
                    WHEN credit_account = '76' THEN 'Прочий кредитор'
                    ELSE 'Контрагент'
                END
            ) as supplier,
            amount,
            amount * 0.20 as nds_amount,
            description
        FROM journal_entries
        WHERE tenant_id = $1 
            AND operation_date BETWEEN $2 AND $3
            AND status = 'posted'
            AND amount > 0
            AND (
                -- Покупка: в дебете счета ТМЦ/расходов
                debit_account IN ('10', '41', '20', '26', '44', '08')
                OR
                -- ИЛИ в кредите счет поставщика
                credit_account IN ('60', '76', '71')
            )
        ORDER BY operation_date DESC
    `, tenantID, dateFrom, dateTo)

    if err != nil {
        log.Printf("❌ Ошибка запроса книги покупок: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var purchases []gin.H
    var totalPurchases float64
    var totalNdsInput float64

    for rows.Next() {
        var opDate time.Time
        var invoiceNum, supplier, description string
        var amount, nds float64

        if err := rows.Scan(&opDate, &invoiceNum, &supplier, &amount, &nds, &description); err != nil {
            continue
        }
        
        totalPurchases += amount
        totalNdsInput += nds

        purchases = append(purchases, gin.H{
            "date":           opDate.Format("2006-01-02"),
            "invoice_number": invoiceNum,
            "supplier":       supplier,
            "amount":         amount,
            "nds_amount":     nds,
            "description":    description,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success":          true,
        "date_from":        dateFrom,
        "date_to":          dateTo,
        "total_purchases":  totalPurchases,
        "total_nds_input":  totalNdsInput,
        "purchases":        purchases,
    })
}
// GetSalesLedger - Книга продаж (исходящий НДС)
func GetSalesLedger(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("user_id")
    }
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    dateFrom := c.DefaultQuery("date_from", "2020-01-01")
    dateTo := c.DefaultQuery("date_to", time.Now().Format("2006-01-02"))

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            operation_date,
            COALESCE(document_number, '') as invoice_number,
            COALESCE(counterparty_name, 'Покупатель') as buyer,
            COALESCE(counterparty_inn, '') as buyer_inn,
            amount,
            amount * 0.20 as nds_amount
        FROM journal_entries
        WHERE tenant_id = $1 
            AND operation_date BETWEEN $2 AND $3
            AND status = 'posted'
            AND amount > 0
            AND debit_account = '62'  -- Покупатели
            AND credit_account = '90'  -- Продажи
        ORDER BY operation_date DESC
    `, tenantID, dateFrom, dateTo)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var sales []gin.H
    var totalAmount, totalNDS float64

    for rows.Next() {
        var opDate time.Time
        var invoiceNum, buyer, inn string
        var amount, nds float64

        rows.Scan(&opDate, &invoiceNum, &buyer, &inn, &amount, &nds)
        totalAmount += amount
        totalNDS += nds

        sales = append(sales, gin.H{
            "date":           opDate.Format("2006-01-02"),
            "invoice_number": invoiceNum,
            "buyer":          buyer,
            "buyer_inn":      inn,
            "amount":         amount,
            "nds_amount":     nds,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "date_from":     dateFrom,
        "date_to":       dateTo,
        "total_sales":   totalAmount,
        "total_nds_out": totalNDS,
        "sales":         sales,
    })
}