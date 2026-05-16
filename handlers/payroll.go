package handlers

import (
    "fmt"
    "log"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "subscription-system/database"
)

func GetEmployeesForPayroll(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, first_name, last_name, position, 
               COALESCE(department, '') as department,
               COALESCE(salary, 0) as salary, 
               COALESCE(email, '') as email,
               COALESCE(phone, '') as phone,
               COALESCE(hire_date::text, '') as hire_date,
               status
        FROM hr_employees 
        WHERE tenant_id = $1 AND status = 'active'
        ORDER BY last_name
    `, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка загрузки сотрудников: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load employees: " + err.Error()})
        return
    }
    defer rows.Close()

    var employees []gin.H
    for rows.Next() {
        var id uuid.UUID
        var firstName, lastName, position, department, email, phone, hireDate, status string
        var salary float64

        err := rows.Scan(&id, &firstName, &lastName, &position, &department, &salary, &email, &phone, &hireDate, &status)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
            continue
        }

        tax := salary * 0.13
        netAmount := salary - tax

        employees = append(employees, gin.H{
            "id":         id,
            "full_name":  firstName + " " + lastName,
            "first_name": firstName,
            "last_name":  lastName,
            "position":   position,
            "department": department,
            "salary":     salary,
            "rate":       100,
            "email":      email,
            "phone":      phone,
            "hire_date":  hireDate,
            "status":     status,
            "tax":        tax,
            "net_amount": netAmount,
        })
    }

    c.JSON(http.StatusOK, gin.H{"employees": employees})
}

func DeleteEmployeeFromPayroll(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    employeeID := c.Param("id")

    log.Printf("🔍 Удаление сотрудника ID: %s, tenant: %s", employeeID, tenantID)

    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM hr_employees WHERE id = $1 AND tenant_id = $2)
    `, employeeID, tenantID).Scan(&exists)

    if err != nil {
        log.Printf("❌ Ошибка проверки существования: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки"})
        return
    }

    if !exists {
        log.Printf("⚠️ Сотрудник %s не найден", employeeID)
        c.JSON(http.StatusNotFound, gin.H{"error": "Сотрудник не найден"})
        return
    }

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM hr_employees WHERE id = $1 AND tenant_id = $2
    `, employeeID, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка удаления: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления: " + err.Error()})
        return
    }

    rowsAffected := result.RowsAffected()
    log.Printf("📊 Удалено строк: %d", rowsAffected)

    if rowsAffected == 0 {
        log.Printf("⚠️ Сотрудник %s не был удалён", employeeID)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Сотрудник не был удалён"})
        return
    }

    var stillExists bool
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM hr_employees WHERE id = $1)
    `, employeeID).Scan(&stillExists)

    if stillExists {
        log.Printf("❌ КРИТИЧЕСКАЯ ОШИБКА: Сотрудник %s всё ещё в БД после DELETE!", employeeID)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Сотрудник не был удалён из-за ошибки БД"})
        return
    }

    log.Printf("✅ Сотрудник %s успешно удалён", employeeID)
    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "message":       "Сотрудник удалён",
        "rows_affected": rowsAffected,
    })
}

func CalculatePayroll(c *gin.Context) {
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

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, first_name, last_name, salary, tax_rate
        FROM hr_employees 
        WHERE tenant_id = $1 AND status = 'active'
    `, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка загрузки сотрудников: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load employees"})
        return
    }
    defer rows.Close()

    var payrolls []gin.H
    for rows.Next() {
        var id uuid.UUID
        var firstName, lastName string
        var salary, taxRate float64

        rows.Scan(&id, &firstName, &lastName, &salary, &taxRate)

        tax := salary * taxRate / 100
        netAmount := salary - tax

        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO payroll (id, tenant_id, employee_id, period_month, period_year, salary, tax, net_amount, status, created_at)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'calculated', NOW())
            ON CONFLICT (employee_id, period_month, period_year) DO UPDATE
            SET salary = $6, tax = $7, net_amount = $8, status = 'calculated'
        `, uuid.New(), tenantID, id, req.Month, req.Year, salary, tax, netAmount)

        if err != nil {
            log.Printf("⚠️ Ошибка вставки payroll: %v", err)
        }

        payrolls = append(payrolls, gin.H{
            "employee_id": id,
            "name":        firstName + " " + lastName,
            "salary":      salary,
            "tax":         tax,
            "net_amount":  netAmount,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "message":  "Расчёт выполнен",
        "payrolls": payrolls,
        "total":    len(payrolls),
        "month":    req.Month,
        "year":     req.Year,
    })
}

