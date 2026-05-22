package handlers

import (
    "log"
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)

// ==================== ОСНОВНЫЕ СТРАНИЦЫ ====================

func HRDashboardHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "hr/hr.html", gin.H{
        "Title": "HR-модуль | Business Stack",
    })
}

// ==================== СОТРУДНИКИ (EMPLOYEES) ====================

func GetEmployeesHandler(c *gin.Context) {
    log.Println("🔍 GetEmployeesHandler вызван")
    
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, 
               COALESCE(first_name, '') as first_name,
               COALESCE(last_name, '') as last_name,
               COALESCE(email, '') as email,
               COALESCE(phone, '') as phone,
               COALESCE(position, '') as position,
               COALESCE(department, '') as department,
               COALESCE(hire_date, NOW()) as hire_date,
               COALESCE(salary, 0) as salary,
               COALESCE(status, 'active') as status,
               COALESCE(created_at, NOW()) as created_at
        FROM hr_employees 
        WHERE tenant_id = $1 
        ORDER BY created_at DESC
    `, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка запроса: %v", err)
        c.JSON(http.StatusOK, gin.H{"employees": []interface{}{}})
        return
    }
    defer rows.Close()
    
    var employees []gin.H
    for rows.Next() {
        var id uuid.UUID
        var firstName, lastName, email, phone, position, department, status string
        var hireDate, createdAt time.Time
        var salary float64
        
        err := rows.Scan(&id, &firstName, &lastName, &email, &phone, &position, &department,
            &hireDate, &salary, &status, &createdAt)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
            continue
        }
        
        employee := gin.H{
            "id":         id.String(),
            "first_name": firstName,
            "last_name":  lastName,
            "full_name":  firstName + " " + lastName,
            "email":      email,
            "phone":      phone,
            "position":   position,
            "department": department,
            "hire_date":  hireDate.Format("2006-01-02"),
            "salary":     salary,
            "status":     status,
            "created_at": createdAt.Format("2006-01-02"),
        }
        
        employees = append(employees, employee)
    }
    
    log.Printf("📊 ИТОГО сотрудников: %d", len(employees))
    
    if employees == nil {
        employees = []gin.H{}
    }
    
    c.JSON(http.StatusOK, gin.H{"employees": employees})
}

func AddEmployeeHandler(c *gin.Context) {
    var req struct {
        FirstName  string  `json:"first_name" binding:"required"`
        LastName   string  `json:"last_name" binding:"required"`
        Email      string  `json:"email"`
        Phone      string  `json:"phone"`
        Position   string  `json:"position" binding:"required"`
        Department string  `json:"department" binding:"required"`
        Salary     float64 `json:"salary"`
        HireDate   string  `json:"hire_date"`
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

    var id uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO hr_employees (id, first_name, last_name, email, phone, position, department, 
        hire_date, salary, status, tenant_id)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, 'active', $9)
        RETURNING id
    `, req.FirstName, req.LastName, req.Email, req.Phone, req.Position, req.Department,
        hireDate, req.Salary, tenantID).Scan(&id)

    if err != nil {
        log.Printf("❌ Ошибка добавления: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create employee"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "id": id.String()})
}

