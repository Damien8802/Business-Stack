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

// StartModuleTrialHandler - активация пробного периода (переименовано из StartModuleTrial чтобы избежать конфликта с oauth.go)
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
    var err error
    err = database.Pool.QueryRow(c.Request.Context(), `
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