func GetPayrollHistory(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT p.id, e.first_name, e.last_name, p.period_month, p.period_year, 
               p.salary, p.tax, p.net_amount, p.status, p.created_at
        FROM payroll p
        JOIN hr_employees e ON p.employee_id = e.id
        WHERE p.tenant_id = $1
        ORDER BY p.period_year DESC, p.period_month DESC, e.last_name
        LIMIT 100
    `, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка загрузки истории: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load history"})
        return
    }
    defer rows.Close()

    var history []gin.H
    for rows.Next() {
        var id uuid.UUID
        var firstName, lastName string
        var month, year int
        var salary, tax, netAmount float64
        var status string
        var createdAt time.Time

        err := rows.Scan(&id, &firstName, &lastName, &month, &year, &salary, &tax, &netAmount, &status, &createdAt)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
            continue
        }

        history = append(history, gin.H{
            "id":         id,
            "employee":   firstName + " " + lastName,
            "period":     fmt.Sprintf("%d/%d", month, year),
            "salary":     salary,
            "tax":        tax,
            "net_amount": netAmount,
            "status":     status,
            "created_at": createdAt.Format("2006-01-02"),
        })
    }

    c.JSON(http.StatusOK, gin.H{"history": history})
}

func ProcessPayrollPayment(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var req struct {
        PayrollID string `json:"payroll_id" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE payroll 
        SET status = 'paid', paid_at = NOW()
        WHERE id = $1 AND tenant_id = $2
    `, req.PayrollID, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка выплаты: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process payment"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "Зарплата выплачена"})
}

func GenerateTaxReport(c *gin.Context) {
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

    var totalIncome, totalTax float64
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT COALESCE(SUM(salary), 0), COALESCE(SUM(tax), 0)
        FROM payroll
        WHERE tenant_id = $1 AND period_month = $2 AND period_year = $3
    `, tenantID, req.Month, req.Year)

    if err == nil {
        defer rows.Close()
        if rows.Next() {
            rows.Scan(&totalIncome, &totalTax)
        }
    }

    reportID := uuid.New()
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tax_reports (id, tenant_id, report_type, period_month, period_year, total_income, total_tax, created_at)
        VALUES ($1, $2, '6-НДФЛ', $3, $4, $5, $6, NOW())
    `, reportID, tenantID, req.Month, req.Year, totalIncome, totalTax)

    if err != nil {
        log.Printf("❌ Ошибка генерации отчёта: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate report"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message":      "Отчёт сгенерирован",
        "report_id":    reportID,
        "total_income": totalIncome,
        "total_tax":    totalTax,
        "month":        req.Month,
        "year":         req.Year,
    })
}

func CalculateSickLeave(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var req struct {
        EmployeeID string    `json:"employee_id" binding:"required"`
        StartDate  time.Time `json:"start_date" binding:"required"`
        EndDate    time.Time `json:"end_date" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var salary, experienceYears float64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(salary, 0), COALESCE(EXTRACT(YEAR FROM AGE(NOW(), hire_date)), 0)
        FROM hr_employees 
        WHERE id = $1 AND tenant_id = $2
    `, req.EmployeeID, tenantID).Scan(&salary, &experienceYears)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Employee not found"})
        return
    }

    daysCount := int(req.EndDate.Sub(req.StartDate).Hours()/24) + 1

    var payPercent float64
    if experienceYears < 5 {
        payPercent = 0.60
    } else if experienceYears < 8 {
        payPercent = 0.80
    } else {
        payPercent = 1.00
    }

    avgDailySalary := salary / 29.3
    amount := avgDailySalary * float64(daysCount) * payPercent

    sickLeaveID := uuid.New()
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO sick_leaves (id, tenant_id, employee_id, start_date, end_date, days_count, amount, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, 'approved', NOW())
    `, sickLeaveID, tenantID, req.EmployeeID, req.StartDate, req.EndDate, daysCount, amount)

    if err != nil {
        log.Printf("❌ Ошибка сохранения больничного: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save sick leave"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message":           "Больничный рассчитан",
        "sick_leave_id":     sickLeaveID,
        "days_count":        daysCount,
        "pay_percent":       payPercent * 100,
        "amount":            amount,
        "experience_years":  experienceYears,
    })
}

