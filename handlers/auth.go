package handlers

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "log"
    "net/http"
    "os"  
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "golang.org/x/crypto/bcrypt"
    "subscription-system/config"
    "subscription-system/database"
    "subscription-system/models"
    "subscription-system/utils"
)

// InitAuthHandler инициализирует обработчики авторизации
func InitAuthHandler(cfg *config.Config) {
    log.Println("✅ Auth handler initialized")
}

// generateRandomStringAuth генерирует случайную строку
func generateRandomStringAuth(length int) string {
    bytes := make([]byte, length)
    rand.Read(bytes)
    return hex.EncodeToString(bytes)[:length]
}

// SendPhoneCode отправляет код на телефон
func SendPhoneCode(c *gin.Context) {
    var req struct {
        Phone string `json:"phone" binding:"required"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
    expiresAt := time.Now().Add(5 * time.Minute)

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO phone_auth_codes (phone, code, expires_at)
        VALUES ($1, $2, $3)
        ON CONFLICT (phone) DO UPDATE SET code = $2, expires_at = $3
    `, req.Phone, code, expiresAt)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save code"})
        return
    }

    log.Printf("📱 Код для %s: %s", req.Phone, code)

    c.JSON(http.StatusOK, gin.H{
        "message":    "Код отправлен",
        "expires_in": 300,
    })
}

// VerifyPhoneCode проверяет код с телефона
func VerifyPhoneCode(c *gin.Context) {
    var req struct {
        Phone string `json:"phone" binding:"required"`
        Code  string `json:"code" binding:"required"`
        Name  string `json:"name"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var storedCode string
    var expiresAt time.Time
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT code, expires_at FROM phone_auth_codes
        WHERE phone = $1 AND expires_at > NOW()
    `, req.Phone).Scan(&storedCode, &expiresAt)

    if err != nil || storedCode != req.Code {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired code"})
        return
    }

    var userID uuid.UUID
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT id FROM users WHERE phone = $1
    `, req.Phone).Scan(&userID)

    userName := req.Name
    if userName == "" {
        userName = "User_" + req.Phone[len(req.Phone)-4:]
    }

    if err != nil {
        email := fmt.Sprintf("%s@phone.Business Stack.ru", generateRandomStringAuth(8))
        err = database.Pool.QueryRow(c.Request.Context(), `
            INSERT INTO users (phone, name, email, role, tenant_id, password_changed_at, email_verified) 
            VALUES ($1, $2, $3, 'user', '11111111-1111-1111-1111-111111111111', NOW(), true) 
            RETURNING id
        `, req.Phone, userName, email).Scan(&userID)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
            return
        }
    }

    token, err := GenerateJWTForUser(userID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
        return
    }

    database.Pool.Exec(c.Request.Context(), "DELETE FROM phone_auth_codes WHERE phone = $1", req.Phone)

    c.JSON(http.StatusOK, gin.H{
        "token": token,
        "user": gin.H{
            "id":   userID,
            "name": userName,
        },
    })
}

// LoginHandler обрабатывает вход пользователя
func LoginHandler(c *gin.Context) {
    var req struct {
        Email    string `json:"email" binding:"required,email"`
        Password string `json:"password" binding:"required"`
        Remember bool   `json:"remember"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var user models.User
    var tenantID string
    var isDeveloper bool
    var developerLevel int

    err := database.Pool.QueryRow(c.Request.Context(),
        `SELECT id, email, password_hash, name, role, 
                COALESCE(tenant_id, '11111111-1111-1111-1111-111111111111') as tenant_id,
                COALESCE(is_developer, false) as is_developer,
                COALESCE(developer_level, 0) as developer_level
         FROM users WHERE email = $1`,
        req.Email).Scan(
        &user.ID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
        &tenantID, &isDeveloper, &developerLevel)

    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
        return
    }
    user.TenantID = tenantID

    if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
        return
    }

    var actualRole string
    err = database.Pool.QueryRow(c.Request.Context(),
        "SELECT role FROM users WHERE email = $1", req.Email).Scan(&actualRole)
    if err == nil && actualRole != "" {
        user.Role = actualRole
        log.Printf("[LOGIN] 🔄 Обновлена роль для %s: %s", req.Email, user.Role)
    }

    if isDeveloper && user.Role != "owner" {
        user.Role = "admin"
        log.Printf("[LOGIN] 🔧 Разработчик %s получил роль admin", req.Email)
    }

    if req.Email == "dev@businesstack.ru" {
        user.Role = "owner"
        log.Printf("[LOGIN] 👑 ВЛАДЕЛЕЦ %s авторизован", req.Email)
    }

    var accessExpiry, refreshExpiry time.Duration
    if req.Remember {
        accessExpiry = 30 * 24 * time.Hour
        refreshExpiry = 90 * 24 * time.Hour
    } else {
        accessExpiry = 15 * time.Minute
        refreshExpiry = 24 * time.Hour
    }

    accessToken, refreshToken, err := utils.GenerateTokensWithExpiry(
        user.ID.String(), user.Name, user.Email, user.Role, accessExpiry, refreshExpiry)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
        return
    }

    c.SetCookie("token", accessToken, int(accessExpiry.Seconds()), "/", "", false, true)

    _, err = database.Pool.Exec(c.Request.Context(),
        `INSERT INTO user_tokens (user_id, token, expires_at, created_at, tenant_id) 
         VALUES ($1, $2, NOW() + $3 * interval '1 second', NOW(), $4)`,
        user.ID.String(), refreshToken, int(refreshExpiry.Seconds()), user.TenantID)
    if err != nil {
        log.Printf("⚠️ Failed to save refresh token: %v", err)
    }

    database.Pool.Exec(context.Background(),
        `INSERT INTO login_history (user_id, ip_address, user_agent, login_time, tenant_id) 
         VALUES ($1, $2, $3, NOW(), $4)`,
        user.ID.String(), c.ClientIP(), c.GetHeader("User-Agent"), user.TenantID)

    log.Printf("[LOGIN] ✅ Успешный вход: %s (%s), роль=%s", user.Name, user.Email, user.Role)

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "access_token":  accessToken,
        "refresh_token": refreshToken,
        "remember":      req.Remember,
        "expires_in":    accessExpiry.Seconds(),
        "user": gin.H{
            "id":    user.ID.String(),
            "email": user.Email,
            "name":  user.Name,
            "role":  user.Role,
        },
    })
}

