
package handlers

import (
    "net/http"
    "strings"
    "subscription-system/database"
    "subscription-system/models"

    "github.com/gin-gonic/gin"
)

func ProfilePageHandler(c *gin.Context) {
    userID, exists := c.Get("userID")
    if !exists {
        var id string
        rows, err := database.Pool.Query(c.Request.Context(), "SELECT id FROM users ORDER BY created_at LIMIT 1")
        if err == nil && rows.Next() {
            rows.Scan(&id)
            userID = id
        }
        if rows != nil { rows.Close() }
        if userID == nil || userID == "" {
            c.HTML(http.StatusOK, "profile.html", gin.H{
                "Title":    "Мой профиль - Business Stack",
                "Version":  "3.0",
                "User":     nil,
                "Initials": "?",
                "Error":    "Пользователь не найден",
            })
            return
        }
    }

    user, err := models.GetUserByID(userID.(string))
    if err != nil {
        c.HTML(http.StatusOK, "profile.html", gin.H{
            "Title":    "Мой профиль - Business Stack",
            "Version":  "3.0",
            "User":     nil,
            "Initials": "?",
            "Error":    "Пользователь не найден",
        })
        return
    }

    initials := ""
    if user.Name != "" {
        parts := strings.Fields(user.Name)
        if len(parts) > 0 {
            initials = strings.ToUpper(string(parts[0][0]))
            if len(parts) > 1 {
                initials += strings.ToUpper(string(parts[1][0]))
            }
        }
    }
    if initials == "" && user.Email != "" {
        initials = strings.ToUpper(string(user.Email[0]))
    }
    if initials == "" {
        initials = "U"
    }

    c.HTML(http.StatusOK, "profile.html", gin.H{
        "Title":    "Мой профиль - Business Stack",
        "Version":  "3.0",
        "User":     user,
        "Initials": initials,
    })
}

type UpdateProfileRequest struct {
    Name             string `json:"name" binding:"required"`
    Email            string `json:"email" binding:"required,email"`
    Phone            string `json:"phone"`
    OrganizationName string `json:"organization_name"`
    OrganizationInn  string `json:"organization_inn"`
}

func UpdateProfileHandler(c *gin.Context) {
    // Используем правильный ключ - user_id (как в middleware)
    userID := c.GetString("user_id")
    
    if userID == "" {
        // fallback для совместимости
        if id, exists := c.Get("userID"); exists {
            if str, ok := id.(string); ok {
                userID = str
            }
        }
    }
    
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": "unauthorized - user not found in context",
            "debug": "user_id not set",
        })
        return
    }
    
    var req UpdateProfileRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    updates := map[string]interface{}{
        "name":  req.Name,
        "email": req.Email,
    }
    if req.Phone != "" {
        updates["phone"] = req.Phone
    }
    if req.OrganizationName != "" {
        updates["organization_name"] = req.OrganizationName
    }
    if req.OrganizationInn != "" {
        updates["organization_inn"] = req.OrganizationInn
    }
    
    err := models.UpdateUser(userID, updates)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile: " + err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "message": "profile updated successfully",
        "user_id": userID,
    })
}
type UpdatePasswordRequest struct {
    OldPassword string `json:"old_password" binding:"required"`
    NewPassword string `json:"new_password" binding:"required,min=6"`
}

func UpdatePasswordHandler(c *gin.Context) {
    // ===== ИСПРАВЛЕНО: используем c.GetString("user_id") =====
    userID := c.GetString("user_id")
    if userID == "" {
        // fallback для совместимости
        if id, exists := c.Get("userID"); exists {
            if str, ok := id.(string); ok {
                userID = str
            }
        }
    }
    
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized - user not found"})
        return
    }
    
    var req UpdatePasswordRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    user, err := models.GetUserByID(userID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
        return
    }
    
    if !models.CheckPasswordHash(req.OldPassword, user.PasswordHash) {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid old password"})
        return
    }
    
    hashedPassword, err := models.HashPassword(req.NewPassword)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
        return
    }
    
    err = models.UpdatePassword(userID, hashedPassword)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"message": "password updated successfully"})
}