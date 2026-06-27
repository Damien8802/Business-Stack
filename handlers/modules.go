package handlers

import (
    "context" 
    "fmt"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
    "subscription-system/services"
)

// ============================================================
// 1. ОСНОВНЫЕ ФУНКЦИИ МОДУЛЕЙ (РАБОТА С ПОДПИСКАМИ)
// ============================================================

// GetModules - получить список всех доступных модулей
// GET /api/modules
// Возвращает: список модулей с ценой, иконкой, описанием
// ✅ СТАРАЯ ФУНКЦИЯ (была)
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
// GET /api/modules/my-subscriptions
// Возвращает: список подписок пользователя с датами и статусами
// ✅ СТАРАЯ ФУНКЦИЯ (была)
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
// POST /api/modules/start-trial
// Тело: {"module_code": "module_name"}
// ✅ СТАРАЯ ФУНКЦИЯ (была)
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
// GET /api/modules/check/:module
// ✅ СТАРАЯ ФУНКЦИЯ (была)
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

// ============================================================
// 2. ФУНКЦИИ ДЛЯ РАЗРАБОТКИ МОДУЛЕЙ (DEV MODULES)
// ============================================================

// GetModuleStatus - получить статус модуля (для страницы)
// GET /api/dev-modules/status/:route
// ✅ СТАРАЯ ФУНКЦИЯ (была)
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

// UpdateModuleStatus - обновить статус модуля (админ)
// PUT /api/dev-modules/status/:route
// ✅ СТАРАЯ ФУНКЦИЯ (была)
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

// ReportModuleIssue - сообщить о проблеме (обратная связь)
// POST /api/dev-modules/feedback
// ✅ СТАРАЯ ФУНКЦИЯ (была)
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
    
    // ✅ ОТПРАВКА УВЕДОМЛЕНИЯ В TELEGRAM
    services.NotifyAdminModuleRequest(req.Module, userName, userEmail, req.Issue)
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetModuleFeedback - получить отзывы (админ)
// GET /api/dev-modules/feedback
// ✅ СТАРАЯ ФУНКЦИЯ (была)
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