// LogoutHandler обрабатывает выход пользователя
func LogoutHandler(c *gin.Context) {
    var req struct {
        RefreshToken string `json:"refresh_token" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(),
        "DELETE FROM user_tokens WHERE token = $1", req.RefreshToken)
    if err != nil {
        log.Printf("⚠️ Failed to delete refresh token: %v", err)
    }

    c.SetCookie("access_token", "", -1, "/", "", false, true)
    c.SetCookie("refresh_token", "", -1, "/", "", false, true)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Successfully logged out",
    })
}

// RefreshHandler обновляет access token
func RefreshHandler(c *gin.Context) {
    var req struct {
        RefreshToken string `json:"refresh_token" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT EXISTS(SELECT 1 FROM user_tokens WHERE token = $1 AND expires_at > NOW())",
        req.RefreshToken).Scan(&exists)
    if err != nil || !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token"})
        return
    }

    newAccessToken, err := utils.RefreshToken(req.RefreshToken)
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":      true,
        "access_token": newAccessToken,
    })
}

// ResendVerificationHandler отправляет код подтверждения повторно
func ResendVerificationHandler(c *gin.Context) {
    var req struct {
        Email string `json:"email" binding:"required,email"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var user models.User
    err := database.Pool.QueryRow(c.Request.Context(),
        `SELECT id, name, email_verified FROM users WHERE email = $1`,
        req.Email).Scan(&user.ID, &user.Name, &user.EmailVerified)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
        return
    }

    if user.EmailVerified {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Email already verified"})
        return
    }

    verificationCode, err := GenerateVerificationCode(user.ID.String(), "email")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate code"})
        return
    }

    go func() {
        emailService := utils.NewEmailService(config.Load())
        err := emailService.SendVerificationEmail(req.Email, user.Name, verificationCode)
        if err != nil {
            log.Printf("❌ Failed to send verification email: %v", err)
        } else {
            log.Printf("✅ Verification email resent to %s", req.Email)
        }
    }()

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Verification code sent",
    })
}

// LoginByIDHandler - login by ID
func LoginByIDHandler(c *gin.Context) {
    var req struct {
        Login    string `json:"login" binding:"required"`
        Password string `json:"password" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var user models.User
    err := database.Pool.QueryRow(c.Request.Context(),
        `SELECT id, name, email, password_hash, role, COALESCE(login, '') as login 
         FROM users WHERE login = $1 OR email = $1`,
        req.Login).Scan(&user.ID, &user.Name, &user.Email, &user.PasswordHash, &user.Role, &user.Login)

    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid login or password"})
        return
    }

    if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid login or password"})
        return
    }

    accessToken, refreshToken, err := utils.GenerateTokens(user.ID.String(), user.Name, user.Email, user.Role)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "access_token":  accessToken,
        "refresh_token": refreshToken,
        "user": gin.H{
            "id":    user.ID,
            "login": user.Login,
            "email": user.Email,
            "name":  user.Name,
        },
    })
}

