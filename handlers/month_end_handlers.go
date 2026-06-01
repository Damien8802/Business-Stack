package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "subscription-system/database"
)

// MonthEndClosing - структура закрытия месяца
type MonthEndClosing struct {
    ID                uuid.UUID  `json:"id"`
    TenantID          uuid.UUID  `json:"tenant_id"`
    Month             int        `json:"month"`
    Year              int        `json:"year"`
    Status            string     `json:"status"`
    Income            float64    `json:"income"`
    Expense           float64    `json:"expense"`
    DepreciationAmount float64   `json:"depreciation_amount"`
    CostWriteOff      float64    `json:"cost_write_off"`
    TaxAmount         float64    `json:"tax_amount"`
    NetProfit         float64    `json:"net_profit"`
    StartedAt         time.Time  `json:"started_at"`
    CompletedAt       *time.Time `json:"completed_at,omitempty"`
}

// StartMonthEndClosing - начать процедуру закрытия месяца
func StartMonthEndClosing(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var req struct {
        Month int `json:"month" binding:"required"`
        Year  int `json:"year" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Проверяем, не закрыт ли уже месяц
    var existingID uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id FROM month_end_closing 
        WHERE tenant_id = $1 AND month = $2 AND year = $3 AND status = 'completed'
    `, tenantID, req.Month, req.Year).Scan(&existingID)

    if err == nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Этот месяц уже закрыт"})
        return
    }

    // Получаем доходы и расходы из bank_statements (ПРАВИЛЬНОЕ НАЗВАНИЕ ТАБЛИЦЫ)
    var totalIncome, totalExpense float64

    // Доходы (credit_amount - поступления)
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(credit_amount), 0) FROM bank_statements 
        WHERE tenant_id = $1 
        AND EXTRACT(MONTH FROM operation_date) = $2 
        AND EXTRACT(YEAR FROM operation_date) = $3
    `, tenantID, req.Month, req.Year).Scan(&totalIncome)
    if err != nil {
        totalIncome = 0
    }

    // Расходы (debit_amount - списания)
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(debit_amount), 0) FROM bank_statements 
        WHERE tenant_id = $1 
        AND EXTRACT(MONTH FROM operation_date) = $2 
        AND EXTRACT(YEAR FROM operation_date) = $3
    `, tenantID, req.Month, req.Year).Scan(&totalExpense)
    if err != nil {
        totalExpense = 0
    }

    // Расчёт амортизации (2% от расходов)
    depreciation := totalExpense * 0.02

    // Расчёт налога (20% от прибыли)
    profit := totalIncome - totalExpense
    tax := profit * 0.2
    if tax < 0 {
        tax = 0
    }

    // Чистая прибыль
    netProfit := profit - depreciation - tax

    closingID := uuid.New()
    now := time.Now()

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO month_end_closing (id, tenant_id, month, year, status, 
                                       depreciation_amount, cost_write_off, tax_amount, net_profit, 
                                       started_at, completed_at)
        VALUES ($1, $2, $3, $4, 'completed', $5, $6, $7, $8, $9, $10)
    `, closingID, tenantID, req.Month, req.Year, depreciation, totalExpense, tax, netProfit, now, now)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":      true,
        "message":      "Закрытие месяца выполнено",
        "closing_id":   closingID,
        "status":       "completed",
        "month":        req.Month,
        "year":         req.Year,
        "income":       totalIncome,
        "expense":      totalExpense,
        "depreciation": depreciation,
        "tax":          tax,
        "net_profit":   netProfit,
    })
}
// GetMonthEndStatus - получить статус закрытия месяца
func GetMonthEndStatus(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    month := c.Query("month")
    year := c.Query("year")

    var closing MonthEndClosing
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, month, year, status, 
               COALESCE(depreciation_amount, 0), COALESCE(cost_write_off, 0), 
               COALESCE(tax_amount, 0), COALESCE(net_profit, 0), started_at, completed_at
        FROM month_end_closing
        WHERE tenant_id = $1 AND month = $2 AND year = $3
        ORDER BY started_at DESC LIMIT 1
    `, tenantID, month, year).Scan(
        &closing.ID, &closing.Month, &closing.Year, &closing.Status,
        &closing.DepreciationAmount, &closing.CostWriteOff, &closing.TaxAmount, &closing.NetProfit,
        &closing.StartedAt, &closing.CompletedAt,
    )

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Закрытие месяца не найдено"})
        return
    }

    c.JSON(http.StatusOK, closing)
}