func UpdateEmployeeHandler(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        FirstName  string  `json:"first_name"`
        LastName   string  `json:"last_name"`
        Email      string  `json:"email"`
        Phone      string  `json:"phone"`
        Position   string  `json:"position"`
        Department string  `json:"department"`
        Salary     float64 `json:"salary"`
        Status     string  `json:"status"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    query := `UPDATE hr_employees SET updated_at = NOW()`
    args := []interface{}{}
    argIdx := 1

    if req.FirstName != "" {
        argIdx++
        query += ", first_name = $" + strconv.Itoa(argIdx)
        args = append(args, req.FirstName)
    }
    if req.LastName != "" {
        argIdx++
        query += ", last_name = $" + strconv.Itoa(argIdx)
        args = append(args, req.LastName)
    }
    if req.Email != "" {
        argIdx++
        query += ", email = $" + strconv.Itoa(argIdx)
        args = append(args, req.Email)
    }
    if req.Phone != "" {
        argIdx++
        query += ", phone = $" + strconv.Itoa(argIdx)
        args = append(args, req.Phone)
    }
    if req.Position != "" {
        argIdx++
        query += ", position = $" + strconv.Itoa(argIdx)
        args = append(args, req.Position)
    }
    if req.Department != "" {
        argIdx++
        query += ", department = $" + strconv.Itoa(argIdx)
        args = append(args, req.Department)
    }
    if req.Salary > 0 {
        argIdx++
        query += ", salary = $" + strconv.Itoa(argIdx)
        args = append(args, req.Salary)
    }
    if req.Status != "" {
        argIdx++
        query += ", status = $" + strconv.Itoa(argIdx)
        args = append(args, req.Status)
    }
    
    query += " WHERE id = $1"
    args = append([]interface{}{id}, args...)

    _, err := database.Pool.Exec(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

func DeleteEmployeeHandler(c *gin.Context) {
    id := c.Param("id")
    log.Printf("🔍 Удаление сотрудника с ID: %s", id)
    
    _, err := uuid.Parse(id)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат ID"})
        return
    }
    
    result, err := database.Pool.Exec(c.Request.Context(), 
        `UPDATE hr_employees SET status = 'fired', updated_at = NOW() WHERE id = $1`, id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Сотрудник не найден"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Сотрудник уволен"})
}

// ==================== ОТПУСКА (VACATIONS) ====================

func GetVacationRequestsHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT r.id, r.employee_id, r.start_date, r.end_date, 
               COALESCE(r.days, 0) as days,
               COALESCE(r.type, 'vacation') as type,
               r.status, 
               COALESCE(r.comment, '') as comment,
               r.created_at,
               e.first_name, e.last_name, e.position, e.department
        FROM hr_vacation_requests r
        JOIN hr_employees e ON r.employee_id = e.id
        WHERE r.tenant_id = $1
        ORDER BY r.created_at DESC
    `, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка запроса: %v", err)
        c.JSON(http.StatusOK, gin.H{"requests": []interface{}{}})
        return
    }
    defer rows.Close()

    var requests []gin.H
    for rows.Next() {
        var id uuid.UUID
        var employeeID uuid.UUID
        var startDate, endDate, createdAt time.Time
        var days int
        var reqType, status, comment, firstName, lastName, position, department string

        err := rows.Scan(&id, &employeeID, &startDate, &endDate, &days, &reqType, &status, &comment, &createdAt,
            &firstName, &lastName, &position, &department)
        
        if err != nil {
            log.Printf("Ошибка сканирования: %v", err)
            continue
        }

        requests = append(requests, gin.H{
            "id":            id.String(),
            "employee_id":   employeeID.String(),
            "employee_name": firstName + " " + lastName,
            "position":      position,
            "department":    department,
            "start_date":    startDate.Format("2006-01-02"),
            "end_date":      endDate.Format("2006-01-02"),
            "days":          days,
            "type":          reqType,
            "status":        status,
            "comment":       comment,
            "created_at":    createdAt.Format("2006-01-02"),
        })
    }

    c.JSON(http.StatusOK, gin.H{"requests": requests})
}

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

    employeeUUID, _ := uuid.Parse(req.EmployeeID)

    var id uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO hr_vacation_requests (id, employee_id, start_date, end_date, days, type, status, comment, tenant_id)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'pending', $6, $7)
        RETURNING id
    `, employeeUUID, startDate, endDate, days, req.Type, req.Comment, tenantID).Scan(&id)

    if err != nil {
        log.Printf("❌ Ошибка создания заявки: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось создать заявку"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "id": id.String()})
}

func ApproveRequestHandler(c *gin.Context) {
    id := c.Param("id")
    
    if _, err := uuid.Parse(id); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат ID"})
        return
    }
    
    result, err := database.Pool.Exec(c.Request.Context(), 
        `UPDATE hr_vacation_requests SET status = 'approved', approved_at = NOW() WHERE id = $1`, id)
    if err != nil {
        log.Printf("❌ Ошибка approve: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Заявка не найдена"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

func RejectRequestHandler(c *gin.Context) {
    id := c.Param("id")
    
    if _, err := uuid.Parse(id); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат ID"})
        return
    }
    
    result, err := database.Pool.Exec(c.Request.Context(), 
        `UPDATE hr_vacation_requests SET status = 'rejected' WHERE id = $1`, id)
    if err != nil {
        log.Printf("❌ Ошибка reject: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Заявка не найдена"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== КАНДИДАТЫ (CANDIDATES) ====================

func GetCandidatesHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, first_name, last_name, email, phone, position, 
               COALESCE(status, 'new') as status,
               COALESCE(source, '') as source,
               COALESCE(match_score, 0) as match_score,
               interview_date,
               created_at
        FROM hr_candidates 
        WHERE tenant_id = $1 
        ORDER BY created_at DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"candidates": []interface{}{}})
        return
    }
    defer rows.Close()

    candidates := gin.H{
        "new":       []gin.H{},
        "interview": []gin.H{},
        "offer":     []gin.H{},
        "hired":     []gin.H{},
        "rejected":  []gin.H{},
    }

    for rows.Next() {
        var id uuid.UUID
        var firstName, lastName, email, phone, position, status, source string
        var matchScore int
        var interviewDate *time.Time
        var createdAt time.Time

        rows.Scan(&id, &firstName, &lastName, &email, &phone, &position, 
            &status, &source, &matchScore, &interviewDate, &createdAt)

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
            "status":      status,
            "created_at":  createdAt.Format("2006-01-02"),
        }

        if interviewDate != nil {
            cand["interview_date"] = interviewDate.Format("2006-01-02")
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
        case "rejected":
            candidates["rejected"] = append(candidates["rejected"].([]gin.H), cand)
        default:
            candidates["new"] = append(candidates["new"].([]gin.H), cand)
        }
    }

    c.JSON(http.StatusOK, gin.H{"candidates": candidates})
}

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
        INSERT INTO hr_candidates (id, first_name, last_name, email, phone, position, status, source, tenant_id)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, 'new', $6, $7)
        RETURNING id
    `, req.FirstName, req.LastName, req.Email, req.Phone, req.Position, req.Source, tenantID).Scan(&id)

    if err != nil {
        log.Printf("❌ Ошибка создания кандидата: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create candidate"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "id": id.String()})
}