// RegisterByIDHandler - register by ID
func RegisterByIDHandler(c *gin.Context) {
    var req struct {
        Name     string `json:"name" binding:"required"`
        ID       string `json:"id" binding:"required"`
        Password string `json:"password" binding:"required,min=6"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var exists bool
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 OR email = $1)", req.ID).Scan(&exists)

    if exists {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID already registered"})
        return
    }

    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
        return
    }

    userID := uuid.New()
    email := req.ID + "@id.Business Stack.ru"

    _, err = database.Pool.Exec(c.Request.Context(),
        `INSERT INTO users (id, name, email, password_hash, role, tenant_id, created_at, updated_at)
         VALUES ($1, $2, $3, $4, 'user', '11111111-1111-1111-1111-111111111111', NOW(), NOW())`,
        userID, req.Name, email, string(hashedPassword))

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
        return
    }

    accessToken, refreshToken, err := utils.GenerateTokens(userID.String(), req.Name, email, "user")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "access_token":  accessToken,
        "refresh_token": refreshToken,
        "user":          gin.H{"id": userID, "email": email, "name": req.Name, "role": "user"},
        "message":       "Registration by ID successful",
    })
}

// RegisterHandler отправляет ссылку для подтверждения
func RegisterHandler(c *gin.Context) {
    var req struct {
        Email    string `json:"email" binding:"required,email"`
        Password string `json:"password" binding:"required,min=6"`
        Name     string `json:"name"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    log.Printf("📝 Попытка регистрации: email=%s, name=%s", req.Email, req.Name)

    var exists bool
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", req.Email).Scan(&exists)
    if exists {
        log.Printf("❌ Регистрация отклонена: email %s уже существует", req.Email)
        c.JSON(http.StatusConflict, gin.H{"error": "Пользователь с таким email уже зарегистрирован"})
        return
    }

    token := uuid.New().String()
    hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)

    log.Printf("📝 Сохранение в pending_users: email=%s, token=%s", req.Email, token)

    _, err := database.Pool.Exec(c.Request.Context(),
        `INSERT INTO pending_users (email, password_hash, name, token, expires_at)
         VALUES ($1, $2, $3, $4, NOW() + INTERVAL '24 hours')
         ON CONFLICT (email) DO UPDATE SET 
            password_hash = $2, name = $3, token = $4, expires_at = NOW() + INTERVAL '24 hours'`,
        req.Email, string(hashedPassword), req.Name, token)

    if err != nil {
        log.Printf("❌ Ошибка сохранения в pending_users: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения"})
        return
    }

   // Используем DOMAIN или BASE_URL из настроек
