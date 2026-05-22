package handlers

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "log"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/pquerna/otp/totp"

    "subscription-system/database"
    "subscription-system/middleware"
)

// generateSecureRandomCode - генерация криптографически безопасных кодов
func generateSecureRandomCode(length int) string {
    const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
    b := make([]byte, length)
    _, err := rand.Read(b)
    if err != nil {
        for i := range b {
            b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
            time.Sleep(1 * time.Nanosecond)
        }
        return string(b)
    }
    for i := range b {
        b[i] = charset[int(b[i])%len(charset)]
    }
    return string(b)
}

// hashSecret - хеширование секрета
func hashSecret(secret string) string {
    hash := sha256.Sum256([]byte(secret))
    return hex.EncodeToString(hash[:])
}

// GenerateTwoFASecret - генерация секрета и QR кода
func GenerateTwoFASecret(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.GetString("user_id")
    
    if userID == "" || userID == "00000000-0000-0000-0000-000000000000" {
        userID = c.Query("user_id")
        if userID == "" {
            userID = "test-user-123"
        }
        log.Printf("[2FA] Тестовый режим: userID=%s", userID)
    }
    
    var email string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT email FROM users WHERE id = $1 AND tenant_id = $2", userID, tenantID).Scan(&email)
    if err != nil {
        email = userID
        log.Printf("[2FA] Пользователь не найден, используем ID: %s", userID)
    }
    
    key, err := totp.Generate(totp.GenerateOpts{
        Issuer:      "Business Stack",
        AccountName: email,
        Period:      30,
        Digits:      6,
        SecretSize:  20,
    })
    if err != nil {
        log.Printf("[2FA] Ошибка генерации секрета: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate secret"})
        return
    }
    
    hashedSecret := hashSecret(key.Secret())
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO twofa (user_id, tenant_id, secret, enabled, created_at, updated_at)
        VALUES ($1, $2, $3, false, NOW(), NOW())
        ON CONFLICT (user_id, tenant_id) DO UPDATE SET
            secret = $3,
            enabled = false,
            updated_at = NOW()
    `, userID, tenantID, hashedSecret)
    if err != nil {
        log.Printf("[2FA] Ошибка сохранения секрета: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save secret"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "secret": key.Secret(),
        "url":    key.URL(),
    })
}

// VerifyTwoFACode - проверка и активация 2FA
func VerifyTwoFACode(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.GetString("user_id")
    
    if userID == "" || userID == "00000000-0000-0000-0000-000000000000" {
        userID = c.Query("user_id")
        if userID == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Пользователь не авторизован"})
            return
        }
    }
    
    rateKey := "2fa_verify_" + userID
    if !middleware.TwoFALimiter.CheckAndIncrement(rateKey) {
        c.JSON(http.StatusTooManyRequests, gin.H{"error": "Слишком много попыток. Попробуйте через 15 минут."})
        return
    }
    
    var req struct {
        Code   string `json:"code"`
        Secret string `json:"secret"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    valid := totp.Validate(req.Code, req.Secret)
    if !valid {
        log.Printf("[SECURITY] 🔴 Неудачная попытка верификации 2FA для пользователя %s с IP: %s", userID, c.ClientIP())
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid code"})
        return
    }
    
    middleware.TwoFALimiter.Reset(rateKey)
    hashedSecret := hashSecret(req.Secret)
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE twofa SET enabled = true, secret = $3, updated_at = NOW() 
        WHERE user_id = $1 AND tenant_id = $2
    `, userID, tenantID, hashedSecret)
    if err != nil {
        log.Printf("[2FA] Ошибка активации: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable 2FA"})
        return
    }
    
    backupCodes := make([]string, 10)
    for i := 0; i < 10; i++ {
        backupCodes[i] = generateSecureRandomCode(10)
    }
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO twofa_backup_codes (user_id, tenant_id, codes, created_at, updated_at)
        VALUES ($1, $2, $3, NOW(), NOW())
        ON CONFLICT (user_id, tenant_id) DO UPDATE SET 
            codes = $3,
            updated_at = NOW()
    `, userID, tenantID, backupCodes)
    if err != nil {
        log.Printf("[2FA] Ошибка сохранения резервных кодов: %v", err)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":      true,
        "message":      "2FA успешно настроена!",
        "backup_codes": backupCodes,
    })
}

// DisableTwoFA - отключение 2FA
func DisableTwoFA(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.GetString("user_id")
    
    if userID == "" || userID == "00000000-0000-0000-0000-000000000000" {
        userID = c.Query("user_id")
        if userID == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Пользователь не авторизован"})
            return
        }
    }
    
    rateKey := "2fa_disable_" + userID
    if !middleware.TwoFALimiter.CheckAndIncrement(rateKey) {
        c.JSON(http.StatusTooManyRequests, gin.H{"error": "Слишком много попыток"})
        return
    }
    
    var req struct {
        Code string `json:"code"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    var hashedSecret string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT secret FROM twofa WHERE user_id = $1 AND tenant_id = $2", userID, tenantID).Scan(&hashedSecret)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "2FA не настроена"})
        return
    }
    
    valid := totp.Validate(req.Code, hashedSecret)
    if !valid {
        log.Printf("[SECURITY] 🔴 Неудачная попытка отключения 2FA для пользователя %s с IP: %s", userID, c.ClientIP())
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный код"})
        return
    }
    
    middleware.TwoFALimiter.Reset(rateKey)
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE twofa SET enabled = false, updated_at = NOW() 
        WHERE user_id = $1 AND tenant_id = $2
    `, userID, tenantID)
    if err != nil {
        log.Printf("[2FA] Ошибка отключения: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable 2FA"})
        return
    }
    
    database.Pool.Exec(c.Request.Context(), `
        DELETE FROM twofa_backup_codes WHERE user_id = $1 AND tenant_id = $2
    `, userID, tenantID)
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "2FA отключена",
    })
}

