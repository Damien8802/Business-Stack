package handlers

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "strings"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    
    "subscription-system/database"
)
// Получить проекты
func GetProjects(c *gin.Context) {
    userID := getUserID(c)
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, description, status, start_date, end_date, created_at
        FROM projects
        WHERE user_id = $1 AND status != 'archived'
        ORDER BY created_at DESC
    `, userID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
        return
    }
    defer rows.Close()
    
    var projects []map[string]interface{}
    for rows.Next() {
        var id uuid.UUID
        var name, description, status string
        var startDate, endDate, createdAt time.Time
        
        rows.Scan(&id, &name, &description, &status, &startDate, &endDate, &createdAt)
        
        // Считаем задачи в проекте
        var taskCount int
        database.Pool.QueryRow(c.Request.Context(), `
            SELECT COUNT(*) FROM tasks WHERE project_id = $1
        `, id).Scan(&taskCount)
        
        var completedCount int
        database.Pool.QueryRow(c.Request.Context(), `
            SELECT COUNT(*) FROM tasks WHERE project_id = $1 AND status = 'completed'
        `, id).Scan(&completedCount)
        
        var progress float64
        if taskCount > 0 {
            progress = float64(completedCount) / float64(taskCount) * 100
        }
        
        projects = append(projects, map[string]interface{}{
            "id":              id,
            "name":            name,
            "description":     description,
            "status":          status,
            "start_date":      startDate,
            "end_date":        endDate,
            "created_at":      createdAt,
            "task_count":      taskCount,
            "completed_count": completedCount,
            "progress":        progress,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// Создать проект
func CreateProject(c *gin.Context) {
    userID := getUserID(c)
    
    var req struct {
        Name        string `json:"name" binding:"required"`
        Description string `json:"description"`
        StartDate   string `json:"start_date"`
        EndDate     string `json:"end_date"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    var projectID uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO projects (user_id, name, description, start_date, end_date)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id
    `, userID, req.Name, req.Description, req.StartDate, req.EndDate).Scan(&projectID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "project_id": projectID,
        "message":    "Проект создан",
    })
}

// Получить задачи
func GetTasks(c *gin.Context) {
    userID := getUserID(c)
    projectID := c.Query("project_id")
    
    query := `
        SELECT t.id, t.title, t.description, t.priority, t.status, t.due_date, t.created_at,
               u.name as assigned_name, c.name as created_name
        FROM tasks t
        LEFT JOIN users u ON t.assigned_to = u.id
        LEFT JOIN users c ON t.created_by = c.id
        WHERE t.created_by = $1
    `
    args := []interface{}{userID}
    argIndex := 2
    
    if projectID != "" {
        query += fmt.Sprintf(" AND t.project_id = $%d", argIndex)
        args = append(args, projectID)
        argIndex++
    }
    
    query += " ORDER BY t.created_at DESC"
    
    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
        return
    }
    defer rows.Close()
    
    var tasks []map[string]interface{}
    for rows.Next() {
        var id uuid.UUID
        var title, description, priority, status, assignedName, createdName string
        var dueDate, createdAt time.Time
        
        rows.Scan(&id, &title, &description, &priority, &status, &dueDate, &createdAt, &assignedName, &createdName)
        
        tasks = append(tasks, map[string]interface{}{
            "id":          id,
            "title":       title,
            "description": description,
            "priority":    priority,
            "status":      status,
            "due_date":    dueDate,
            "created_at":  createdAt,
            "assigned_to": assignedName,
            "created_by":  createdName,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// Создать задачу
func CreateTask(c *gin.Context) {
    userID := getUserID(c)
    
    var req struct {
        ProjectID   string `json:"project_id"`
        Title       string `json:"title" binding:"required"`
        Description string `json:"description"`
        Priority    string `json:"priority"`
        AssignedTo  string `json:"assigned_to"`
        DueDate     string `json:"due_date"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    if req.Priority == "" {
        req.Priority = "medium"
    }
    
    var taskID uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO tasks (project_id, created_by, assigned_to, title, description, priority, due_date)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id
    `, req.ProjectID, userID, req.AssignedTo, req.Title, req.Description, req.Priority, req.DueDate).Scan(&taskID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
        return
    }
    
    // Отправляем уведомление (если есть кому)
    if req.AssignedTo != "" {
        SendNotificationToUser(req.AssignedTo, "Новая задача", "Вам назначена задача: "+req.Title, "/tasks/"+taskID.String())
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "task_id": taskID,
        "message": "Задача создана",
    })
}

// Обновить задачу
func UpdateTask(c *gin.Context) {
    userID := getUserID(c)
    taskID := c.Param("id")
    
    var req struct {
        Status     string `json:"status"`
        Priority   string `json:"priority"`
        AssignedTo string `json:"assigned_to"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    var updateFields []string
    var args []interface{}
    argIndex := 1
    
    if req.Status != "" {
        updateFields = append(updateFields, fmt.Sprintf("status = $%d", argIndex))
        args = append(args, req.Status)
        argIndex++
        
        if req.Status == "completed" {
            updateFields = append(updateFields, "completed_at = NOW()")
        }
    }
    
    if req.Priority != "" {
        updateFields = append(updateFields, fmt.Sprintf("priority = $%d", argIndex))
        args = append(args, req.Priority)
        argIndex++
    }
    
    if req.AssignedTo != "" {
        updateFields = append(updateFields, fmt.Sprintf("assigned_to = $%d", argIndex))
        args = append(args, req.AssignedTo)
        argIndex++
    }
    
    if len(updateFields) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
        return
    }
    
    args = append(args, taskID, userID)
    query := fmt.Sprintf(`
        UPDATE tasks SET %s WHERE id = $%d AND created_by = $%d
    `, strings.Join(updateFields, ", "), argIndex, argIndex+1)
    
    _, err := database.Pool.Exec(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Задача обновлена"})
}