// НОВЫЙ КОД (работает с .env)
baseURL := os.Getenv("BASE_URL")
if baseURL == "" {
    baseURL = "http://localhost:8080"
}

verifyLink := fmt.Sprintf("%s/verify-email?token=%s", baseURL, token)
    log.Printf("📧 Ссылка для подтверждения: %s", verifyLink)

    emailService := utils.NewEmailService(config.Load())
    log.Printf("📧 Отправка письма на %s...", req.Email)

    if err := emailService.SendVerificationLink(req.Email, req.Name, verifyLink); err != nil {
        log.Printf("❌ Ошибка отправки письма: %v", err)
        database.Pool.Exec(c.Request.Context(), "DELETE FROM pending_users WHERE email = $1", req.Email)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный email или проблема с отправкой"})
        return
    }
    log.Printf("✅ Письмо успешно отправлено на %s", req.Email)

    go func() {
        adminEmail := config.Load().EmailFrom
        if adminEmail != "" {
            log.Printf("📧 Отправка уведомления админу на %s...", adminEmail)
            if err := emailService.SendAdminNotification(req.Name, req.Email); err != nil {
                log.Printf("❌ Ошибка отправки уведомления админу: %v", err)
            } else {
                log.Printf("✅ Уведомление админу отправлено на %s", adminEmail)
            }
        }
    }()

    c.JSON(http.StatusOK, gin.H{
        "message": "На вашу почту отправлена ссылка для подтверждения",
        "email":   req.Email,
    })
}

// VerifyEmailHandler подтверждает email по токену
func VerifyEmailHandler(c *gin.Context) {
    token := c.Query("token")
    log.Printf("📝 Попытка подтверждения email с токеном: %s", token)

    if token == "" {
        log.Printf("❌ Токен отсутствует")
        c.JSON(http.StatusBadRequest, gin.H{"error": "Токен отсутствует"})
        return
    }

    var email, passwordHash, name string
    var expiresAt time.Time

    err := database.Pool.QueryRow(c.Request.Context(),
        `SELECT email, password_hash, name, expires_at FROM pending_users WHERE token = $1`,
        token).Scan(&email, &passwordHash, &name, &expiresAt)

    if err != nil {
        log.Printf("❌ Неверный токен: %v", err)
        c.String(http.StatusBadRequest, "Неверная или просроченная ссылка")
        return
    }

    if time.Now().After(expiresAt) {
        log.Printf("❌ Токен просрочен: %s", token)
        database.Pool.Exec(c.Request.Context(), "DELETE FROM pending_users WHERE token = $1", token)
        c.String(http.StatusBadRequest, "Ссылка просрочена. Зарегистрируйтесь заново.")
        return
    }

    log.Printf("✅ Токен валиден, создаём пользователя: email=%s, name=%s", email, name)

    _, err = database.Pool.Exec(c.Request.Context(),
        `INSERT INTO users (email, password_hash, name, role, email_verified)
         VALUES ($1, $2, $3, 'client', true)`,
        email, passwordHash, name)

    if err != nil {
        log.Printf("❌ Ошибка при создании пользователя: %v", err)
        c.String(http.StatusInternalServerError, "Ошибка при создании пользователя")
        return
    }

    database.Pool.Exec(c.Request.Context(), "DELETE FROM pending_users WHERE token = $1", token)
    log.Printf("✅ Пользователь %s успешно подтверждён и зарегистрирован!", email)

    c.Header("Content-Type", "text/html")
    c.String(http.StatusOK, `
        <html>
        <body style="font-family: sans-serif; text-align: center; margin-top: 100px;">
            <h1 style="color: #4f46e5;">✅ Email подтверждён!</h1>
            <p>Вы успешно зарегистрировались в Business Stack.</p>
            <a href="/login" style="background: #4f46e5; color: white; padding: 10px 20px; text-decoration: none; border-radius: 8px;">Войти</a>
        </body>
        </html>
    `)
}

