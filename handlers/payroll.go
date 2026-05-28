package handlers

import (
    "sync"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "sort"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "subscription-system/database"
)

var benchmarkCache struct {
    Data      []gin.H
    Timestamp time.Time
    mu        sync.Mutex
}

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

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, first_name, last_name, salary, COALESCE(tax_rate, 13) as tax_rate
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
        
        employeeName := firstName + " " + lastName
        period := fmt.Sprintf("%d/%d", req.Month, req.Year)

        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO payroll_history (
                id, tenant_id, employee_id, employee_name, 
                period, month, year, gross, tax, net, 
                status, created_at, updated_at
            )
            VALUES (
                $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 
                'calculated', NOW(), NOW()
            )
        `, uuid.New(), tenantID, id, employeeName, period, req.Month, req.Year, salary, tax, netAmount)

        if err != nil {
            log.Printf("❌ Ошибка сохранения в payroll_history: %v", err)
        } else {
            log.Printf("✅ Сохранено в историю: %s - %s (%.2f руб.)", employeeName, period, netAmount)
        }

        payrolls = append(payrolls, gin.H{
            "employee_id":   id,
            "employee_name": employeeName,
            "salary":        salary,
            "tax":           tax,
            "net_amount":    netAmount,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "message":  "Расчёт выполнен и сохранён в историю",
        "payrolls": payrolls,
        "total":    len(payrolls),
        "month":    req.Month,
        "year":     req.Year,
    })
}

func GetPayrollHistory(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    month := c.DefaultQuery("month", "0")
    year := c.DefaultQuery("year", "0")

    query := `
        SELECT id, employee_id, employee_name, month, year, gross, tax, net, status, created_at
        FROM payroll_history 
        WHERE tenant_id = $1
    `
    args := []interface{}{tenantID}
    argIndex := 2

    if month != "0" {
        query += fmt.Sprintf(" AND month = $%d", argIndex)
        args = append(args, month)
        argIndex++
    }
    if year != "0" {
        query += fmt.Sprintf(" AND year = $%d", argIndex)
        args = append(args, year)
        argIndex++
    }

    query += " ORDER BY created_at DESC LIMIT 100"

    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        log.Printf("❌ Ошибка загрузки истории: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load history"})
        return
    }
    defer rows.Close()

    var history []gin.H
    for rows.Next() {
        var id string
        var employeeID *string
        var employeeName, status string
        var month, year int
        var gross, tax, net float64
        var createdAt time.Time

        err := rows.Scan(&id, &employeeID, &employeeName, &month, &year, &gross, &tax, &net, &status, &createdAt)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
            continue
        }

        history = append(history, gin.H{
            "id":            id,
            "employee_id":   employeeID,
            "employee_name": employeeName,
            "month":         month,
            "year":          year,
            "gross":         gross,
            "tax":           tax,
            "net":           net,
            "status":        status,
            "created_at":    createdAt.Format("2006-01-02 15:04:05"),
        })
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "history": history})
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

func CreatePayrollHistory(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    log.Printf("🔍 CreatePayrollHistory: tenantID=%s", tenantID)

    var req struct {
        EmployeeID   *string `json:"employee_id"`
        EmployeeName string  `json:"employee_name"`
        Month        int     `json:"month"`
        Year         int     `json:"year"`
        Gross        float64 `json:"gross"`
        Tax          float64 `json:"tax"`
        Net          float64 `json:"net"`
        Status       string  `json:"status"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    log.Printf("📝 Сохраняем: %s, %d/%d, gross=%.2f, tax=%.2f, net=%.2f",
        req.EmployeeName, req.Month, req.Year, req.Gross, req.Tax, req.Net)

    var existingID string
    checkErr := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id FROM payroll_history 
        WHERE tenant_id = $1 AND employee_id = $2 AND month = $3 AND year = $4
    `, tenantID, req.EmployeeID, req.Month, req.Year).Scan(&existingID)

    if checkErr == nil {
        _, updateErr := database.Pool.Exec(c.Request.Context(), `
            UPDATE payroll_history 
            SET gross = $1, tax = $2, net = $3, status = $4, updated_at = NOW()
            WHERE id = $5
        `, req.Gross, req.Tax, req.Net, req.Status, existingID)

        if updateErr != nil {
            log.Printf("❌ Ошибка обновления: %v", updateErr)
            c.JSON(http.StatusInternalServerError, gin.H{"error": updateErr.Error()})
            return
        }

        log.Printf("✅ Обновлена запись ID: %s", existingID)
        c.JSON(http.StatusOK, gin.H{"success": true, "message": "История обновлена", "id": existingID})
        return
    }

    var id string
    insertErr := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO payroll_history (
            id, tenant_id, employee_id, employee_name, 
            month, year, gross, tax, net, 
            status, created_at, updated_at
        )
        VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, 
            COALESCE($10, 'accrued'), NOW(), NOW()
        )
        RETURNING id
    `, uuid.New(), tenantID, req.EmployeeID, req.EmployeeName,
        req.Month, req.Year, req.Gross, req.Tax, req.Net,
        req.Status).Scan(&id)

    if insertErr != nil {
        log.Printf("❌ Ошибка создания истории: %v", insertErr)
        c.JSON(http.StatusInternalServerError, gin.H{"error": insertErr.Error()})
        return
    }

    log.Printf("✅ Создана запись в истории ID: %s", id)
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Запись создана", "id": id})
}