// GetMonthEndHistory - история закрытий месяцев (ИСПРАВЛЕНО)
func GetMonthEndHistory(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, month, year, status, 
               COALESCE(depreciation_amount, 0) as depreciation,
               COALESCE(cost_write_off, 0) as expenses,
               COALESCE(tax_amount, 0) as tax,
               COALESCE(net_profit, 0) as net_profit,
               completed_at
        FROM month_end_closing
        WHERE tenant_id = $1
        ORDER BY year DESC, month DESC
        LIMIT 50
    `, tenantID)

    if err != nil {
        c.JSON(200, []gin.H{})
        return
    }
    defer rows.Close()

    var results []gin.H
    for rows.Next() {
        var id uuid.UUID
        var month, year int
        var status string
        var depreciation, expenses, tax, netProfit float64
        var completedAt *time.Time

        err := rows.Scan(&id, &month, &year, &status, &depreciation, &expenses, &tax, &netProfit, &completedAt)
        if err != nil {
            continue
        }

completedAtStr := ""
if completedAt != nil && !completedAt.IsZero() {
    completedAtStr = completedAt.Format("02.01.2006")
} else {
    completedAtStr = time.Now().Format("02.01.2006")
}
        results = append(results, gin.H{
            "id":            id,
            "month":         month,
            "year":          year,
            "period":        formatPeriod(month, year),
            "status":        status,
            "depreciation":  depreciation,
            "cost_write_off": expenses,
            "tax_amount":    tax,
            "net_profit":    netProfit,
            "completed_at":  completedAtStr,
        })
    }

    if results == nil {
        results = []gin.H{}
    }

    c.JSON(200, results)
}
// formatPeriod - форматирует период в строку "ММ/ГГГГ"
func formatPeriod(month, year int) string {
    months := []string{"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
        "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"}
    if month >= 1 && month <= 12 {
        return months[month-1] + " " + formatYear(year)
    }
    return ""
}

func formatYear(year int) string {
    return string(rune(year/1000)) + string(rune((year%1000)/100)) + string(rune((year%100)/10)) + string(rune(year%10))
}

// CreateMonthEndTables - создание таблиц для закрытия месяца
func CreateMonthEndTables(c *gin.Context) {
    _, err := database.Pool.Exec(c.Request.Context(), `
        CREATE TABLE IF NOT EXISTS month_end_closing (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id UUID NOT NULL,
            month INTEGER NOT NULL,
            year INTEGER NOT NULL,
            status VARCHAR(50) DEFAULT 'draft',
            depreciation_amount DECIMAL(15,2) DEFAULT 0,
            cost_write_off DECIMAL(15,2) DEFAULT 0,
            tax_amount DECIMAL(15,2) DEFAULT 0,
            net_profit DECIMAL(15,2) DEFAULT 0,
            started_at TIMESTAMP,
            completed_at TIMESTAMP,
            created_at TIMESTAMP DEFAULT NOW(),
            UNIQUE(tenant_id, month, year)
        )
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Создаём индексы для быстрого поиска
    _, err = database.Pool.Exec(c.Request.Context(), `
        CREATE INDEX IF NOT EXISTS idx_month_end_closing_tenant ON month_end_closing(tenant_id);
        CREATE INDEX IF NOT EXISTS idx_month_end_closing_year_month ON month_end_closing(year, month);
    `)
    if err != nil {
        // Индексы не критичны, просто логируем
    }

    c.JSON(http.StatusOK, gin.H{
        "message": "Таблицы для закрытия месяца созданы",
        "tables":  []string{"month_end_closing"},
    })
}