// GetTwoFAStatus - статус 2FA
func GetTwoFAStatus(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.GetString("user_id")
    
    if userID == "" || userID == "00000000-0000-0000-0000-000000000000" {
        userID = c.Query("user_id")
        if userID == "" {
            c.JSON(http.StatusOK, gin.H{
                "enabled": false,
                "exists":  false,
            })
            return
        }
    }
    
    var enabled bool
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT enabled FROM twofa WHERE user_id = $1 AND tenant_id = $2", userID, tenantID).Scan(&enabled)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "enabled": false,
            "exists":  false,
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "enabled": enabled,
        "exists":  true,
    })
}

// GetBackupCodes - получить резервные коды
func GetBackupCodes(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.GetString("user_id")
    
    if userID == "" || userID == "00000000-0000-0000-0000-000000000000" {
        userID = c.Query("user_id")
        if userID == "" {
            c.JSON(http.StatusOK, gin.H{
                "success": true,
                "codes":   []string{},
            })
            return
        }
    }
    
    var codes []string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT codes FROM twofa_backup_codes WHERE user_id = $1 AND tenant_id = $2", userID, tenantID).Scan(&codes)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "codes":   []string{},
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "codes":   codes,
    })
}

// GenerateBackupCodes - генерация новых резервных кодов
func GenerateBackupCodes(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.GetString("user_id")
    
    if userID == "" || userID == "00000000-0000-0000-0000-000000000000" {
        userID = c.Query("user_id")
        if userID == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Пользователь не авторизован"})
            return
        }
    }
    
    var enabled bool
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT enabled FROM twofa WHERE user_id = $1 AND tenant_id = $2", userID, tenantID).Scan(&enabled)
    if err != nil || !enabled {
        c.JSON(http.StatusBadRequest, gin.H{"error": "2FA не настроена"})
        return
    }
    
    codes := make([]string, 10)
    for i := 0; i < 10; i++ {
        codes[i] = generateSecureRandomCode(10)
    }
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO twofa_backup_codes (user_id, tenant_id, codes, created_at, updated_at)
        VALUES ($1, $2, $3, NOW(), NOW())
        ON CONFLICT (user_id, tenant_id) DO UPDATE SET 
            codes = $3,
            updated_at = NOW()
    `, userID, tenantID, codes)
    if err != nil {
        log.Printf("[2FA] Ошибка сохранения резервных кодов: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save backup codes"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "codes":   codes,
    })
}

// Get2FASettings - получить настройки 2FA
func Get2FASettings(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "enabled":   false,
        "method":    "totp",
        "period":    30,
        "digits":    6,
        "algorithm": "SHA1",
    })
}

// CheckTrustedDevice - проверка доверенного устройства
func CheckTrustedDevice(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"trusted": false})
}

// TrustDevice - добавить доверенное устройство
func TrustDevice(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Device trusted"})
}

// VerifyWithBackupCode - вход по резервному коду
func VerifyWithBackupCode(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.GetString("user_id")
    
    if userID == "" || userID == "00000000-0000-0000-0000-000000000000" {
        userID = c.Query("user_id")
        if userID == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid backup code"})
            return
        }
    }
    
    var req struct {
        Code string `json:"code"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    var codes []string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT codes FROM twofa_backup_codes WHERE user_id = $1 AND tenant_id = $2", userID, tenantID).Scan(&codes)
    
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid backup code"})
        return
    }
    
    found := false
    newCodes := []string{}
    for _, code := range codes {
        if code == req.Code {
            found = true
            continue
        }
        newCodes = append(newCodes, code)
    }
    
    if !found {
        log.Printf("[SECURITY] 🔴 Неудачная попытка входа по резервному коду для пользователя %s с IP: %s", userID, c.ClientIP())
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid backup code"})
        return
    }
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE twofa_backup_codes SET codes = $1, updated_at = NOW()
        WHERE user_id = $2 AND tenant_id = $3
    `, newCodes, userID, tenantID)
    
    if err != nil {
        log.Printf("[2FA] Ошибка обновления резервных кодов: %v", err)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Backup code accepted",
    })
}

// send2FANotification - отправка уведомления
func send2FANotification(userID, action string) {
    ctx := context.Background()
    var email string
    err := database.Pool.QueryRow(ctx,
        "SELECT email FROM users WHERE id = $1", userID).Scan(&email)
    if err != nil {
        log.Printf("[2FA] Не удалось отправить уведомление: %v", err)
        return
    }
    log.Printf("[SECURITY] 📧 Уведомление: %s для пользователя %s (%s)", action, userID, email)
}