func UpdateCandidateStatusHandler(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        Status string `json:"status"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(), 
        `UPDATE hr_candidates SET status = $1, updated_at = NOW() WHERE id = $2`, req.Status, id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

func DeleteCandidateHandler(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(), `DELETE FROM hr_candidates WHERE id = $1`, id)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== ВАКАНСИИ (VACANCIES) ====================

func GetVacanciesHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, title, department, 
               COALESCE(description, '') as description,
               COALESCE(requirements, '') as requirements,
               COALESCE(salary_min, 0) as salary_min,
               COALESCE(salary_max, 0) as salary_max,
               COALESCE(location, '') as location,
               COALESCE(is_remote, false) as is_remote,
               status, 
               COALESCE(priority, 0) as priority,
               COALESCE(views_count, 0) as views_count,
               COALESCE(applications_count, 0) as applications_count,
               created_at
        FROM hr_vacancies 
        WHERE tenant_id = $1 
        ORDER BY created_at DESC
    `, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка запроса вакансий: %v", err)
        c.JSON(http.StatusOK, gin.H{"vacancies": []interface{}{}})
        return
    }
    defer rows.Close()

    var vacancies []gin.H
    for rows.Next() {
        var id uuid.UUID
        var title, department, description, requirements, location, status string
        var salaryMin, salaryMax float64
        var isRemote bool
        var priority, viewsCount, applicationsCount int
        var createdAt time.Time

        err := rows.Scan(&id, &title, &department, &description, &requirements, 
            &salaryMin, &salaryMax, &location, &isRemote,
            &status, &priority, &viewsCount, &applicationsCount, &createdAt)
        
        if err != nil {
            log.Printf("Ошибка сканирования: %v", err)
            continue
        }

        vacancies = append(vacancies, gin.H{
            "id":                 id.String(),
            "title":              title,
            "department":         department,
            "description":        description,
            "requirements":       requirements,
            "salary_min":         salaryMin,
            "salary_max":         salaryMax,
            "location":           location,
            "is_remote":          isRemote,
            "status":             status,
            "priority":           priority,
            "views_count":        viewsCount,
            "applications_count": applicationsCount,
            "created_at":         createdAt.Format("2006-01-02"),
        })
    }

    c.JSON(http.StatusOK, gin.H{"vacancies": vacancies})
}

