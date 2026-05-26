package handlers

import (
    "fmt" 
    "net/http"
    "strconv"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)

// GetBudgets - получить бюджеты для тега
func GetBudgets(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    tagID := c.Query("tag_id")
    year := c.DefaultQuery("year", strconv.Itoa(time.Now().Year()))
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT tag_id, year, month, planned_amount, actual_amount
        FROM fincore_budgets
        WHERE tenant_id = $1 AND year = $2
        AND ($3 = '' OR tag_id = $3::uuid)
        ORDER BY month
    `, tenantID, year, tagID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var budgets []gin.H
    for rows.Next() {
        var tagID, year, month string
        var planned, actual float64
        
        rows.Scan(&tagID, &year, &month, &planned, &actual)
        
        budgets = append(budgets, gin.H{
            "tag_id":   tagID,
            "year":     year,
            "month":    month,
            "planned":  planned,
            "actual":   actual,
            "variance": actual - planned,
            "percent": func() float64 {
                if planned == 0 {
                    return 0
                }
                return (actual / planned) * 100
            }(),
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    budgets,
    })
}

// UpdateBudget - обновить бюджет
func UpdateBudget(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    var req struct {
        TagID   string  `json:"tag_id"`
        Year    int     `json:"year"`
        Month   int     `json:"month"`
        Planned float64 `json:"planned"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO fincore_budgets (tenant_id, tag_id, year, month, planned_amount, updated_at)
        VALUES ($1, $2, $3, $4, $5, NOW())
        ON CONFLICT (tenant_id, tag_id, year, month) 
        DO UPDATE SET planned_amount = EXCLUDED.planned_amount, updated_at = NOW()
    `, tenantID, req.TagID, req.Year, req.Month, req.Planned)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Бюджет сохранён"})
}

// ========== ПЛАН-ФАКТ АНАЛИЗ ==========

// GetPlanFactAnalysis - анализ план-факт
func GetPlanFactAnalysis(c *gin.Context) {
    userID := getCurrentUserID(c)
    period := c.DefaultQuery("period", "month")
    
    var fromDate, toDate time.Time
    now := time.Now()
    switch period {
    case "month":
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
    case "quarter":
        fromDate = time.Date(now.Year(), now.Month()-2, 1, 0, 0, 0, 0, time.UTC)
    case "year":
        fromDate = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
    default:
        fromDate = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
    }
    toDate = now
    
    // Фактические доходы и расходы
    var actualIncome, actualExpense float64
    query := `
        SELECT 
            COALESCE(SUM(CASE WHEN debit_account LIKE '90%%' AND credit_account LIKE '90%%' THEN amount ELSE 0 END), 0) as income,
            COALESCE(SUM(CASE WHEN credit_account LIKE '90%%' AND debit_account LIKE '20%%' THEN amount ELSE 0 END), 0) as expense
        FROM journal_entries
        WHERE user_id = $1 AND date BETWEEN $2 AND $3
    `
    database.Pool.QueryRow(c.Request.Context(), query, userID, fromDate, toDate).Scan(&actualIncome, &actualExpense)
    
    // Плановые данные (из таблицы budgets)
    var planIncome, planExpense float64
    planQuery := `
        SELECT 
            COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0) as plan_income,
            COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0) as plan_expense
        FROM budgets
        WHERE user_id = $1 AND year = $2 AND month = $3
    `
    database.Pool.QueryRow(c.Request.Context(), planQuery, userID, fromDate.Year(), int(fromDate.Month())).Scan(&planIncome, &planExpense)
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "actual": gin.H{
            "income":  actualIncome,
            "expense": actualExpense,
            "profit":  actualIncome - actualExpense,
        },
        "plan": gin.H{
            "income":  planIncome,
            "expense": planExpense,
            "profit":  planIncome - planExpense,
        },
        "period": gin.H{
            "from": fromDate,
            "to":   toDate,
        },
    })
}

// CreateTemplatePosting - создание шаблона проводки
func CreateTemplatePosting(c *gin.Context) {
    userID := getCurrentUserID(c)
    
    var req struct {
        Name          string  `json:"name" binding:"required"`
        DebitAccount  string  `json:"debit_account" binding:"required"`
        CreditAccount string  `json:"credit_account" binding:"required"`
        Amount        float64 `json:"amount"`
        Description   string  `json:"description"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO posting_templates (id, user_id, name, debit_account, credit_account, amount, description, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
    `, uuid.New(), userID, req.Name, req.DebitAccount, req.CreditAccount, req.Amount, req.Description)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Шаблон создан"})
}

// GetPostingTemplates - получение шаблонов проводок
func GetPostingTemplates(c *gin.Context) {
    userID := getCurrentUserID(c)
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, debit_account, credit_account, amount, description, created_at
        FROM posting_templates
        WHERE user_id = $1
        ORDER BY created_at DESC
    `, userID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var templates []gin.H
    for rows.Next() {
        var id, name, debitAccount, creditAccount, description string
        var amount float64
        var createdAt time.Time
        
        rows.Scan(&id, &name, &debitAccount, &creditAccount, &amount, &description, &createdAt)
        templates = append(templates, gin.H{
            "id":             id,
            "name":           name,
            "debit_account":  debitAccount,
            "credit_account": creditAccount,
            "amount":         amount,
            "description":    description,
            "created_at":     createdAt,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "templates": templates})
}

// CloseMonth - закрытие месяца
func CloseMonth(c *gin.Context) {
    userID := getCurrentUserID(c)
    
    var req struct {
        Month int `json:"month" binding:"required"`
        Year  int `json:"year" binding:"required"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Проверяем, не закрыт ли уже месяц
    var count int
    checkQuery := `SELECT COUNT(*) FROM month_closing WHERE user_id = $1 AND year = $2 AND month = $3`
    database.Pool.QueryRow(c.Request.Context(), checkQuery, userID, req.Year, req.Month).Scan(&count)
    
    if count > 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Месяц уже закрыт"})
        return
    }
    
    // Создаём запись о закрытии
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO month_closing (id, user_id, year, month, closed_at, status)
        VALUES ($1, $2, $3, $4, NOW(), 'closed')
    `, uuid.New(), userID, req.Year, req.Month)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": fmt.Sprintf("Месяц %d/%d успешно закрыт", req.Month, req.Year),
    })
}

// GetMonthClosingStatus - статус закрытия месяцев
func GetMonthClosingStatus(c *gin.Context) {
    userID := getCurrentUserID(c)
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT year, month, closed_at, status
        FROM month_closing
        WHERE user_id = $1
        ORDER BY year DESC, month DESC
        LIMIT 12
    `, userID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var closings []gin.H
    for rows.Next() {
        var year, month int
        var closedAt time.Time
        var status string
        rows.Scan(&year, &month, &closedAt, &status)
        closings = append(closings, gin.H{
            "year":      year,
            "month":     month,
            "closed_at": closedAt,
            "status":    status,
        })
    }
    
    c.JSON(http.StatusOK, closings)
}