package handlers

import (
    "encoding/csv"
    "fmt"
    "io"
    "net/http"
    "strings"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)

func ImportEmployeesFromExcel(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    file, _, err := c.Request.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Файл не загружен"})
        return
    }
    defer file.Close()
    
    reader := csv.NewReader(file)
    reader.Comma = ';'
    
    imported := 0
    errors := []string{}
    
    for {
        record, err := reader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            errors = append(errors, err.Error())
            continue
        }
        
        if len(record) < 3 {
            continue
        }
        
        fullName := strings.TrimSpace(record[0])
        position := strings.TrimSpace(record[1])
        var salary float64
        fmt.Sscanf(strings.TrimSpace(record[2]), "%f", &salary)
        
        if fullName == "" || position == "" || salary == 0 {
            continue
        }
        
        employeeID := uuid.New()
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO employees (id, tenant_id, full_name, position, salary, status, created_at)
            VALUES ($1, $2, $3, $4, $5, 'active', NOW())
            ON CONFLICT (id) DO NOTHING
        `, employeeID, tenantID, fullName, position, salary)
        
        if err == nil {
            imported++
        }
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":   true,
        "imported":  imported,
        "errors":    errors,
        "message":   fmt.Sprintf("Импортировано %d сотрудников", imported),
    })
}