func CalculateVacation(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var req struct {
        EmployeeID string    `json:"employee_id" binding:"required"`
        StartDate  time.Time `json:"start_date" binding:"required"`
        DaysCount  int       `json:"days_count" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var avgSalary float64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(AVG(salary), 0)
        FROM payroll
        WHERE employee_id = $1 AND tenant_id = $2 
        AND period_year >= EXTRACT(YEAR FROM NOW()) - 1
    `, req.EmployeeID, tenantID).Scan(&avgSalary)

    if err != nil || avgSalary == 0 {
        database.Pool.QueryRow(c.Request.Context(), `
            SELECT COALESCE(salary, 0) FROM hr_employees WHERE id = $1
        `, req.EmployeeID).Scan(&avgSalary)
    }

    avgDailySalary := avgSalary / 29.3
    amount := avgDailySalary * float64(req.DaysCount)

    vacationID := uuid.New()
    endDate := req.StartDate.AddDate(0, 0, req.DaysCount-1)
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO vacations (id, tenant_id, employee_id, start_date, end_date, days_count, amount, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, 'approved', NOW())
    `, vacationID, tenantID, req.EmployeeID, req.StartDate, endDate, req.DaysCount, amount)

    if err != nil {
        log.Printf("❌ Ошибка сохранения отпуска: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save vacation"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message":     "Отпускные рассчитаны",
        "vacation_id": vacationID,
        "days_count":  req.DaysCount,
        "avg_salary":  avgSalary,
        "amount":      amount,
        "end_date":    endDate,
    })
}

func CalculateAlimony(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var req struct {
        EmployeeID    string  `json:"employee_id" binding:"required"`
        ChildrenCount int     `json:"children_count" binding:"required"`
        NetSalary     float64 `json:"net_salary"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    netSalary := req.NetSalary
    if netSalary == 0 {
        database.Pool.QueryRow(c.Request.Context(), `
            SELECT COALESCE(net_amount, 0)
            FROM payroll
            WHERE employee_id = $1 AND tenant_id = $2
            ORDER BY period_year DESC, period_month DESC
            LIMIT 1
        `, req.EmployeeID, tenantID).Scan(&netSalary)
    }

    var percent float64
    switch req.ChildrenCount {
    case 1:
        percent = 0.25
    case 2:
        percent = 0.33
    default:
        percent = 0.50
    }

    alimonyAmount := netSalary * percent

    c.JSON(http.StatusOK, gin.H{
        "employee_id":     req.EmployeeID,
        "children_count":  req.ChildrenCount,
        "net_salary":      netSalary,
        "percent":         percent * 100,
        "alimony_amount":  alimonyAmount,
        "message":         fmt.Sprintf("Алименты составят %.2f руб. (%.0f%%)", alimonyAmount, percent*100),
    })
}

