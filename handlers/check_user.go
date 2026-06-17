package handlers

import (
    "github.com/gin-gonic/gin"
)

func GetCurrentUserID(c *gin.Context) {
    userID := c.GetString("user_id")
    userEmail := c.GetString("user_email")
    userName := c.GetString("user_name")
    userRole := c.GetString("role")
    tenantID := c.GetString("tenant_id")  // ← ДОБАВИТЬ ЭТУ СТРОКУ
    
    // Если роль не установлена - пробуем получить из контекста
    if userRole == "" {
        userRole = c.GetString("role")
    }
    
    // ПРИНУДИТЕЛЬНАЯ УСТАНОВКА ДЛЯ ВЛАДЕЛЬЦА
    if userEmail == "dev@businesstack.ru" {
        userRole = "owner"
        if userName == "" || userName == "user" {
            userName = "Максим Владелец"
        }
    }
    
    // Конвертируем userID в строку, если это UUID
    userIDStr := userID
    if userID == "" {
        userIDStr = "00000000-0000-0000-0000-000000000000"
    }
    
    // Если tenantID пустой, пробуем получить как tenant_id_string
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    
    c.JSON(200, gin.H{
        "user_id":       userIDStr,
        "email":         userEmail,
        "name":          userName,
        "role":          userRole,
        "tenant_id":     tenantID,  // ← ДОБАВИТЬ ЭТУ СТРОКУ
        "is_admin":      userRole == "admin" || userRole == "owner" || userRole == "developer",
        "is_owner":      userRole == "owner",
        "is_developer":  userRole == "developer" || userRole == "owner",
    })
}