// GetMyModuleRequests - получить заявки текущего пользователя на модули
// GET /api/dev-modules/my-requests
// ✅ СТАРАЯ ФУНКЦИЯ (была)
func GetMyModuleRequests(c *gin.Context) {
    userID := c.GetString("user_id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, module, issue, status, created_at
        FROM module_feedback
        WHERE user_id = $1
        ORDER BY created_at DESC
    `, userID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var requests []gin.H
    for rows.Next() {
        var id int
        var module, issue, status string
        var createdAt time.Time
        err := rows.Scan(&id, &module, &issue, &status, &createdAt)
        if err != nil {
            continue
        }
        requests = append(requests, gin.H{
            "id": id,
            "module": module,
            "issue": issue,
            "status": status,
            "created_at": createdAt.Format("02.01.2006 15:04"),
        })
    }
    c.JSON(200, requests)
}

// ============================================================
// 3. 🔥 НОВЫЕ ФУНКЦИИ ДЛЯ УПРАВЛЕНИЯ ЗАЯВКАМИ (ДОБАВЛЕНЫ)
// ============================================================

// UpdateModuleRequestStatus - обновить статус заявки (админ)
// PUT /api/dev-modules/feedback/:id/status
// Тело: {"status": "in_progress", "comment": "Начали работу"}
// 🔥 НОВАЯ ФУНКЦИЯ
func UpdateModuleRequestStatus(c *gin.Context) {
    requestID := c.Param("id")
    
    var req struct {
        Status   string `json:"status"`
        Comment  string `json:"comment"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Валидация статуса
    validStatuses := map[string]bool{
        "new": true, "in_progress": true, "review": true, 
        "completed": true, "rejected": true,
    }
    
    if !validStatuses[req.Status] {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
        return
    }
    
    // Получаем старый статус для уведомления
    var oldStatus string
    var userID string
    var moduleName string
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT status, user_id::text, module FROM module_feedback WHERE id = $1
    `, requestID).Scan(&oldStatus, &userID, &moduleName)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Заявка не найдена"})
        return
    }
    
    // Обновляем статус
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE module_feedback 
        SET status = $1, updated_at = NOW()
        WHERE id = $2
    `, req.Status, requestID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    // Добавляем комментарий если есть
    if req.Comment != "" {
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO request_comments (request_id, user_id, comment, created_at)
            VALUES ($1, 'system', $2, NOW())
        `, requestID, req.Comment)
        if err != nil {
            fmt.Printf("Error adding comment: %v\n", err)
        }
    }
    
    // Отправляем уведомление пользователю
    go sendStatusUpdateNotification(userID, moduleName, req.Status, oldStatus, req.Comment)
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Статус заявки обновлен",
    })
}

// sendStatusUpdateNotification - отправка уведомления об изменении статуса
// 🔥 НОВАЯ ФУНКЦИЯ (внутренняя)
func sendStatusUpdateNotification(userID, moduleName, newStatus, oldStatus, comment string) {
    // Получаем email пользователя
    var email, name string
    err := database.Pool.QueryRow(context.Background(), `
        SELECT email, name FROM users WHERE id = $1
    `, userID).Scan(&email, &name)
    
    if err != nil {
        fmt.Printf("User not found: %v\n", err)
        return
    }
    
    statusMap := map[string]string{
        "new":         "Новая",
        "in_progress": "В работе",
        "review":      "На проверке",
        "completed":   "Выполнена",
        "rejected":    "Отклонена",
    }
    
    // subject используется для email
    _ = fmt.Sprintf("Изменение статуса заявки: %s", moduleName)
    body := fmt.Sprintf(`
        Здравствуйте, %s!
        
        Статус вашей заявки на модуль "%s" изменен с "%s" на "%s".
    `, name, moduleName, statusMap[oldStatus], statusMap[newStatus])
    
    if comment != "" {
        body += fmt.Sprintf("\n\nКомментарий администратора: %s", comment)
    }
    
    // Отправляем email (раскомментируйте когда настроите SMTP)
    // services.SendEmail([]string{email}, subject, body)
    
    // Сохраняем уведомление в БД
    _, err = database.Pool.Exec(context.Background(), `
        INSERT INTO notifications (user_id, message, type, created_at)
        VALUES ($1, $2, 'status_update', NOW())
    `, userID, body)
    
    if err != nil {
        fmt.Printf("Error saving notification: %v\n", err)
    }
}

// GetModuleStatistics - статистика использования модулей (админ)
// GET /api/dev-modules/stats
// 🔥 НОВАЯ ФУНКЦИЯ
func GetModuleStatistics(c *gin.Context) {
    stats := gin.H{
        "total_modules": 0,
        "active_users":  0,
        "trial_users":   0,
        "popular_modules": []gin.H{},
    }
    
    // Количество модулей
    var totalModules int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM modules WHERE is_active = true
    `).Scan(&totalModules)
    stats["total_modules"] = totalModules
    
    // Активные пользователи
    var activeUsers int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(DISTINCT user_id) FROM user_module_subscriptions 
        WHERE status = 'active'
    `).Scan(&activeUsers)
    stats["active_users"] = activeUsers
    
    // Пользователи на триале
    var trialUsers int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(DISTINCT user_id) FROM user_module_subscriptions 
        WHERE status = 'trial' AND end_date > NOW()
    `).Scan(&trialUsers)
    stats["trial_users"] = trialUsers
    
    // Популярные модули
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT m.code, m.name, COUNT(u.user_id) as users_count
        FROM modules m
        LEFT JOIN user_module_subscriptions u ON m.code = u.module_code
        WHERE m.is_active = true
        GROUP BY m.code, m.name
        ORDER BY users_count DESC
        LIMIT 5
    `)
    
    if err == nil {
        defer rows.Close()
        var popular []gin.H
        for rows.Next() {
            var code, name string
            var count int
            rows.Scan(&code, &name, &count)
            popular = append(popular, gin.H{
                "code":  code,
                "name":  name,
                "users": count,
            })
        }
        stats["popular_modules"] = popular
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "stats":   stats,
    })
}

// GetModuleFeedbackByID - получить детали заявки по ID (админ)
// GET /api/dev-modules/feedback/:id
// 🔥 НОВАЯ ФУНКЦИЯ
func GetModuleFeedbackByID(c *gin.Context) {
    id := c.Param("id")
    
    var feedback struct {
        ID        int       `json:"id"`
        UserID    string    `json:"user_id"`
        UserEmail string    `json:"user_email"`
        UserName  string    `json:"user_name"`
        Module    string    `json:"module"`
        Issue     string    `json:"issue"`
        Status    string    `json:"status"`
        URL       string    `json:"url"`
        UserAgent string    `json:"user_agent"`
        Resolved  bool      `json:"resolved"`
        CreatedAt time.Time `json:"created_at"`
        UpdatedAt time.Time `json:"updated_at"`
    }
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, COALESCE(user_id::text, ''), COALESCE(user_email, ''), 
               COALESCE(user_name, ''), module, issue, COALESCE(status, 'new'),
               COALESCE(url, ''), COALESCE(user_agent, ''), resolved,
               created_at, COALESCE(updated_at, created_at)
        FROM module_feedback WHERE id = $1
    `, id).Scan(
        &feedback.ID, &feedback.UserID, &feedback.UserEmail, &feedback.UserName,
        &feedback.Module, &feedback.Issue, &feedback.Status, &feedback.URL,
        &feedback.UserAgent, &feedback.Resolved, &feedback.CreatedAt, &feedback.UpdatedAt,
    )
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Заявка не найдена"})
        return
    }
    
    // Получаем комментарии
    comments, _ := getRequestComments(c.Request.Context(), feedback.ID)
    
    c.JSON(http.StatusOK, gin.H{
        "success":  true,
        "feedback": feedback,
        "comments": comments,
    })
}