func GeneratePaymentOrder(c *gin.Context) {
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

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT p.id, e.first_name, e.last_name, e.position, 
               p.salary, p.tax, p.net_amount,
               COALESCE((SELECT amount FROM sick_leaves WHERE employee_id = e.id AND status = 'approved' 
                         AND EXTRACT(YEAR FROM start_date) = $2 AND EXTRACT(MONTH FROM start_date) = $1), 0) as sick_pay,
               COALESCE((SELECT amount FROM vacations WHERE employee_id = e.id AND status = 'approved'
                         AND EXTRACT(YEAR FROM start_date) = $2 AND EXTRACT(MONTH FROM start_date) = $1), 0) as vacation_pay
        FROM payroll p
        JOIN hr_employees e ON p.employee_id = e.id
        WHERE p.tenant_id = $3 AND p.period_month = $1 AND p.period_year = $2
    `, req.Month, req.Year, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var employees []gin.H
    var totalNetAmount float64

    for rows.Next() {
        var id uuid.UUID
        var firstName, lastName, position string
        var salary, tax, netAmount, sickPay, vacationPay float64

        rows.Scan(&id, &firstName, &lastName, &position, &salary, &tax, &netAmount, &sickPay, &vacationPay)

        totalWithAdditions := netAmount + sickPay + vacationPay
        totalNetAmount += totalWithAdditions

        employees = append(employees, gin.H{
            "id":           id,
            "name":         firstName + " " + lastName,
            "position":     position,
            "salary":       salary,
            "sick_pay":     sickPay,
            "vacation_pay": vacationPay,
            "tax":          tax,
            "net_amount":   totalWithAdditions,
        })
    }

    paymentOrder := gin.H{
        "order_number":    fmt.Sprintf("ВП-%d%02d", req.Year, req.Month),
        "date":            time.Now().Format("2006-01-02"),
        "month":           req.Month,
        "year":            req.Year,
        "total_amount":    totalNetAmount,
        "employees_count": len(employees),
        "employees":       employees,
        "status":          "ready_for_payment",
    }

    c.JSON(http.StatusOK, paymentOrder)
}

func GetEmployeePayrollDetails(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    employeeID := c.Param("id")

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT p.period_month, p.period_year, p.salary, p.tax, p.net_amount, p.status, p.paid_at,
               COALESCE((SELECT SUM(amount) FROM sick_leaves WHERE employee_id = p.employee_id 
                         AND EXTRACT(YEAR FROM start_date) = p.period_year 
                         AND EXTRACT(MONTH FROM start_date) = p.period_month), 0) as sick_pay,
               COALESCE((SELECT SUM(amount) FROM vacations WHERE employee_id = p.employee_id
                         AND EXTRACT(YEAR FROM start_date) = p.period_year
                         AND EXTRACT(MONTH FROM start_date) = p.period_month), 0) as vacation_pay
        FROM payroll p
        WHERE p.employee_id = $1 AND p.tenant_id = $2
        ORDER BY p.period_year DESC, p.period_month DESC
    `, employeeID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var history []gin.H
    for rows.Next() {
        var month, year int
        var salary, tax, netAmount, sickPay, vacationPay float64
        var status string
        var paidAt *time.Time

        rows.Scan(&month, &year, &salary, &tax, &netAmount, &status, &paidAt, &sickPay, &vacationPay)

        totalNet := netAmount + sickPay + vacationPay

        history = append(history, gin.H{
            "period":       fmt.Sprintf("%d/%d", month, year),
            "salary":       salary,
            "sick_pay":     sickPay,
            "vacation_pay": vacationPay,
            "tax":          tax,
            "net_amount":   totalNet,
            "status":       status,
            "paid_at":      paidAt,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "employee_id": employeeID,
        "history":     history,
    })
}

func CreatePayrollTables(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        CREATE TABLE IF NOT EXISTS sick_leaves (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id UUID NOT NULL,
            employee_id UUID NOT NULL,
            start_date DATE NOT NULL,
            end_date DATE NOT NULL,
            days_count INTEGER DEFAULT 0,
            amount DECIMAL(15,2) DEFAULT 0,
            status VARCHAR(50) DEFAULT 'pending',
            created_at TIMESTAMP DEFAULT NOW(),
            approved_at TIMESTAMP
        )
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        CREATE TABLE IF NOT EXISTS vacations (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id UUID NOT NULL,
            employee_id UUID NOT NULL,
            start_date DATE NOT NULL,
            end_date DATE NOT NULL,
            days_count INTEGER DEFAULT 0,
            amount DECIMAL(15,2) DEFAULT 0,
            status VARCHAR(50) DEFAULT 'pending',
            created_at TIMESTAMP DEFAULT NOW(),
            approved_at TIMESTAMP
        )
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        CREATE TABLE IF NOT EXISTS alimonies (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id UUID NOT NULL,
            employee_id UUID NOT NULL,
            children_count INTEGER DEFAULT 0,
            percent DECIMAL(5,2) DEFAULT 0,
            amount DECIMAL(15,2) DEFAULT 0,
            month INTEGER,
            year INTEGER,
            created_at TIMESTAMP DEFAULT NOW()
        )
    `)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message": "Таблицы ЗУП созданы",
        "tables":  []string{"sick_leaves", "vacations", "alimonies"},
    })
}

func AddEmployeeToPayroll(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var req struct {
        FirstName  string  `json:"first_name"`
        LastName   string  `json:"last_name"`
        Position   string  `json:"position"`
        Department string  `json:"department"`
        Salary     float64 `json:"salary"`
        Rate       float64 `json:"rate"`
        Email      string  `json:"email"`
        Phone      string  `json:"phone"`
        HireDate   string  `json:"hire_date"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    log.Printf("📝 Добавление сотрудника: firstName=%s, lastName=%s, position=%s, salary=%f, email=%s, hireDate=%s",
        req.FirstName, req.LastName, req.Position, req.Salary, req.Email, req.HireDate)

    if req.FirstName == "" || req.Position == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "first_name and position are required"})
        return
    }

    if req.Salary == 0 {
        req.Salary = 50000
    }
    if req.Rate == 0 {
        req.Rate = 100
    }

    var existingId string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT id FROM hr_employees WHERE email = $1 AND tenant_id = $2",
        req.Email, tenantID).Scan(&existingId)

    if err == nil {
        c.JSON(http.StatusConflict, gin.H{"error": "Employee with this email already exists"})
        return
    }

    var hireDateSQL interface{}
    if req.HireDate != "" && req.HireDate != "null" {
        hireDateSQL = req.HireDate
    } else {
        hireDateSQL = nil
    }

    var employeeID string
    err = database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO hr_employees (tenant_id, first_name, last_name, position, department, salary, rate, email, phone, hire_date, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'active', NOW())
        RETURNING id
    `, tenantID, req.FirstName, req.LastName, req.Position, req.Department, req.Salary, req.Rate, req.Email, req.Phone, hireDateSQL).Scan(&employeeID)

    if err != nil {
        log.Printf("❌ Ошибка добавления: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    log.Printf("✅ Сотрудник добавлен с ID: %s", employeeID)
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Сотрудник добавлен",
        "id":      employeeID,
    })
}

func UpdateEmployeeInPayroll(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    employeeID := c.Param("id")

    var req struct {
        FirstName  string  `json:"first_name"`
        LastName   string  `json:"last_name"`
        Position   string  `json:"position"`
        Department string  `json:"department"`
        Salary     float64 `json:"salary"`
        Rate       float64 `json:"rate"`
        Email      string  `json:"email"`
        Phone      string  `json:"phone"`
        HireDate   string  `json:"hire_date"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var hireDateSQL interface{}
    if req.HireDate != "" && req.HireDate != "null" {
        hireDateSQL = req.HireDate
    } else {
        hireDateSQL = nil
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE hr_employees 
        SET first_name = COALESCE($1, first_name),
            last_name = COALESCE($2, last_name),
            position = COALESCE($3, position),
            department = COALESCE($4, department),
            salary = COALESCE($5, salary),
            rate = COALESCE($6, rate),
            email = COALESCE($7, email),
            phone = COALESCE($8, phone),
            hire_date = COALESCE($9, hire_date),
            updated_at = NOW()
        WHERE id = $10 AND tenant_id = $11
    `, req.FirstName, req.LastName, req.Position, req.Department, req.Salary, req.Rate, req.Email, req.Phone, hireDateSQL, employeeID, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка обновления: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Сотрудник обновлён"})
}

func GenerateSimpleTaxReport(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var req struct {
        Month int `json:"month" binding:"required"`
        Year  int `json:"year" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var totalIncome, totalTax float64
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(SUM(salary), 0), COALESCE(SUM(tax), 0)
        FROM payroll
        WHERE tenant_id = $1 AND period_month = $2 AND period_year = $3
    `, tenantID, req.Month, req.Year).Scan(&totalIncome, &totalTax)

    if err != nil {
        log.Printf("⚠️ Ошибка получения данных: %v", err)
        totalIncome, totalTax = 0, 0
    }

    if totalIncome == 0 {
        err = database.Pool.QueryRow(c.Request.Context(), `
            SELECT COALESCE(SUM(salary), 0)
            FROM hr_employees
            WHERE tenant_id = $1 AND status = 'active'
        `, tenantID).Scan(&totalIncome)
        if err == nil {
            totalTax = totalIncome * 0.13
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "total_income": totalIncome,
        "total_tax":    totalTax,
        "month":        req.Month,
        "year":         req.Year,
        "message":      "Отчёт сформирован",
    })
}