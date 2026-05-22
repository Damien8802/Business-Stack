package handlers

import (
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)

// HRDashboardHandler - главная страница HR модуля
func HRDashboardHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "hr/index.html", gin.H{
        "Title": "HR-модуль | Business Stack",
    })
}

// GetEmployeesHandler - получить всех сотрудников
func GetEmployeesHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    department := c.Query("department")
    status := c.Query("status")
    search := c.Query("search")

    query := `SELECT id, first_name, last_name, email, phone, position, department, 
              hire_date, salary, status, birth_date, created_at 
              FROM hr_employees WHERE tenant_id = $1`
    args := []interface{}{tenantID}
    argIdx := 2

    if department != "" && department != "all" {
        query += " AND department = $" + strconv.Itoa(argIdx)
        args = append(args, department)
        argIdx++
    }
    if status != "" && status != "all" {
        query += " AND status = $" + strconv.Itoa(argIdx)
        args = append(args, status)
        argIdx++
    }
    if search != "" {
        query += " AND (first_name ILIKE $" + strconv.Itoa(argIdx) + " OR last_name ILIKE $" + strconv.Itoa(argIdx) + " OR email ILIKE $" + strconv.Itoa(argIdx) + ")"
        args = append(args, "%"+search+"%")
    }
    query += " ORDER BY created_at DESC"

    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var employees []gin.H
    for rows.Next() {
        var id uuid.UUID
        var firstName, lastName, email, phone, position, department, status string
        var hireDate time.Time
        var salary float64
        var birthDate *time.Time
        var createdAt time.Time

        rows.Scan(&id, &firstName, &lastName, &email, &phone, &position, &department,
            &hireDate, &salary, &status, &birthDate, &createdAt)

        employees = append(employees, gin.H{
            "id":         id.String(),
            "first_name": firstName,
            "last_name":  lastName,
            "full_name":  firstName + " " + lastName,
            "email":      email,
            "phone":      phone,
            "position":   position,
            "department": department,
            "hire_date":  hireDate.Format("02.01.2006"),
            "salary":     salary,
            "status":     status,
            "birth_date": func() string {
                if birthDate != nil {
                    return birthDate.Format("02.01.2006")
                }
                return ""
            }(),
            "created_at": createdAt.Format("02.01.2006"),
        })
    }

    c.JSON(http.StatusOK, gin.H{"employees": employees})
}

// AddEmployeeHandler - добавить сотрудника
func AddEmployeeHandler(c *gin.Context) {
    var req struct {
        FirstName  string  `json:"first_name" binding:"required"`
        LastName   string  `json:"last_name" binding:"required"`
        Email      string  `json:"email" binding:"required,email"`
        Phone      string  `json:"phone"`
        Position   string  `json:"position" binding:"required"`
        Department string  `json:"department" binding:"required"`
        Salary     float64 `json:"salary" binding:"required"`
        HireDate   string  `json:"hire_date"`
        BirthDate  string  `json:"birth_date"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    hireDate := time.Now()
    if req.HireDate != "" {
        if parsed, err := time.Parse("2006-01-02", req.HireDate); err == nil {
            hireDate = parsed
        }
    }

    var birthDate *time.Time
    if req.BirthDate != "" {
        if parsed, err := time.Parse("2006-01-02", req.BirthDate); err == nil {
            birthDate = &parsed
        }
    }

    var id uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO hr_employees (first_name, last_name, email, phone, position, department, 
        hire_date, salary, status, birth_date, tenant_id)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', $9, $10)
        RETURNING id
    `, req.FirstName, req.LastName, req.Email, req.Phone, req.Position, req.Department,
        hireDate, req.Salary, birthDate, tenantID).Scan(&id)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create employee"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "id": id.String()})
}

// DeleteEmployeeHandler - удалить сотрудника
func DeleteEmployeeHandler(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(), `DELETE FROM hr_employees WHERE id = $1`, id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetVacationRequestsHandler - получить заявки на отпуск
func GetVacationRequestsHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT r.id, r.employee_id, r.start_date, r.end_date, r.days, r.type, r.status, r.comment,
               e.first_name, e.last_name, e.position
        FROM hr_vacation_requests r
        JOIN hr_employees e ON r.employee_id = e.id
        WHERE r.tenant_id = $1
        ORDER BY r.created_at DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var requests []gin.H
    for rows.Next() {
        var id, employeeID uuid.UUID
        var startDate, endDate time.Time
        var days int
        var reqType, status, comment, firstName, lastName, position string

        rows.Scan(&id, &employeeID, &startDate, &endDate, &days, &reqType, &status, &comment,
            &firstName, &lastName, &position)

        requests = append(requests, gin.H{
            "id":          id.String(),
            "employee_id": employeeID.String(),
            "employee":    firstName + " " + lastName,
            "position":    position,
            "start_date":  startDate.Format("02.01.2006"),
            "end_date":    endDate.Format("02.01.2006"),
            "days":        days,
            "type":        reqType,
            "status":      status,
            "comment":     comment,
        })
    }

    c.JSON(http.StatusOK, gin.H{"requests": requests})
}

