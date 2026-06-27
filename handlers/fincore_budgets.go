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
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
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
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
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
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    tagID := c.Query("tag_id")
    year := c.DefaultQuery("year", strconv.Itoa(time.Now().Year()))
    
    // Получаем план и факт по месяцам
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            b.month,
            b.planned_amount as planned,
            COALESCE(SUM(j.debit_amount - j.credit_amount), 0) as actual
        FROM fincore_budgets b
        LEFT JOIN journal_entries j ON (
            j.tenant_id = b.tenant_id 
            AND EXTRACT(MONTH FROM j.operation_date) = b.month 
            AND EXTRACT(YEAR FROM j.operation_date) = b.year
        )
        LEFT JOIN journal_entry_tags jet ON jet.entry_id = j.id
        WHERE b.tenant_id = $1 
            AND b.year = $2 
            AND b.tag_id = $3
        GROUP BY b.month, b.planned_amount
        ORDER BY b.month
    `, tenantID, year, tagID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var months []gin.H
    var totalPlanned, totalActual float64
    monthNames := []string{"Янв", "Фев", "Мар", "Апр", "Май", "Июн", "Июл", "Авг", "Сен", "Окт", "Ноя", "Дек"}
    
    for rows.Next() {
        var month int
        var planned, actual float64
        rows.Scan(&month, &planned, &actual)
        totalPlanned += planned
        totalActual += actual
        
        months = append(months, gin.H{
            "month":     monthNames[month-1],
            "month_num": month,
            "planned":   planned,
            "actual":    actual,
            "variance":  actual - planned,
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
        "months":  months,
        "total": gin.H{
            "planned":  totalPlanned,
            "actual":   totalActual,
            "variance": totalActual - totalPlanned,
            "percent": func() float64 {
                if totalPlanned == 0 {
                    return 0
                }
                return (totalActual / totalPlanned) * 100
            }(),
        },
    })
}

// CreateTemplatePosting - создание шаблона проводки
func CreateTemplatePosting(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }
    
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
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }
    
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
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }
    
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
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }
    
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }
    
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

// ========== АВТОМАТИЧЕСКОЕ СОЗДАНИЕ БЮДЖЕТОВ ==========

// AutoCreateBudgets - автоматически создаёт бюджеты для тега
func AutoCreateBudgets(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }

    tagID := c.Query("tag_id")
    if tagID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tag_id required"})
        return
    }

    year := time.Now().Year()
    if y := c.Query("year"); y != "" {
        if val, err := strconv.Atoi(y); err == nil {
            year = val
        }
    }

    // Проверяем, есть ли уже бюджеты
    var count int
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM fincore_budgets WHERE tag_id = $1 AND year = $2
    `, tagID, year).Scan(&count)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    if count > 0 {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "message": "Бюджеты уже существуют",
            "count":   count,
        })
        return
    }

    // Создаём бюджеты на 12 месяцев
    var inserted int
    for month := 1; month <= 12; month++ {
        // Получаем фактическую сумму за прошлый год для этого месяца
        var actual float64
        database.Pool.QueryRow(c.Request.Context(), `
            SELECT COALESCE(SUM(debit_amount - credit_amount), 0)
            FROM journal_entries
            WHERE tenant_id = $1 
                AND EXTRACT(MONTH FROM operation_date) = $2
                AND EXTRACT(YEAR FROM operation_date) = $3
        `, tenantID, month, year-1).Scan(&actual)

        // План = факт прошлого года * 1.1 (рост 10%)
        planned := actual * 1.1
        if planned == 0 {
            planned = 10000 // Минимальный план, если нет данных
        }

        _, err := database.Pool.Exec(c.Request.Context(), `
            INSERT INTO fincore_budgets (tenant_id, tag_id, year, month, planned_amount, created_at)
            VALUES ($1, $2, $3, $4, $5, NOW())
            ON CONFLICT (tenant_id, tag_id, year, month) DO UPDATE SET
                planned_amount = EXCLUDED.planned_amount
        `, tenantID, tagID, year, month, planned)

        if err == nil {
            inserted++
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "success":   true,
        "message":   fmt.Sprintf("Создано %d бюджетов на %d год", inserted, year),
        "count":     inserted,
        "year":      year,
    })
}

// UpdateBudgetsFromEntries - обновляет фактические данные из проводок
func UpdateBudgetsFromEntries(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: tenant not found"})
        return
    }

    tagID := c.Query("tag_id")
    if tagID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tag_id required"})
        return
    }

    year := time.Now().Year()
    if y := c.Query("year"); y != "" {
        if val, err := strconv.Atoi(y); err == nil {
            year = val
        }
    }

    // Обновляем фактические суммы для каждого месяца
    var updated int
    for month := 1; month <= 12; month++ {
        var actual float64
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT COALESCE(SUM(j.debit_amount - j.credit_amount), 0)
            FROM journal_entries j
            JOIN journal_entry_tags jet ON jet.entry_id = j.id
            WHERE j.tenant_id = $1 
                AND jet.tag_id = $2
                AND EXTRACT(MONTH FROM j.operation_date) = $3
                AND EXTRACT(YEAR FROM j.operation_date) = $4
        `, tenantID, tagID, month, year).Scan(&actual)

        if err != nil {
            continue
        }

        result, err := database.Pool.Exec(c.Request.Context(), `
            UPDATE fincore_budgets 
            SET actual_amount = $1, updated_at = NOW()
            WHERE tag_id = $2 AND year = $3 AND month = $4
        `, actual, tagID, year, month)

        if err == nil && result.RowsAffected() > 0 {
            updated++
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": fmt.Sprintf("Обновлено %d месяцев", updated),
        "updated": updated,
    })
}