func AddVacancyHandler(c *gin.Context) {
    var req struct {
        Title        string  `json:"title" binding:"required"`
        Department   string  `json:"department" binding:"required"`
        Description  string  `json:"description"`
        Requirements string  `json:"requirements"`
        SalaryMin    float64 `json:"salary_min"`
        SalaryMax    float64 `json:"salary_max"`
        Location     string  `json:"location"`
        IsRemote     bool    `json:"is_remote"`
        Priority     int     `json:"priority"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var id uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO hr_vacancies (id, title, department, description, requirements, 
                                  salary_min, salary_max, location, is_remote, 
                                  status, priority, tenant_id)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, 'open', $9, $10)
        RETURNING id
    `, req.Title, req.Department, req.Description, req.Requirements, 
        req.SalaryMin, req.SalaryMax, req.Location, req.IsRemote, 
        req.Priority, tenantID).Scan(&id)

    if err != nil {
        log.Printf("❌ Ошибка создания вакансии: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create vacancy: " + err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "id": id.String()})
}

func UpdateVacancyHandler(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        Title        string  `json:"title"`
        Department   string  `json:"department"`
        Description  string  `json:"description"`
        Requirements string  `json:"requirements"`
        SalaryMin    float64 `json:"salary_min"`
        SalaryMax    float64 `json:"salary_max"`
        Location     string  `json:"location"`
        IsRemote     bool    `json:"is_remote"`
        Status       string  `json:"status"`
        Priority     int     `json:"priority"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    query := `UPDATE hr_vacancies SET updated_at = NOW()`
    args := []interface{}{}
    argIdx := 1

    if req.Title != "" {
        argIdx++
        query += ", title = $" + strconv.Itoa(argIdx)
        args = append(args, req.Title)
    }
    if req.Department != "" {
        argIdx++
        query += ", department = $" + strconv.Itoa(argIdx)
        args = append(args, req.Department)
    }
    if req.Description != "" {
        argIdx++
        query += ", description = $" + strconv.Itoa(argIdx)
        args = append(args, req.Description)
    }
    if req.Requirements != "" {
        argIdx++
        query += ", requirements = $" + strconv.Itoa(argIdx)
        args = append(args, req.Requirements)
    }
    if req.SalaryMin > 0 {
        argIdx++
        query += ", salary_min = $" + strconv.Itoa(argIdx)
        args = append(args, req.SalaryMin)
    }
    if req.SalaryMax > 0 {
        argIdx++
        query += ", salary_max = $" + strconv.Itoa(argIdx)
        args = append(args, req.SalaryMax)
    }
    if req.Location != "" {
        argIdx++
        query += ", location = $" + strconv.Itoa(argIdx)
        args = append(args, req.Location)
    }
    if req.IsRemote {
        argIdx++
        query += ", is_remote = $" + strconv.Itoa(argIdx)
        args = append(args, req.IsRemote)
    }
    if req.Status != "" {
        argIdx++
        query += ", status = $" + strconv.Itoa(argIdx)
        args = append(args, req.Status)
    }
    if req.Priority > 0 {
        argIdx++
        query += ", priority = $" + strconv.Itoa(argIdx)
        args = append(args, req.Priority)
    }
    
    query += " WHERE id = $1"
    args = append([]interface{}{id}, args...)

    _, err := database.Pool.Exec(c.Request.Context(), query, args...)
    if err != nil {
        log.Printf("❌ Ошибка обновления вакансии: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

func DeleteVacancyHandler(c *gin.Context) {
    id := c.Param("id")
    
    if _, err := uuid.Parse(id); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат ID"})
        return
    }
    
    result, err := database.Pool.Exec(c.Request.Context(), `DELETE FROM hr_vacancies WHERE id = $1`, id)
    if err != nil {
        log.Printf("❌ Ошибка удаления вакансии: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    if result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Вакансия не найдена"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== СТАТИСТИКА И АНАЛИТИКА ====================

func GetStatisticsHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    var totalEmployees, vacationCount, pendingRequests, candidatesCount, vacanciesCount int
    var totalSalary, avgSalary float64
    
    database.Pool.QueryRow(c.Request.Context(), 
        `SELECT COUNT(*) FROM hr_employees WHERE tenant_id = $1 AND status = 'active'`, 
        tenantID).Scan(&totalEmployees)
    
    database.Pool.QueryRow(c.Request.Context(), 
        `SELECT COUNT(*) FROM hr_vacation_requests WHERE tenant_id = $1 AND status = 'approved'`, 
        tenantID).Scan(&vacationCount)
    
    database.Pool.QueryRow(c.Request.Context(), 
        `SELECT COUNT(*) FROM hr_vacation_requests WHERE tenant_id = $1 AND status = 'pending'`, 
        tenantID).Scan(&pendingRequests)
    
    database.Pool.QueryRow(c.Request.Context(), 
        `SELECT COUNT(*) FROM hr_candidates WHERE tenant_id = $1`, 
        tenantID).Scan(&candidatesCount)
    
    database.Pool.QueryRow(c.Request.Context(), 
        `SELECT COUNT(*) FROM hr_vacancies WHERE tenant_id = $1 AND status = 'open'`, 
        tenantID).Scan(&vacanciesCount)
    
    database.Pool.QueryRow(c.Request.Context(), 
        `SELECT COALESCE(SUM(salary), 0), COALESCE(AVG(salary), 0) FROM hr_employees WHERE tenant_id = $1 AND status = 'active'`, 
        tenantID).Scan(&totalSalary, &avgSalary)
    
    c.JSON(http.StatusOK, gin.H{"statistics": gin.H{
        "totalEmployees":  totalEmployees,
        "vacationCount":   vacationCount,
        "pendingRequests": pendingRequests,
        "candidatesCount": candidatesCount,
        "vacanciesCount":  vacanciesCount,
        "totalSalary":     totalSalary,
        "avgSalary":       avgSalary,
    }})
}

func AnalyzeCandidateHandler(c *gin.Context) {
    id := c.Param("id")
    var firstName, lastName, position string

    database.Pool.QueryRow(c.Request.Context(), 
        `SELECT first_name, last_name, position FROM hr_candidates WHERE id = $1`, id).Scan(&firstName, &lastName, &position)

    matchScore := 75 + (len(firstName)+len(lastName))%25

    c.JSON(http.StatusOK, gin.H{
        "analysis": gin.H{
            "candidate":      firstName + " " + lastName,
            "position":       position,
            "match_score":    matchScore,
            "strengths":      "Хорошие коммуникативные навыки, опыт работы с командой",
            "weaknesses":     "Требуется дополнительное обучение",
            "recommendation": "Рекомендуется к собеседованию",
        },
    })
}

func AIChatHandler(c *gin.Context) {
    var req struct{ Message string }
    c.ShouldBindJSON(&req)

    reply := "Я AI ассистент HR-модуля. Чем могу помочь? Могу подобрать кандидатов, проанализировать резюме или дать рекомендации по подбору персонала."

    if req.Message != "" {
        reply = "Анализирую ваш запрос: \"" + req.Message + "\". Рекомендую обратить внимание на кандидатов с опытом работы от 3 лет и высокой мотивацией."
    }

    c.JSON(http.StatusOK, gin.H{"reply": reply})
}

func SuggestTrainingHandler(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "suggestions": []gin.H{
            {"name": "Курс по управлению персоналом", "duration": "40 часов", "level": "Продвинутый", "price": 15000},
            {"name": "Тренинг лидерства и мотивации", "duration": "24 часа", "level": "Средний", "price": 12000},
            {"name": "HR-аналитика и метрики", "duration": "32 часа", "level": "Продвинутый", "price": 18000},
        },
    })
}

func PredictTurnoverHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var predictions []gin.H
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT first_name, last_name, position, department, hire_date
        FROM hr_employees WHERE tenant_id = $1 AND status = 'active'
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"predictions": []interface{}{}})
        return
    }
    defer rows.Close()

    for rows.Next() {
        var firstName, lastName, position, department string
        var hireDate time.Time
        rows.Scan(&firstName, &lastName, &position, &department, &hireDate)

        yearsWorked := time.Since(hireDate).Hours() / 24 / 365
        risk := "Низкий"
        riskColor := "green"
        if yearsWorked < 1 {
            risk = "Высокий"
            riskColor = "red"
        } else if yearsWorked < 2 {
            risk = "Средний"
            riskColor = "orange"
        }

        predictions = append(predictions, gin.H{
            "name":       firstName + " " + lastName,
            "position":   position,
            "department": department,
            "risk":       risk,
            "risk_color": riskColor,
            "years":      yearsWorked,
        })
    }

    c.JSON(http.StatusOK, gin.H{"predictions": predictions})
}

func GenerateOrderHandler(c *gin.Context) {
    var req struct {
        Type       string `json:"type"`
        EmployeeID string `json:"employee_id"`
    }
    c.ShouldBindJSON(&req)

    var employeeName, position string
    database.Pool.QueryRow(c.Request.Context(), 
        `SELECT first_name || ' ' || last_name, position FROM hr_employees WHERE id = $1`, 
        req.EmployeeID).Scan(&employeeName, &position)

    order := "ПРИКАЗ №" + time.Now().Format("20060102-1504") + "\n\n"
    if req.Type == "hire" {
        order += "О приеме на работу\n\nПринять " + employeeName + " на должность " + position + " с " + time.Now().Format("02.01.2006")
    } else if req.Type == "vacation" {
        order += "О предоставлении отпуска\n\nПредоставить " + employeeName + " ежегодный оплачиваемый отпуск с " + time.Now().Format("02.01.2006")
    } else {
        order += "О кадровом перемещении\n\nПеревести " + employeeName + " на новую должность"
    }

    c.JSON(http.StatusOK, gin.H{"order": order})
}

func GetDepartmentsHandler(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT department, COUNT(*) as count, SUM(salary) as budget
        FROM hr_employees 
        WHERE tenant_id = $1 AND status = 'active'
        GROUP BY department
        ORDER BY department
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"departments": []interface{}{}})
        return
    }
    defer rows.Close()

    var departments []gin.H
    for rows.Next() {
        var department string
        var count int
        var budget float64
        rows.Scan(&department, &count, &budget)

        departments = append(departments, gin.H{
            "name":   department,
            "count":  count,
            "budget": budget,
        })
    }

    c.JSON(http.StatusOK, gin.H{"departments": departments})
}