// ==================== ВОССТАНОВЛЕНИЕ ПАРОЛЯ ====================

// ForgotPasswordHandler - отправка ссылки для сброса пароля на email
func ForgotPasswordHandler(c *gin.Context) {
    var req struct {
        Email string `json:"email" binding:"required,email"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var user models.User
    err := database.Pool.QueryRow(c.Request.Context(),
        `SELECT id, name FROM users WHERE email = $1`,
        req.Email).Scan(&user.ID, &user.Name)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "message": "Если пользователь существует, ссылка для сброса отправлена",
        })
        return
    }

    resetToken := uuid.New().String()

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO password_resets (user_id, reset_token, expires_at, method)
        VALUES ($1, $2, NOW() + INTERVAL '24 hours', 'email')
        ON CONFLICT (user_id) DO UPDATE SET 
            reset_token = $2, expires_at = NOW() + INTERVAL '24 hours'
    `, user.ID, resetToken)

    if err != nil {
        log.Printf("❌ Ошибка сохранения токена сброса: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сервера"})
        return
    }

    resetLink := fmt.Sprintf("https://businesstack.ru/reset-password?token=%s", resetToken)

    emailService := utils.NewEmailService(config.Load())
    go func() {
        if err := emailService.SendPasswordResetEmail(req.Email, user.Name, resetLink); err != nil {
            log.Printf("❌ Ошибка отправки email для сброса: %v", err)
        } else {
            log.Printf("✅ Email для сброса пароля отправлен на %s", req.Email)
        }
    }()

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Ссылка для сброса пароля отправлена на email",
    })
}

// SendResetCodeHandler - отправка кода для сброса пароля на телефон
func SendResetCodeHandler(c *gin.Context) {
    var req struct {
        Phone string `json:"phone" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var user models.User
    err := database.Pool.QueryRow(c.Request.Context(),
        `SELECT id, name FROM users WHERE phone = $1`,
        req.Phone).Scan(&user.ID, &user.Name)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "message": "Если пользователь существует, код отправлен",
        })
        return
    }

    code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
    if code[0] == '0' {
        code = "1" + code[1:]
    }
    resetToken := uuid.New().String()

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO password_resets (user_id, reset_token, code, expires_at, method)
        VALUES ($1, $2, $3, NOW() + INTERVAL '15 minutes', 'phone')
        ON CONFLICT (user_id) DO UPDATE SET 
            reset_token = $2, code = $3, expires_at = NOW() + INTERVAL '15 minutes', method = 'phone'
    `, user.ID, resetToken, code)

    if err != nil {
        log.Printf("❌ Ошибка сохранения кода сброса: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сервера"})
        return
    }

    log.Printf("📱 Код для сброса пароля для %s: %s", req.Phone, code)

    c.JSON(http.StatusOK, gin.H{
        "success":     true,
        "reset_token": resetToken,
        "message":     "Код подтверждения отправлен на телефон",
    })
}