// Отправить уведомление пользователю
func SendNotificationToUser(userIDStr string, title, message, link string) {
    userID, err := uuid.Parse(userIDStr)
    if err != nil {
        return
    }
    
    _, err = database.Pool.Exec(context.Background(), `
        INSERT INTO notifications (user_id, type, title, message, link, created_at)
        VALUES ($1, $2, $3, $4, $5, NOW())
    `, userID, "task", title, message, link)
    
    if err != nil {
        log.Printf("Failed to send notification: %v", err)
    }
}

// Страница проектов
func ProjectsPageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "projects.html", gin.H{
        "title": "Проекты и задачи | Business Stack",
    })
}

// Получить данные для гант-диаграммы
func GetGanttData(c *gin.Context) {
    userID := getUserID(c)
    projectID := c.Query("project_id")
    
    query := `
        SELECT 
            t.id,
            t.title,
            t.start_date,
            t.due_date,
            t.status,
            t.priority,
            t.progress
        FROM tasks t
        WHERE t.created_by = $1
    `
    args := []interface{}{userID}
    
    if projectID != "" {
        query += " AND t.project_id = $2"
        args = append(args, projectID)
    }
    
    query += " ORDER BY t.start_date"
    
    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
        return
    }
    defer rows.Close()
    
    var tasks []map[string]interface{}
    for rows.Next() {
        var id uuid.UUID
        var title, status, priority string
        var startDate, dueDate time.Time
        var progress int
        
        rows.Scan(&id, &title, &startDate, &dueDate, &status, &priority, &progress)
        
        tasks = append(tasks, map[string]interface{}{
            "id":         id,
            "name":       title,
            "start_date": startDate.Format("2006-01-02"),
            "end_date":   dueDate.Format("2006-01-02"),
            "status":     status,
            "priority":   priority,
            "progress":   progress,
        })
    }
    
    c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}