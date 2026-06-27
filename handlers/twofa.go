package handlers

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    
    "log"
    "net/http"
    "time"
     "fmt"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/pquerna/otp/totp"
     "subscription-system/utils"

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
// GenerateTwoFASecret - генерация секрета и QR кода
func GenerateTwoFASecret(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    if tenantID == uuid.Nil {
        log.Printf("❌ tenant_id не найден в контексте")
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id not found"})
        return
    }

    userID := c.GetString("user_id")
    if userID == "" {
        log.Printf("❌ user_id не найден в контексте")
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
        return
    }

    log.Printf("[2FA] Генерация секрета для userID=%s, tenantID=%s", userID, tenantID.String())

    var email string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT email FROM users WHERE id = $1 AND tenant_id = $2", userID, tenantID).Scan(&email)
    if err != nil {
        log.Printf("[2FA] Ошибка получения email: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
        return
    }

    // ✅ ПРОВЕРЯЕМ, ЕСТЬ ЛИ УЖЕ СЕКРЕТ
    var existingRawSecret string
    var existingEnabled bool
    err = database.Pool.QueryRow(c.Request.Context(),
        "SELECT raw_secret, enabled FROM twofa WHERE user_id = $1 AND tenant_id = $2",
        userID, tenantID).Scan(&existingRawSecret, &existingEnabled)

    // ✅ ЕСЛИ СЕКРЕТ УЖЕ ЕСТЬ - ИСПОЛЬЗУЕМ ЕГО
    if err == nil && existingRawSecret != "" {
        log.Printf("[2FA] Используем существующий секрет для пользователя %s", userID)
        
        // Генерируем URL для QR-кода с существующим секретом
        qrURL := fmt.Sprintf("otpauth://totp/Business%%20Stack:%s?secret=%s&issuer=Business%%20Stack", 
            email, existingRawSecret)
        
        c.JSON(http.StatusOK, gin.H{
            "secret": existingRawSecret,
            "url":    qrURL,
            "exists": true,
        })
        return
    }

    // ✅ ЕСЛИ СЕКРЕТА НЕТ - СОЗДАЕМ НОВЫЙ
    log.Printf("[2FA] Создаем новый секрет для пользователя %s", userID)

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
        INSERT INTO twofa (user_id, tenant_id, secret, raw_secret, enabled, created_at, updated_at)
        VALUES ($1, $2, $3, $4, false, NOW(), NOW())
        ON CONFLICT (user_id, tenant_id) DO UPDATE SET
            secret = $3,
            raw_secret = $4,
            enabled = false,
            updated_at = NOW()
    `, userID, tenantID, hashedSecret, key.Secret())
    if err != nil {
        log.Printf("[2FA] Ошибка сохранения секрета: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save secret"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "secret": key.Secret(),
        "url":    key.URL(),
        "exists": false,
    })
}
// VerifyTwoFACode - проверка и активация 2FA
// VerifyTwoFACode - проверка и активация 2FA
func VerifyTwoFACode(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    if tenantID == uuid.Nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id not found"})
        return
    }

    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
        return
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
    
    // ✅ СОХРАНЯЕМ ПРАВИЛЬНО!
    hashedSecret := hashSecret(req.Secret)
    rawSecret := req.Secret

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE twofa SET 
            enabled = true, 
            secret = $3, 
            raw_secret = $4,
            updated_at = NOW() 
        WHERE user_id = $1 AND tenant_id = $2
    `, userID, tenantID, hashedSecret, rawSecret)
    if err != nil {
        log.Printf("[2FA] Ошибка активации: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable 2FA"})
        return
    }

    // ✅ АКТИВИРУЕМ СКИДКУ ЗА ВКЛЮЧЕНИЕ 2FA
    discountID, err := ActivateTwoFADiscount(userID, tenantID)
    if err != nil {
        log.Printf("[2FA] Ошибка активации скидки: %v", err)
        // Не возвращаем ошибку, чтобы не ломать процесс
    }

    // ✅ СОЗДАЕМ РЕЗЕРВНЫЕ КОДЫ
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

    // ✅ ФОРМИРУЕМ ОТВЕТ С ИНФОРМАЦИЕЙ О СКИДКЕ
    response := gin.H{
        "success":      true,
        "message":      "2FA успешно настроена!",
        "backup_codes": backupCodes,
    }

    if discountID > 0 {
        response["discount_info"] = gin.H{
            "percent":      5,
            "valid_from":   time.Now().AddDate(0, 0, 1).Format("2006-01-02"),
            "valid_to":     time.Now().AddDate(0, 1, 0).Format("2006-01-02"),
            "description":  "Скидка 5% на следующий месяц за включение 2FA",
            "discount_id":  discountID,
        }
    }

    c.JSON(http.StatusOK, response)
}
func DisableTwoFA(c *gin.Context) {
    var req struct {
        UserID string `json:"user_id"`
        Code   string `json:"code"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат запроса: " + err.Error()})
        return
    }

    userID := req.UserID
    if userID == "" {
        userID = c.GetString("user_id")
    }

    if userID == "" {
        log.Printf("❌ user_id не найден")
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
        return
    }

    tenantID := middleware.GetTenantIDFromContext(c)
    if tenantID == uuid.Nil {
        log.Printf("❌ tenant_id не найден в контексте")
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id not found"})
        return
    }

    log.Printf("🔍 ========================================")
    log.Printf("🔍 DisableTwoFA: userID=%s, tenantID=%s", userID, tenantID.String())
    log.Printf("🔍 Введенный код: '%s', длина: %d", req.Code, len(req.Code))

    if req.Code == "" || len(req.Code) < 6 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Введите код"})
        return
    }

    rateKey := "2fa_disable_" + userID
    if !middleware.TwoFALimiter.CheckAndIncrement(rateKey) {
        c.JSON(http.StatusTooManyRequests, gin.H{"error": "Слишком много попыток"})
        return
    }

    // ✅ БЕРЕМ ОРИГИНАЛЬНЫЙ СЕКРЕТ
    var rawSecret string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT raw_secret FROM twofa WHERE user_id = $1 AND tenant_id = $2",
        userID, tenantID).Scan(&rawSecret)
    if err != nil {
        log.Printf("❌ Ошибка получения raw_secret: %v", err)
        c.JSON(http.StatusNotFound, gin.H{"error": "2FA не настроена или raw_secret отсутствует"})
        return
    }

    log.Printf("🔍 raw_secret из БД: '%s...' (длина: %d)", rawSecret[:10], len(rawSecret))

    // ✅ ПРОВЕРЯЕМ КОД ИЗ GOOGLE AUTHENTICATOR
    valid := false
    if len(req.Code) == 6 {
        // Пробуем с текущим временем
        valid = totp.Validate(req.Code, rawSecret)
        log.Printf("🔍 TOTP проверка (текущее время): %v", valid)
        
        if !valid {
            // Пробуем с предыдущим периодом (30 сек назад)
            valid, _ = totp.ValidateCustom(req.Code, rawSecret, time.Now().Add(-30*time.Second), totp.ValidateOpts{
                Period: 30,
                Digits: 6,
            })
            log.Printf("🔍 TOTP проверка (-30 сек): %v", valid)
        }
        
        if !valid {
            // Пробуем со следующим периодом (30 сек вперед)
            valid, _ = totp.ValidateCustom(req.Code, rawSecret, time.Now().Add(30*time.Second), totp.ValidateOpts{
                Period: 30,
                Digits: 6,
            })
            log.Printf("🔍 TOTP проверка (+30 сек): %v", valid)
        }
        
        // Пробуем с разными допусками (SKEW)
        if !valid {
            valid, _ = totp.ValidateCustom(req.Code, rawSecret, time.Now(), totp.ValidateOpts{
                Period: 30,
                Digits: 6,
                Skew:   2,
            })
            log.Printf("🔍 TOTP проверка (skew=2): %v", valid)
        }
    }

    // ✅ ЕСЛИ НЕ ПРОШЕЛ - ПРОВЕРЯЕМ РЕЗЕРВНЫЙ КОД
    if !valid && len(req.Code) == 10 {
        log.Printf("🔍 Проверяем резервный код...")
        
        var backupCodes []string
        err := database.Pool.QueryRow(c.Request.Context(),
            "SELECT codes FROM twofa_backup_codes WHERE user_id = $1 AND tenant_id = $2",
            userID, tenantID).Scan(&backupCodes)

        if err == nil && len(backupCodes) > 0 {
            log.Printf("🔍 Найдено %d резервных кодов", len(backupCodes))
            for i, code := range backupCodes {
                log.Printf("🔍 Резервный код #%d: %s", i+1, code)
                if code == req.Code {
                    valid = true
                    backupCodes = append(backupCodes[:i], backupCodes[i+1:]...)
                    _, _ = database.Pool.Exec(c.Request.Context(), `
                        UPDATE twofa_backup_codes SET codes = $1, updated_at = NOW()
                        WHERE user_id = $2 AND tenant_id = $3
                    `, backupCodes, userID, tenantID)
                    log.Printf("✅ 2FA отключена через резервный код")
                    break
                }
            }
        } else {
            log.Printf("❌ Резервные коды не найдены: %v", err)
        }
    }

    if !valid {
        log.Printf("[SECURITY] 🔴 Неудачная попытка отключения 2FA для пользователя %s с IP: %s", userID, c.ClientIP())
        log.Printf("🔍 ========================================")
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": "Неверный код. Используйте 6-значный код из Google Authenticator или 10-значный резервный код.",
        })
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

    log.Printf("✅ 2FA успешно отключена для пользователя %s", userID)
    log.Printf("🔍 ========================================")

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "2FA отключена",
    })
}
// GetTwoFAStatus - статус 2FA
func GetTwoFAStatus(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    if tenantID == uuid.Nil {
        c.JSON(http.StatusOK, gin.H{
            "enabled": false,
            "exists":  false,
        })
        return
    }

    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusOK, gin.H{
            "enabled": false,
            "exists":  false,
        })
        return
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
    if tenantID == uuid.Nil {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "codes":   []string{},
        })
        return
    }

    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "codes":   []string{},
        })
        return
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
    if tenantID == uuid.Nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id not found"})
        return
    }

    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
        return
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
    if tenantID == uuid.Nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
        return
    }

    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
        return
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

// Check2FAProfileStatus - проверка статуса 2FA для входа в профиль
func Check2FAProfileStatus(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var enabled bool
    var lastVerifiedAt *time.Time
    var secret string

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT enabled, last_verified_at, secret 
        FROM twofa_settings 
        WHERE user_id = $1
    `, userID).Scan(&enabled, &lastVerifiedAt, &secret)

    if err != nil || !enabled {
        c.JSON(http.StatusOK, gin.H{
            "enabled":               false,
            "requires_verification": false,
            "message":               "2FA не включена",
        })
        return
    }

    // Если 2FA включена, проверяем когда была последняя верификация
    requiresVerification := true
    verifiedUntil := time.Now().Add(-1 * time.Hour)

    if lastVerifiedAt != nil {
        twoWeeksAgo := time.Now().Add(-14 * 24 * time.Hour)
        if lastVerifiedAt.After(twoWeeksAgo) {
            requiresVerification = false
            verifiedUntil = lastVerifiedAt.Add(14 * 24 * time.Hour)
        }
    }

    // Генерируем QR-код для профиля (если требуется)
    var qrCode string
    if requiresVerification {
        email := c.GetString("user_email")
        issuer := "Business Stack"
        qrCode = utils.GenerateQRCode(secret, email, issuer)
    }

    c.JSON(http.StatusOK, gin.H{
        "enabled":                enabled,
        "requires_verification": requiresVerification,
        "last_verified_at":      lastVerifiedAt,
        "verified_until":        verifiedUntil,
        "qr_code":               qrCode,
        "secret":                secret,
        "message": func() string {
            if requiresVerification {
                return "Требуется подтверждение 2FA для доступа к профилю"
            }
            return "2FA подтверждена, доступ к профилю разрешён"
        }(),
    })
}

