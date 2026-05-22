package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/pquerna/otp/totp"
    "subscription-system/database"
)

// AdminRequire2FA - middleware для проверки 2FA у админов
func AdminRequire2FA() gin.HandlerFunc {
    return func(c *gin.Context) {
        userRole, _ := c.Get("user_role")
        userID, _ := c.Get("user_id")
        
        // Проверяем, если это админ
        if userRole == "admin" {
            var twofaEnabled bool
            err := database.Pool.QueryRow(c, `
                SELECT enabled FROM twofa WHERE user_id = $1
            `, userID).Scan(&twofaEnabled)
            
            if err == nil && twofaEnabled {
                // Проверяем, прошла ли 2FA в этой сессии
                twofaVerified, _ := c.Get("twofa_verified")
                if twofaVerified != true {
                    c.JSON(http.StatusUnauthorized, gin.H{
                        "error": "2FA required",
                        "code":  "2FA_REQUIRED",
                    })
                    c.Abort()
                    return
                }
            }
        }
        c.Next()
    }
}

// EnableAdmin2FA - включение 2FA для админа
func EnableAdmin2FA(c *gin.Context) {
    userID, _ := c.Get("user_id")
    
    // Генерируем секрет
    key, err := totp.Generate(totp.GenerateOpts{
        Issuer:      "Business Stack",
        AccountName: c.GetString("user_email"),
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate 2FA secret"})
        return
    }
    
    // Сохраняем в базу
    _, err = database.Pool.Exec(c, `
        INSERT INTO twofa (user_id, secret, enabled) 
        VALUES ($1, $2, false)
        ON CONFLICT (user_id) DO UPDATE SET secret = $2
    `, userID, key.Secret())
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save 2FA secret"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "secret": key.Secret(),
        "qr_code": key.URL(),
    })
}

// VerifyAdmin2FA - верификация 2FA кода
func VerifyAdmin2FA(c *gin.Context) {
    var req struct {
        Code string `json:"code"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
        return
    }
    
    userID, _ := c.Get("user_id")
    
    var secret string
    err := database.Pool.QueryRow(c, `
        SELECT secret FROM twofa WHERE user_id = $1
    `, userID).Scan(&secret)
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "2FA not configured"})
        return
    }
    
    // Проверяем код
    if totp.Validate(req.Code, secret) {
        // Помечаем сессию как проверенную
        c.Set("twofa_verified", true)
        
              
        c.JSON(http.StatusOK, gin.H{"success": true})
    } else {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid code"})
    }
}