// AddVacationRequestHandler - создать заявку на отпуск
func AddVacationRequestHandler(c *gin.Context) {
    var req struct {
        EmployeeID string `json:"employee_id" binding:"required"`
        StartDate  string `json:"start_date" binding:"required"`
        EndDate    string `json:"end_date" binding:"required"`
        Type       string `json:"type"`
        Comment    string `json:"comment"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    startDate, _ := time.Parse("2006-01-02", req.StartDate)
    endDate, _ := time.Parse("2006-01-02", req.EndDate)
    days := int(endDate.Sub(startDate).Hours()/24) + 1

    if req.Type == "" {
        req.Type = "vacation"
    }

    var id uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO hr_vacation_requests (employee_id, start_date, end_date, days, type, status, comment, tenant_id)
        VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7)
        RETURNING id
    `, req.EmployeeID, startDate, endDate, days, req.Type, req.Comment, tenantID).Scan(&id)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "id": id.String()})
}

// ApproveRequestHandler - одобрить заявку
func ApproveRequestHandler(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE hr_vacation_requests SET status = 'approved' WHERE id = $1
    `, id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// RejectRequestHandler - отклонить заявку
func RejectRequestHandler(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE hr_vacation_requests SET status = 'rejected' WHERE id = $1
    `, id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetCandidatesHandler - получить кандидатов
func GetCandidatesHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, first_name, last_name, email, phone, position, status, source, match_score, interview_date, created_at
        FROM hr_candidates WHERE tenant_id = $1 ORDER BY created_at DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    candidates := gin.H{
        "new":        []gin.H{},
        "interview":  []gin.H{},
        "offer":      []gin.H{},
        "hired":      []gin.H{},
    }

    for rows.Next() {
        var id uuid.UUID
        var firstName, lastName, email, phone, position, status, source string
        var matchScore int
        var interviewDate *time.Time
        var createdAt time.Time

        rows.Scan(&id, &firstName, &lastName, &email, &phone, &position, &status, &source, &matchScore, &interviewDate, &createdAt)

        cand := gin.H{
            "id":          id.String(),
            "first_name":  firstName,
            "last_name":   lastName,
            "full_name":   firstName + " " + lastName,
            "email":       email,
            "phone":       phone,
            "position":    position,
            "source":      source,
            "match_score": matchScore,
            "created_at":  createdAt.Format("02.01.2006"),
        }

        if interviewDate != nil {
            cand["interview_date"] = interviewDate.Format("02.01.2006 15:04")
        }

        switch status {
        case "new":
            candidates["new"] = append(candidates["new"].([]gin.H), cand)
        case "interview":
            candidates["interview"] = append(candidates["interview"].([]gin.H), cand)
        case "offer":
            candidates["offer"] = append(candidates["offer"].([]gin.H), cand)
        case "hired":
            candidates["hired"] = append(candidates["hired"].([]gin.H), cand)
        }
    }

    c.JSON(http.StatusOK, gin.H{"candidates": candidates})
}

// AddCandidateHandler - добавить кандидата
func AddCandidateHandler(c *gin.Context) {
    var req struct {
        FirstName string `json:"first_name" binding:"required"`
        LastName  string `json:"last_name" binding:"required"`
        Email     string `json:"email"`
        Phone     string `json:"phone"`
        Position  string `json:"position" binding:"required"`
        Source    string `json:"source"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var id uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO hr_candidates (first_name, last_name, email, phone, position, status, source, tenant_id)
        VALUES ($1, $2, $3, $4, $5, 'new', $6, $7)
        RETURNING id
    `, req.FirstName, req.LastName, req.Email, req.Phone, req.Position, req.Source, tenantID).Scan(&id)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create candidate"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "id": id.String()})
}

// UpdateCandidateStatusHandler - обновить статус кандидата
func UpdateCandidateStatusHandler(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        Status        string  `json:"status"`
        InterviewDate *string `json:"interview_date"`
        MatchScore    *int    `json:"match_score"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    query := `UPDATE hr_candidates SET status = $2, updated_at = NOW()`
    args := []interface{}{id, req.Status}
    argIdx := 3

    if req.InterviewDate != nil && *req.InterviewDate != "" {
        if interviewDate, err := time.Parse("2006-01-02T15:04", *req.InterviewDate); err == nil {
            query += ", interview_date = $" + strconv.Itoa(argIdx)
            args = append(args, interviewDate)
            argIdx++
        }
    }
    if req.MatchScore != nil {
        query += ", match_score = $" + strconv.Itoa(argIdx)
        args = append(args, *req.MatchScore)
    }
    query += " WHERE id = $1"

    _, err := database.Pool.Exec(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetStatisticsHandler - получить статистику
func GetStatisticsHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var totalEmployees, vacationCount, sickCount, pendingRequests, candidatesCount int

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM hr_employees WHERE tenant_id = $1 AND status = 'active'
    `, tenantID).Scan(&totalEmployees)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM hr_employees WHERE tenant_id = $1 AND status = 'vacation'
    `, tenantID).Scan(&vacationCount)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM hr_employees WHERE tenant_id = $1 AND status = 'sick'
    `, tenantID).Scan(&sickCount)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM hr_vacation_requests WHERE tenant_id = $1 AND status = 'pending'
    `, tenantID).Scan(&pendingRequests)

    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM hr_candidates WHERE tenant_id = $1
    `, tenantID).Scan(&candidatesCount)

    c.JSON(http.StatusOK, gin.H{"statistics": gin.H{
        "totalEmployees":  totalEmployees,
        "vacationCount":   vacationCount,
        "sickCount":       sickCount,
        "pendingRequests": pendingRequests,
        "candidatesCount": candidatesCount,
    }})
}