// Verify2FAProfile - подтверждение 2FA для входа в профиль
func Verify2FAProfile(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var req struct {
        Code string `json:"code" binding:"required"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Проверяем, включена ли 2FA
    var secret string
    var enabled bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT secret, enabled FROM twofa_settings WHERE user_id = $1
    `, userID).Scan(&secret, &enabled)

    if err != nil || !enabled {
        c.JSON(http.StatusBadRequest, gin.H{"error": "2FA не включена"})
        return
    }

    // Верифицируем код
    valid := totp.Validate(req.Code, secret)
    if !valid {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный код"})
        return
    }

    // Обновляем время последней верификации
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE twofa_settings 
        SET last_verified_at = NOW() 
        WHERE user_id = $1
    `, userID)

    if err != nil {
        log.Printf("⚠️ Ошибка обновления last_verified_at: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления"})
        return
    }

    verifiedUntil := time.Now().Add(14 * 24 * time.Hour)

    c.JSON(http.StatusOK, gin.H{
        "success":        true,
        "message":        "2FA подтверждена. Доступ к профилю открыт на 14 дней.",
        "verified_until": verifiedUntil,
    })
}
// ============================================================
// ФУНКЦИИ ДЛЯ СКИДКИ ЗА 2FA
// ============================================================

// ActivateTwoFADiscount - активирует скидку после включения 2FA
func ActivateTwoFADiscount(userID string, tenantID uuid.UUID) (int, error) {
    // Проверяем, не было ли уже активной скидки
    var count int
    err := database.Pool.QueryRow(context.Background(), `
        SELECT COUNT(*) FROM twofa_discount_history 
        WHERE user_id = $1 AND status IN ('pending', 'used')
    `, userID).Scan(&count)
    
    if err != nil {
        return 0, err
    }
    
    // Если уже есть активная скидка - не создаем новую
    if count > 0 {
        log.Printf("[2FA] Скидка уже существует для пользователя %s", userID)
        return 0, nil
    }
    
    // Вычисляем даты: со следующего дня на 1 месяц
    now := time.Now()
    validFrom := now.AddDate(0, 0, 1).Format("2006-01-02")
    validTo := now.AddDate(0, 1, 0).Format("2006-01-02")
    
    var discountID int
    // ✅ ТОЛЬКО ПОЛЯ, КОТОРЫЕ ЕСТЬ В ТАБЛИЦЕ
    err = database.Pool.QueryRow(context.Background(), `
        INSERT INTO twofa_discount_history (
            user_id, 
            discount_percent, 
            valid_from, 
            valid_to, 
            status,
            applied_at
        ) VALUES ($1, $2, $3, $4, $5, NOW())
        RETURNING id
    `, userID, 5, validFrom, validTo, "pending").Scan(&discountID)
    
    if err != nil {
        log.Printf("[2FA] Ошибка создания скидки: %v", err)
        return 0, err
    }
    
    // Обновляем пользователя
    _, err = database.Pool.Exec(context.Background(), `
        UPDATE users 
        SET twofa_discount_active = true,
            twofa_discount_start_date = $1,
            twofa_discount_end_date = $2,
            twofa_discount_used = false,
            updated_at = NOW()
        WHERE id = $3
    `, validFrom, validTo, userID)
    
    if err != nil {
        log.Printf("[2FA] Ошибка обновления пользователя: %v", err)
        return discountID, err
    }
    
    log.Printf("[2FA] ✅ Скидка 5%% создана для пользователя %s (ID: %d)", userID, discountID)
    return discountID, nil
}
// GetTwoFADiscountStatus - получить статус скидки
func GetTwoFADiscountStatus(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    if tenantID == uuid.Nil {
        c.JSON(http.StatusOK, gin.H{
            "has_discount": false,
            "message":      "Tenant не найден",
        })
        return
    }

    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
        return
    }

    var discount struct {
        ID          int    `json:"id"`
        Percent     int    `json:"percent"`
        ValidFrom   string `json:"valid_from"`
        ValidTo     string `json:"valid_to"`
        Status      string `json:"status"`
        Description string `json:"description"`
    }
    
    // ✅ ИСПРАВЛЕННЫЙ ЗАПРОС
    err := database.Pool.QueryRow(context.Background(), `
        SELECT 
            id, 
            discount_percent, 
            COALESCE(valid_from::text, '') as valid_from,
            COALESCE(valid_to::text, '') as valid_to,
            status,
            CASE 
                WHEN status = 'pending' AND valid_from <= CURRENT_DATE AND valid_to >= CURRENT_DATE 
                    THEN '✅ Доступна для применения'
                WHEN status = 'pending' AND valid_from > CURRENT_DATE 
                    THEN '⏳ Будет доступна с ' || valid_from::text
                WHEN status = 'used' 
                    THEN '✅ Использована'
                WHEN status = 'expired' 
                    THEN '❌ Истекла'
                ELSE 'ℹ️ ' || COALESCE(status, 'unknown')
            END as description
        FROM twofa_discount_history
        WHERE user_id = $1
          AND (status = 'pending' OR status = 'used')
        ORDER BY id DESC
        LIMIT 1
    `, userID).Scan(
        &discount.ID,
        &discount.Percent,
        &discount.ValidFrom,
        &discount.ValidTo,
        &discount.Status,
        &discount.Description,
    )
    
    if err != nil {
        // Логируем ошибку для отладки
        log.Printf("[2FA] Ошибка получения скидки для user %s: %v", userID, err)
        c.JSON(http.StatusOK, gin.H{
            "has_discount": false,
            "message":      "Нет активных скидок 2FA",
            "error":        err.Error(),
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "has_discount": true,
        "discount":     discount,
    })
}
// ApplyTwoFADiscountToSubscription - применить скидку к подписке
func ApplyTwoFADiscountToSubscription(c *gin.Context) {
    var req struct {
        SubscriptionID int `json:"subscription_id"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат запроса"})
        return
    }
    
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
        return
    }
    
    tenantID := middleware.GetTenantIDFromContext(c)
    if tenantID == uuid.Nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id not found"})
        return
    }
    
    // Проверяем активную скидку 2FA
    var discount struct {
        ID          int
        ValidFrom   string
        ValidTo     string
        DiscountPct int
    }
    
    err := database.Pool.QueryRow(context.Background(), `
        SELECT id, valid_from, valid_to, discount_percent
        FROM twofa_discount_history
        WHERE user_id = $1 
          AND status = 'pending'
          AND valid_from <= CURRENT_DATE
          AND valid_to >= CURRENT_DATE
        ORDER BY id DESC
        LIMIT 1
    `, userID).Scan(
        &discount.ID,
        &discount.ValidFrom,
        &discount.ValidTo,
        &discount.DiscountPct,
    )
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "has_discount": false,
            "message":      "Активных скидок 2FA нет",
        })
        return
    }
    
    // Применяем скидку к подписке
    _, err = database.Pool.Exec(context.Background(), `
        UPDATE user_subscriptions 
        SET discount_percent = $1,
            discount_id = $2,
            final_price = price * (1 - $1::float/100)
        WHERE id = $3 AND user_id = $4
    `, discount.DiscountPct, discount.ID, req.SubscriptionID, userID)
    
    if err != nil {
        log.Printf("[2FA] Ошибка применения скидки: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка применения скидки"})
        return
    }
    
    // Помечаем скидку как использованную
    _, err = database.Pool.Exec(context.Background(), `
        UPDATE twofa_discount_history 
        SET status = 'used',
            subscription_id = $1,
            updated_at = NOW()
        WHERE id = $2
    `, req.SubscriptionID, discount.ID)
    
    if err != nil {
        log.Printf("[2FA] Ошибка обновления статуса скидки: %v", err)
    }
    
    // Обновляем пользователя
    _, err = database.Pool.Exec(context.Background(), `
        UPDATE users 
        SET twofa_discount_used = true,
            updated_at = NOW()
        WHERE id = $1
    `, userID)
    
    if err != nil {
        log.Printf("[2FA] Ошибка обновления пользователя: %v", err)
    }
    
    log.Printf("[2FA] ✅ Скидка %d%% применена к подписке %d для пользователя %s", 
        discount.DiscountPct, req.SubscriptionID, userID)
    
    c.JSON(http.StatusOK, gin.H{
        "success":      true,
        "message":      "Скидка 5% применена к подписке!",
        "discount":     discount.DiscountPct,
        "valid_until":  discount.ValidTo,
    })
}