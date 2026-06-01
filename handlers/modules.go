package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

// GetModules - получить список всех модулей
func GetModules(c *gin.Context) {
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT code, name, description, price, trial_days, icon, sort_order
        FROM modules 
        WHERE is_active = true 
        ORDER BY sort_order
    `)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var modules []gin.H
    for rows.Next() {
        var code, name, description, icon string
        var price float64
        var trialDays, sortOrder int
        
        rows.Scan(&code, &name, &description, &price, &trialDays, &icon, &sortOrder)
        
        modules = append(modules, gin.H{
            "code":        code,
            "name":        name,
            "description": description,
            "price":       price,
            "trial_days":  trialDays,
            "icon":        icon,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "modules": modules,
    })
}

// GetMyModuleSubscriptions - получить подписки текущего пользователя
func GetMyModuleSubscriptions(c *gin.Context) {
    userID := c.GetString("user_id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT m.code, m.name, m.price, u.status, u.start_date, u.end_date, u.trial_used
        FROM user_module_subscriptions u
        JOIN modules m ON u.module_code = m.code
        WHERE u.user_id = $1
        ORDER BY u.created_at DESC
    `, userID)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"subscriptions": []gin.H{}})
        return
    }
    defer rows.Close()
    
    var subscriptions []gin.H
    for rows.Next() {
        var code, name, status string
        var price float64
        var startDate, endDate time.Time
        var trialUsed bool
        
        rows.Scan(&code, &name, &price, &status, &startDate, &endDate, &trialUsed)
        
        subscriptions = append(subscriptions, gin.H{
            "code":        code,
            "name":        name,
            "price":       price,
            "status":      status,
            "start_date":  startDate,
            "end_date":    endDate,
            "trial_used":  trialUsed,
            "is_active":   status == "active" || (status == "trial" && endDate.After(time.Now())),
        })
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "subscriptions": subscriptions,
    })
}

// StartModuleTrialHandler - активация пробного периода
func StartModuleTrialHandler(c *gin.Context) {
    userID := c.GetString("user_id")
    
    var req struct {
        ModuleCode string `json:"module_code" binding:"required"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Проверяем, есть ли уже подписка
    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM user_module_subscriptions 
        WHERE user_id = $1 AND module_code = $2)
    `, userID, req.ModuleCode).Scan(&exists)
    
    if err != nil {
        // Обработка ошибки
    }
    
    if exists {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Пробный период уже активирован или подписка оформлена"})
        return
    }
    
    // Получаем количество дней триала
    var trialDays int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT trial_days FROM modules WHERE code = $1
    `, req.ModuleCode).Scan(&trialDays)
    
    if trialDays == 0 {
        trialDays = 14
    }
    
    // Создаем подписку с триалом
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO user_module_subscriptions (user_id, module_code, status, end_date, trial_used)
        VALUES ($1, $2, 'trial', NOW() + ($3 || ' days')::interval, true)
    `, userID, req.ModuleCode, trialDays)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка активации пробного периода"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":     true,
        "message":     "Пробный период активирован!",
        "trial_days":  trialDays,
    })
}

// CheckModuleAccess - проверить доступ к модулю
func CheckModuleAccess(c *gin.Context) {
    userID := c.GetString("user_id")
    moduleCode := c.Param("module")
    
    var status string
    var endDate time.Time
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT status, end_date 
        FROM user_module_subscriptions 
        WHERE user_id = $1 AND module_code = $2
    `, userID, moduleCode).Scan(&status, &endDate)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "has_access": false,
            "message":    "Нет доступа к модулю",
        })
        return
    }
    
    hasAccess := status == "active" || (status == "trial" && endDate.After(time.Now()))
    
    c.JSON(http.StatusOK, gin.H{
        "has_access":  hasAccess,
        "status":      status,
        "end_date":    endDate,
        "message":     "Доступ разрешен",
    })
}

// ========== DEV MODULES FUNCTIONS ==========

// GetModuleStatus - получить статус модуля (для страницы)
func GetModuleStatus(c *gin.Context) {
    route := c.Param("route")
    
    var status string
    var message string
    
    _, _ = database.Pool.Exec(c.Request.Context(), `
        ALTER TABLE dev_modules ADD COLUMN IF NOT EXISTS status VARCHAR(50) DEFAULT 'development';
        ALTER TABLE dev_modules ADD COLUMN IF NOT EXISTS message TEXT;
    `)
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(status, 'development'), COALESCE(message, '')
        FROM dev_modules 
        WHERE route = $1
    `, "/"+route).Scan(&status, &message)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{"status": "development", "message": ""})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"status": status, "message": message})
}

// UpdateModuleStatus - обновить статус модуля
func UpdateModuleStatus(c *gin.Context) {
    route := c.Param("route")
    
    var req struct {
        Status  string `json:"status"`
        Message string `json:"message"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    validStatuses := map[string]bool{
        "development": true, "beta": true, "stable": true, "deprecated": true,
    }
    
    if !validStatuses[req.Status] {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO dev_modules (route, name, icon, status, message, updated_at)
        VALUES ($1, $2, '🔧', $3, $4, NOW())
        ON CONFLICT (route) DO UPDATE SET
            status = EXCLUDED.status,
            message = EXCLUDED.message,
            updated_at = NOW()
    `, "/"+route, route, req.Status, req.Message)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// ReportModuleIssue - сообщить о проблеме
func ReportModuleIssue(c *gin.Context) {
    userID := c.GetString("user_id")
    userEmail := c.GetString("user_email")
    userName := c.GetString("user_name")
    
    var req struct {
        Module    string `json:"module"`
        Issue     string `json:"issue"`
        Status    string `json:"status"`
        URL       string `json:"url"`
        UserAgent string `json:"userAgent"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    _, _ = database.Pool.Exec(c.Request.Context(), `
        CREATE TABLE IF NOT EXISTS module_feedback (
            id SERIAL PRIMARY KEY,
            user_id UUID, user_email VARCHAR(255), user_name VARCHAR(255),
            module VARCHAR(255) NOT NULL, issue TEXT NOT NULL, status VARCHAR(50),
            url TEXT, user_agent TEXT, created_at TIMESTAMP DEFAULT NOW(),
            resolved BOOLEAN DEFAULT FALSE
        )
    `)
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO module_feedback (user_id, user_email, user_name, module, issue, status, url, user_agent)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
    `, userID, userEmail, userName, req.Module, req.Issue, req.Status, req.URL, req.UserAgent)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetModuleFeedback - получить отзывы (админ)
func GetModuleFeedback(c *gin.Context) {
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, user_email, user_name, module, issue, status, created_at, resolved
        FROM module_feedback ORDER BY created_at DESC LIMIT 100
    `)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var feedbacks []gin.H
    for rows.Next() {
        var id int
        var userEmail, userName, module, issue, status string
        var createdAt time.Time
        var resolved bool
        rows.Scan(&id, &userEmail, &userName, &module, &issue, &status, &createdAt, &resolved)
        feedbacks = append(feedbacks, gin.H{
            "id": id, "user_email": userEmail, "user_name": userName,
            "module": module, "issue": issue, "status": status,
            "created_at": createdAt, "resolved": resolved,
        })
    }
    
    c.JSON(http.StatusOK, feedbacks)
}