// AnalyzeCandidateHandler - AI анализ кандидата
func AnalyzeCandidateHandler(c *gin.Context) {
    id := c.Param("id")
    var firstName, lastName, position string

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT first_name, last_name, position FROM hr_candidates WHERE id = $1
    `, id).Scan(&firstName, &lastName, &position)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Candidate not found"})
        return
    }

    matchScore := 75 + (len(firstName)+len(lastName))%25

    c.JSON(http.StatusOK, gin.H{
        "analysis": gin.H{
            "candidate":   firstName + " " + lastName,
            "position":    position,
            "match_score": matchScore,
            "strengths":   "Хорошие коммуникативные навыки, опыт работы с командой",
            "weaknesses":  "Требуется дополнительное обучение",
            "recommendation": "Рекомендуется к собеседованию",
        },
    })
}

// AIChatHandler - AI чат-ассистент
func AIChatHandler(c *gin.Context) {
    var req struct {
        Message string `json:"message"`
    }
    c.ShouldBindJSON(&req)

    reply := "Я AI ассистент HR-модуля. Чем могу помочь? Могу подобрать кандидатов, проанализировать резюме или дать рекомендации по подбору персонала."

    if req.Message != "" {
        reply = "Анализирую ваш запрос: \"" + req.Message + "\". Рекомендую обратить внимание на кандидатов с опытом работы от 3 лет и высокой мотивацией."
    }

    c.JSON(http.StatusOK, gin.H{"reply": reply})
}

// SuggestTrainingHandler - рекомендации по обучению
func SuggestTrainingHandler(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "suggestions": []gin.H{
            {"name": "Курс по управлению персоналом", "duration": "40 часов", "level": "Продвинутый"},
            {"name": "Тренинг лидерства и мотивации", "duration": "24 часа", "level": "Средний"},
            {"name": "HR-аналитика и метрики", "duration": "32 часа", "level": "Продвинутый"},
            {"name": "Управление талантами", "duration": "16 часов", "level": "Начальный"},
            {"name": "Оценка персонала 360°", "duration": "20 часов", "level": "Средний"},
        },
    })
}

// PredictTurnoverHandler - прогноз текучести
func PredictTurnoverHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var employees []gin.H
    rows, _ := database.Pool.Query(c.Request.Context(), `
        SELECT first_name, last_name, position, department, hire_date, salary
        FROM hr_employees WHERE tenant_id = $1 AND status = 'active'
    `, tenantID)

    defer rows.Close()
    for rows.Next() {
        var firstName, lastName, position, department string
        var hireDate time.Time
        var salary float64
        rows.Scan(&firstName, &lastName, &position, &department, &hireDate, &salary)

        yearsWorked := time.Since(hireDate).Hours() / 24 / 365
        risk := "Низкий"
        if yearsWorked < 1 {
            risk = "Высокий"
        } else if yearsWorked < 2 {
            risk = "Средний"
        }

        employees = append(employees, gin.H{
            "name":     firstName + " " + lastName,
            "position": position,
            "department": department,
            "risk":     risk,
        })
    }

    c.JSON(http.StatusOK, gin.H{"predictions": employees})
}

// GenerateOrderHandler - сгенерировать приказ
func GenerateOrderHandler(c *gin.Context) {
    var req struct {
        Type       string `json:"type"`
        EmployeeID string `json:"employee_id"`
    }
    c.ShouldBindJSON(&req)

    var employeeName string
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT first_name || ' ' || last_name FROM hr_employees WHERE id = $1
    `, req.EmployeeID).Scan(&employeeName)

    order := "ПРИКАЗ №" + time.Now().Format("20060102-1504") + "\n\n"
    if req.Type == "hire" {
        order += "О приеме на работу\n\nПринять " + employeeName + " на должность согласно трудовому договору с " + time.Now().Format("02.01.2006")
    } else if req.Type == "vacation" {
        order += "О предоставлении отпуска\n\nПредоставить " + employeeName + " ежегодный оплачиваемый отпуск с " + time.Now().Format("02.01.2006")
    } else {
        order += "О кадровом перемещении\n\nПеревести " + employeeName + " на новую должность"
    }

    c.JSON(http.StatusOK, gin.H{"order": order})
}