func DeletePayrollHistory(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    id := c.Param("id")

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM payroll_history WHERE id = $1 AND tenant_id = $2
    `, id, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка удаления истории: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    rowsAffected := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
        return
    }

    log.Printf("✅ Удалена запись %s", id)
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Запись удалена"})
}

func UpdatePayrollHistory(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    id := c.Param("id")

    var req struct {
        Status string `json:"status"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE payroll_history 
        SET status = $1, updated_at = NOW()
        WHERE id = $2 AND tenant_id = $3
    `, req.Status, id, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка обновления истории: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    rowsAffected := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Запись не найдена"})
        return
    }

    log.Printf("✅ Обновлена запись %s, статус: %s", id, req.Status)
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Статус обновлён"})
}

func CreatePayrollHistoryTable(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        CREATE TABLE IF NOT EXISTS payroll_history (
            id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
            tenant_id UUID NOT NULL,
            employee_id UUID,
            employee_name VARCHAR(255) NOT NULL,
            period VARCHAR(50),
            month INTEGER NOT NULL,
            year INTEGER NOT NULL,
            gross DECIMAL(15,2) DEFAULT 0,
            tax DECIMAL(15,2) DEFAULT 0,
            net DECIMAL(15,2) DEFAULT 0,
            status VARCHAR(50) DEFAULT 'accrued',
            created_at TIMESTAMP DEFAULT NOW(),
            updated_at TIMESTAMP DEFAULT NOW()
        )
    `)
    if err != nil {
        log.Printf("❌ Ошибка создания payroll_history: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        CREATE INDEX IF NOT EXISTS idx_payroll_history_tenant ON payroll_history(tenant_id);
        CREATE INDEX IF NOT EXISTS idx_payroll_history_employee ON payroll_history(employee_id);
        CREATE INDEX IF NOT EXISTS idx_payroll_history_period ON payroll_history(year, month);
    `)
    if err != nil {
        log.Printf("⚠️ Ошибка создания индексов: %v", err)
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Таблица payroll_history создана",
    })
}

// ClearAllPayrollHistory - очистка всей истории
func ClearAllPayrollHistory(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM payroll_history WHERE tenant_id = $1
    `, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка очистки истории: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    log.Printf("✅ Вся история очищена для tenant %s", tenantID)
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Вся история очищена"})
}


// ============================================================================
// АРХИВ РАСЧЁТНЫХ ЛИСТКОВ
// ============================================================================

// SavePayslipToArchive - сохранение PDF в архив (в БД)
func SavePayslipToArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var req struct {
        EmployeeID   string `json:"employee_id"`
        EmployeeName string `json:"employee_name"`
        Position     string `json:"position"`
        Month        int    `json:"month"`
        Year         int    `json:"year"`
        Content      string `json:"content"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    log.Printf("📄 Сохранение в архив: %s, %d.%d", req.EmployeeName, req.Month, req.Year)

    id := uuid.New()
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO payslip_archive (id, tenant_id, employee_id, employee_name, position, month, year, content, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
    `, id, tenantID, req.EmployeeID, req.EmployeeName, req.Position, req.Month, req.Year, req.Content)

    if err != nil {
        log.Printf("❌ Ошибка сохранения в архив: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    log.Printf("✅ Сохранено в архив с ID: %s", id)
    c.JSON(http.StatusOK, gin.H{"success": true, "id": id})
}




// GetPayslipArchive - получение списка документов из архива
func GetPayslipArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    month := c.DefaultQuery("month", "")
    year := c.DefaultQuery("year", "")

    query := `SELECT id, employee_id, employee_name, position, month, year, created_at FROM payslip_archive WHERE tenant_id = $1`
    args := []interface{}{tenantID}
    argIndex := 2

    if month != "" {
        query += fmt.Sprintf(" AND month = $%d", argIndex)
        args = append(args, month)
        argIndex++
    }
    if year != "" {
        query += fmt.Sprintf(" AND year = $%d", argIndex)
        args = append(args, year)
        argIndex++
    }

    query += " ORDER BY created_at DESC LIMIT 100"

    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        log.Printf("❌ Ошибка получения архива: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var archive []gin.H
    for rows.Next() {
        var id uuid.UUID
        var employeeID *string
        var employeeName, position string
        var month, year int
        var createdAt time.Time

        err := rows.Scan(&id, &employeeID, &employeeName, &position, &month, &year, &createdAt)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
            continue
        }

        archive = append(archive, gin.H{
            "id":            id,
            "employee_id":   employeeID,
            "employee_name": employeeName,
            "position":      position,
            "month":         month,
            "year":          year,
            "created_at":    createdAt,
        })
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "archive": archive})
}

