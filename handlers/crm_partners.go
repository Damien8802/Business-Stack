package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "subscription-system/database"
)

// Partner структура партнёра
type Partner struct {
    ID        string    `json:"id"`
    TenantID  string    `json:"tenant_id"`
    Name      string    `json:"name"`
    INN       string    `json:"inn"`
    Phone     string    `json:"phone"`
    Email     string    `json:"email"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
}

// GetPartners - получение списка партнёров
func GetPartners(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, COALESCE(inn, '') as inn, 
               COALESCE(phone, '') as phone, 
               COALESCE(email, '') as email,
               status, created_at
        FROM crm_partners
        WHERE tenant_id = $1
        ORDER BY name
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var partners []gin.H
    for rows.Next() {
        var id, name, inn, phone, email, status string
        var createdAt time.Time
        err := rows.Scan(&id, &name, &inn, &phone, &email, &status, &createdAt)
        if err != nil {
            continue
        }
        partners = append(partners, gin.H{
            "id":         id,
            "name":       name,
            "inn":        inn,
            "phone":      phone,
            "email":      email,
            "status":     status,
            "created_at": createdAt,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    partners,
        "count":   len(partners),
    })
}

// CreatePartner - создание партнёра
func CreatePartner(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }

    var req struct {
        Name  string `json:"name"`
        INN   string `json:"inn"`
        Phone string `json:"phone"`
        Email string `json:"email"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if req.Name == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Название партнёра обязательно"})
        return
    }

    id := uuid.New().String()
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO crm_partners (id, tenant_id, name, inn, phone, email, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, NOW())
    `, id, tenantID, req.Name, req.INN, req.Phone, req.Email)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusCreated, gin.H{
        "success": true,
        "id":      id,
        "message": "Партнёр успешно создан",
    })
}