// VerifyResetCodeHandler - проверка кода для сброса пароля
func VerifyResetCodeHandler(c *gin.Context) {
    var req struct {
        ResetToken string `json:"reset_token" binding:"required"`
        Code       string `json:"code" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var storedCode string
    var expiresAt time.Time
    var userID string

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT user_id, code, expires_at FROM password_resets 
        WHERE reset_token = $1 AND method = 'phone' AND used = false
    `, req.ResetToken).Scan(&userID, &storedCode, &expiresAt)

    if err != nil || time.Now().After(expiresAt) {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный или просроченный код"})
        return
    }

    if storedCode != req.Code {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный код"})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE password_resets SET verified = true WHERE reset_token = $1
    `, req.ResetToken)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сервера"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Код подтвержден",
    })
}

// ResetPasswordHandler - сброс пароля (после проверки)
func ResetPasswordHandler(c *gin.Context) {
    var req struct {
        ResetToken  string `json:"reset_token" binding:"required"`
        NewPassword string `json:"new_password" binding:"required,min=6"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var userID string
    var verified bool
    var expiresAt time.Time

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT user_id, verified, expires_at FROM password_resets 
        WHERE reset_token = $1 AND used = false
    `, req.ResetToken).Scan(&userID, &verified, &expiresAt)

    if err != nil || time.Now().After(expiresAt) {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверный или просроченный токен"})
        return
    }

    if !verified {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Требуется подтверждение кода"})
        return
    }

    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обработки пароля"})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2
    `, string(hashedPassword), userID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления пароля"})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE password_resets SET used = true WHERE reset_token = $1
    `, req.ResetToken)

    if err != nil {
        log.Printf("⚠️ Ошибка обновления статуса токена: %v", err)
    }

    _, _ = database.Pool.Exec(c.Request.Context(), `
        DELETE FROM user_tokens WHERE user_id = $1
    `, userID)

    log.Printf("✅ Пароль успешно сброшен для пользователя %s", userID)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Пароль успешно изменен",
    })
}

// GenerateResetQRHandler - генерация QR кода для сброса пароля
func GenerateResetQRHandler(c *gin.Context) {
    qrToken := uuid.New().String()

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO qr_reset_sessions (session_token, expires_at, created_at)
        VALUES ($1, NOW() + INTERVAL '5 minutes', NOW())
    `, qrToken)

    if err != nil {
        log.Printf("❌ Ошибка генерации QR: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка генерации QR"})
        return
    }

    deeplink := fmt.Sprintf("Business Stack://reset-password?token=%s", qrToken)
    qrImageUrl := fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=250x250&data=%s", deeplink)

    c.JSON(http.StatusOK, gin.H{
        "session_token": qrToken,
        "qr_data_url":   qrImageUrl,
        "deeplink":      deeplink,
        "expires_in":    300,
    })
}

// CheckResetQRStatusHandler - проверка статуса QR кода
func CheckResetQRStatusHandler(c *gin.Context) {
    token := c.Query("token")
    if token == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Token required"})
        return
    }

    var status string
    var userID string
    var expiresAt time.Time

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT status, COALESCE(user_id, '') as user_id, expires_at 
        FROM qr_reset_sessions 
        WHERE session_token = $1
    `, token).Scan(&status, &userID, &expiresAt)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"status": "expired"})
        return
    }

    if time.Now().After(expiresAt) {
        c.JSON(http.StatusOK, gin.H{"status": "expired"})
        return
    }

    if status == "approved" && userID != "" {
        resetToken := uuid.New().String()
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO password_resets (user_id, reset_token, expires_at, method, verified)
            VALUES ($1, $2, NOW() + INTERVAL '10 minutes', 'qr', true)
        `, userID, resetToken)

        if err == nil {
            c.JSON(http.StatusOK, gin.H{
                "status":      "approved",
                "reset_token": resetToken,
            })
            return
        }
    }

    c.JSON(http.StatusOK, gin.H{"status": status})
}

// ConfirmResetQRHandler - подтверждение сброса через QR (вызывается из мобильного приложения)
func ConfirmResetQRHandler(c *gin.Context) {
    var req struct {
        SessionToken string `json:"session_token" binding:"required"`
        UserID       string `json:"user_id" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    var expiresAt time.Time
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT expires_at FROM qr_reset_sessions 
        WHERE session_token = $1 AND status = 'pending'
    `, req.SessionToken).Scan(&expiresAt)

    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Неверная сессия"})
        return
    }

    if time.Now().After(expiresAt) {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Сессия истекла"})
        return
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE qr_reset_sessions 
        SET status = 'approved', user_id = $1 
        WHERE session_token = $2
    `, req.UserID, req.SessionToken)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка подтверждения"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Сброс пароля подтвержден",
    })
}