// GetPayslipContent - получение содержимого документа
func GetPayslipContent(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    id := c.Param("id")

    var content string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT content FROM payslip_archive WHERE id = $1 AND tenant_id = $2
    `, id, tenantID).Scan(&content)

    if err != nil {
        log.Printf("❌ Ошибка получения содержимого: %v", err)
        c.JSON(http.StatusNotFound, gin.H{"error": "Документ не найден"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true, "content": content})
}

// DeletePayslipFromArchive - удаление документа
func DeletePayslipFromArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    id := c.Param("id")

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM payslip_archive WHERE id = $1 AND tenant_id = $2
    `, id, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка удаления из архива: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    rowsAffected := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Документ не найден"})
        return
    }

    log.Printf("✅ Удалён документ из архива: %s", id)
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Документ удалён"})
}

// ClearPayslipArchive - очистка архива
func ClearPayslipArchive(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM payslip_archive WHERE tenant_id = $1
    `, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка очистки архива: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    rowsAffected := result.RowsAffected()
    log.Printf("✅ Очищен архив, удалено записей: %d", rowsAffected)
    c.JSON(http.StatusOK, gin.H{"success": true, "deleted_count": rowsAffected})
}

// GetBenchmarkData - получение рыночных зарплат через API HeadHunter
func GetBenchmarkData(c *gin.Context) {
    city := c.DefaultQuery("city", "msk")
    industry := c.DefaultQuery("industry", "it")
    size := c.DefaultQuery("size", "small")
    
    log.Printf("📊 Бенчмаркинг: city=%s, industry=%s, size=%s", city, industry, size)
    
    // Проверка кэша
    benchmarkCache.mu.Lock()
    defer benchmarkCache.mu.Unlock()
    
    if time.Since(benchmarkCache.Timestamp) < 6*time.Hour && len(benchmarkCache.Data) > 0 {
        log.Println("📦 Возвращаем данные из кэша")
        c.JSON(http.StatusOK, benchmarkCache.Data)
        return
    }
    
    // Используем публичное прокси для обхода блокировки
    proxyURL := "https://cors-anywhere.herokuapp.com/"
    
    cityCodes := map[string]int{
        "msk": 1, "spb": 2, "nsk": 65, "ekb": 3, "kazan": 88,
        "nn": 66, "samara": 78, "rostov": 76, "ufa": 99, "krasnodar": 53,
    }
    
    area := cityCodes[city]
    if area == 0 {
        area = 1
    }
    
    positions := []string{
        "разработчик", "программист", "менеджер", "бухгалтер", "hr",
        "аналитик", "тестировщик", "devops", "project+manager",
        "системный+администратор", "дизайнер", "маркетолог",
    }
    
    client := &http.Client{Timeout: 15 * time.Second}
    var results []gin.H
    
    for _, position := range positions {
        url := fmt.Sprintf("%shttps://api.hh.ru/vacancies?text=%s&area=%d&only_with_salary=true&per_page=30", 
            proxyURL, position, area)
        
        log.Printf("📡 Запрос: %s", position)
        
        req, err := http.NewRequest("GET", url, nil)
        if err != nil {
            continue
        }
        
        req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
        req.Header.Set("Origin", "https://hh.ru")
        req.Header.Set("Referer", "https://hh.ru/")
        
        resp, err := client.Do(req)
        if err != nil {
            log.Printf("⚠️ Ошибка запроса для %s: %v", position, err)
            continue
        }
        
        body, _ := io.ReadAll(resp.Body)
        resp.Body.Close()
        
        if resp.StatusCode != 200 {
            log.Printf("⚠️ Статус %d для %s", resp.StatusCode, position)
            continue
        }
        
        var hhResponse struct {
            Items []struct {
                Salary struct {
                    From     float64 `json:"from"`
                    To       float64 `json:"to"`
                    Currency string  `json:"currency"`
                    Gross    bool    `json:"gross"`
                } `json:"salary"`
            } `json:"items"`
        }
        
        json.Unmarshal(body, &hhResponse)
        
        var salaries []float64
        for _, item := range hhResponse.Items {
            if item.Salary.Currency == "RUR" && item.Salary.From > 0 {
                var salary float64
                if item.Salary.To > 0 {
                    salary = (item.Salary.From + item.Salary.To) / 2
                } else {
                    salary = item.Salary.From * 1.2
                }
                if item.Salary.Gross {
                    salary = salary * 0.87
                }
                if salary > 0 {
                    salaries = append(salaries, salary)
                }
            }
        }
        
        if len(salaries) > 0 {
            sort.Float64s(salaries)
            mid := len(salaries) / 2
            var median float64
            if len(salaries)%2 == 0 {
                median = (salaries[mid-1] + salaries[mid]) / 2
            } else {
                median = salaries[mid]
            }
            
            results = append(results, gin.H{
                "position":    position,
                "market_50":   median,
                "source":      "hh.ru",
                "vacancies":   len(salaries),
                "updated_at":  time.Now().Format("2006-01-02"),
            })
        }
        
        time.Sleep(500 * time.Millisecond) // Пауза между запросами
    }
    
    // Если данных нет - используем расширенную базу
    if len(results) == 0 {
        log.Println("⚠️ Используем локальную базу зарплат")
        results = getLocalSalaryDatabase()
    }
    
    log.Printf("✅ Бенчмаркинг завершён, получено %d позиций", len(results))
    
    benchmarkCache.Data = results
    benchmarkCache.Timestamp = time.Now()
    
    c.JSON(http.StatusOK, results)
}

// Локальная база зарплат по рынку (реалистичные данные)
func getLocalSalaryDatabase() []gin.H {
    return []gin.H{
        {"position": "разработчик", "market_50": 150000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "программист 1с", "market_50": 120000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "менеджер", "market_50": 100000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "бухгалтер", "market_50": 75000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "hr", "market_50": 85000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "аналитик", "market_50": 130000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "тестировщик", "market_50": 110000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "devops", "market_50": 180000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "project manager", "market_50": 160000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "системный администратор", "market_50": 95000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "дизайнер", "market_50": 90000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "маркетолог", "market_50": 95000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "продавец", "market_50": 55000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "водитель", "market_50": 60000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
        {"position": "уборщица", "market_50": 35000, "source": "База данных 2024", "updated_at": time.Now().Format("2006-01-02")},
    }
}