// DeleteModuleFeedback - удалить заявку (админ)
// DELETE /api/dev-modules/feedback/:id
// 🔥 НОВАЯ ФУНКЦИЯ
func DeleteModuleFeedback(c *gin.Context) {
    id := c.Param("id")
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM module_feedback WHERE id = $1
    `, id)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// ============================================================
// 4. 🔥 НОВЫЕ ФУНКЦИИ ДЛЯ КОММЕНТАРИЕВ (ДОБАВЛЕНЫ)
// ============================================================

// getRequestComments - получить комментарии к заявке (вспомогательная)
// 🔥 НОВАЯ ФУНКЦИЯ (внутренняя)
func getRequestComments(ctx context.Context, requestID int) ([]gin.H, error) {
    rows, err := database.Pool.Query(ctx, `
        SELECT user_id, comment, created_at
        FROM request_comments
        WHERE request_id = $1
        ORDER BY created_at ASC
    `, requestID)
    
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var comments []gin.H
    for rows.Next() {
        var userID, comment string
        var createdAt time.Time
        rows.Scan(&userID, &comment, &createdAt)
        
        author := "Пользователь"
        if userID == "system" || userID == "admin" {
            author = "Администратор"
        }
        
        comments = append(comments, gin.H{
            "author":     author,
            "comment":    comment,
            "created_at": createdAt.Format("02.01.2006 15:04"),
        })
    }
    return comments, nil
}

// GetRequestComments - получить комментарии к заявке (API)
// GET /api/dev-modules/requests/:id/comments
// 🔥 НОВАЯ ФУНКЦИЯ
func GetRequestComments(c *gin.Context) {
    requestID := c.Param("id")
    
    var id int
    fmt.Sscanf(requestID, "%d", &id)
    
    comments, err := getRequestComments(c.Request.Context(), id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":  true,
        "comments": comments,
    })
}

// AddRequestComment - добавить комментарий к заявке
// POST /api/dev-modules/requests/:id/comments
// Тело: {"comment": "Текст комментария"}
// 🔥 НОВАЯ ФУНКЦИЯ
func AddRequestComment(c *gin.Context) {
    requestID := c.Param("id")
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    
    var req struct {
        Comment string `json:"comment" binding:"required"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    var id int
    fmt.Sscanf(requestID, "%d", &id)
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO request_comments (request_id, user_id, comment, created_at)
        VALUES ($1, $2, $3, NOW())
    `, id, userID, req.Comment)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    // Отправляем уведомление админу (если комментарий от пользователя)
    // или пользователю (если комментарий от админа)
    role := c.GetString("role")
    if role == "admin" || role == "developer" {
        // Уведомляем пользователя
        go notifyUserAboutComment(id, userID, userName, req.Comment)
    } else {
        // Уведомляем админа
        services.NotifyAdminModuleRequest("Комментарий к заявке #"+requestID, userName, c.GetString("user_email"), req.Comment)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Комментарий добавлен",
    })
}

// notifyUserAboutComment - уведомить пользователя о комментарии
// 🔥 НОВАЯ ФУНКЦИЯ (внутренняя)
func notifyUserAboutComment(requestID int, adminID, adminName, comment string) {
    // Получаем пользователя, которому принадлежит заявка
    var userID string
    var moduleName string
    
    err := database.Pool.QueryRow(context.Background(), `
        SELECT user_id::text, module FROM module_feedback WHERE id = $1
    `, requestID).Scan(&userID, &moduleName)
    
    if err != nil {
        return
    }
    
    // Получаем email пользователя
    var email, name string
    err = database.Pool.QueryRow(context.Background(), `
        SELECT email, name FROM users WHERE id = $1
    `, userID).Scan(&email, &name)
    
    if err != nil {
        return
    }
    
    body := fmt.Sprintf(`
        Здравствуйте, %s!
        
        Администратор %s оставил комментарий к вашей заявке на модуль "%s":
        
        "%s"
        
        Вы можете посмотреть комментарий в личном кабинете.
    `, name, adminName, moduleName, comment)
    
    // Сохраняем уведомление в БД
    _, _ = database.Pool.Exec(context.Background(), `
        INSERT INTO notifications (user_id, message, type, created_at)
        VALUES ($1, $2, 'comment', NOW())
    `, userID, body)
}