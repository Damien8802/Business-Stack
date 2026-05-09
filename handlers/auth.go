package handlers

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "log"
    "net/http"
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
        email := fmt.Sprintf("%s@phone.saaspro.ru", generateRandomStringAuth(8))
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
    
    // Загружаем ВСЕ данные пользователя из БД
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

    // Проверка пароля
    if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid email or password"})
        return
    }

    // ✅ КРИТИЧЕСКИ ВАЖНО: ЕЩЕ РАЗ ПЕРЕЗАГРУЖАЕМ РОЛЬ (на случай если она изменилась)
    var actualRole string
    err = database.Pool.QueryRow(c.Request.Context(),
        "SELECT role FROM users WHERE email = $1", req.Email).Scan(&actualRole)
    if err == nil && actualRole != "" {
        user.Role = actualRole
        log.Printf("[LOGIN] 🔄 Обновлена роль для %s: %s", req.Email, user.Role)
    }
    
    // Если разработчик и роль не owner - даем права admin
    if isDeveloper && user.Role != "owner" {
        user.Role = "admin"
        log.Printf("[LOGIN] 🔧 Разработчик %s получил роль admin", req.Email)
    }
    
    // Для конкретного email - принудительно устанавливаем owner
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

    // Генерируем НОВЫЙ токен с ПРАВИЛЬНОЙ ролью
    accessToken, refreshToken, err := utils.GenerateTokensWithExpiry(
        user.ID.String(), user.Name, user.Email, user.Role, accessExpiry, refreshExpiry)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
        return
    }

    // Устанавливаем новую куку
    c.SetCookie("token", accessToken, int(accessExpiry.Seconds()), "/", "", false, true)

    // Сохраняем refresh token
    _, err = database.Pool.Exec(c.Request.Context(),
        `INSERT INTO user_tokens (user_id, token, expires_at, created_at, tenant_id) 
         VALUES ($1, $2, NOW() + $3 * interval '1 second', NOW(), $4)`,
        user.ID.String(), refreshToken, int(refreshExpiry.Seconds()), user.TenantID)
    if err != nil {
        log.Printf("⚠️ Failed to save refresh token: %v", err)
    }

    // Записываем историю входа
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
    email := req.ID + "@id.saaspro.ru"

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
        "user": gin.H{"id": userID, "email": email, "name": req.Name, "role": "user"},
        "message": "Registration by ID successful",
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

    // Проверяем, нет ли уже в основной таблице
    var exists bool
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", req.Email).Scan(&exists)
    if exists {
        c.JSON(http.StatusConflict, gin.H{"error": "Пользователь с таким email уже зарегистрирован"})
        return
    }

    // Генерируем уникальный токен
    token := uuid.New().String()
    hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)

    // Сохраняем во временную таблицу
    _, err := database.Pool.Exec(c.Request.Context(),
        `INSERT INTO pending_users (email, password_hash, name, token, expires_at)
         VALUES ($1, $2, $3, $4, NOW() + INTERVAL '24 hours')
         ON CONFLICT (email) DO UPDATE SET 
            password_hash = $2, name = $3, token = $4, expires_at = NOW() + INTERVAL '24 hours'`,
        req.Email, string(hashedPassword), req.Name, token)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения"})
        return
    }

    // Формируем ссылку
    verifyLink := fmt.Sprintf("https://businesstack.ru/verify-email?token=%s", token)

    // Отправляем письмо
    emailService := utils.NewEmailService(config.Load())
    if err := emailService.SendVerificationLink(req.Email, req.Name, verifyLink); err != nil {
        // Удаляем временную запись, если письмо не ушло
        database.Pool.Exec(c.Request.Context(), "DELETE FROM pending_users WHERE email = $1", req.Email)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный email или проблема с отправкой"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "message": "На вашу почту отправлена ссылка для подтверждения",
        "email":   req.Email,
    })
}

// VerifyEmailHandler подтверждает email по токену
func VerifyEmailHandler(c *gin.Context) {
    token := c.Query("token")
    if token == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Токен отсутствует"})
        return
    }

    var email, passwordHash, name string
    var expiresAt time.Time

    err := database.Pool.QueryRow(c.Request.Context(),
        `SELECT email, password_hash, name, expires_at FROM pending_users WHERE token = $1`,
        token).Scan(&email, &passwordHash, &name, &expiresAt)

    if err != nil {
        c.String(http.StatusBadRequest, "Неверная или просроченная ссылка")
        return
    }

    if time.Now().After(expiresAt) {
        database.Pool.Exec(c.Request.Context(), "DELETE FROM pending_users WHERE token = $1", token)
        c.String(http.StatusBadRequest, "Ссылка просрочена. Зарегистрируйтесь заново.")
        return
    }

    // Создаём пользователя
    _, err = database.Pool.Exec(c.Request.Context(),
        `INSERT INTO users (email, password_hash, name, role, email_verified)
         VALUES ($1, $2, $3, 'client', true)`,
        email, passwordHash, name)

    if err != nil {
        c.String(http.StatusInternalServerError, "Ошибка при создании пользователя")
        return
    }

    // Удаляем временную запись
    database.Pool.Exec(c.Request.Context(), "DELETE FROM pending_users WHERE token = $1", token)

    // Успешная страница
    c.Header("Content-Type", "text/html")
    c.String(http.StatusOK, `
        <html>
        <body style="font-family: sans-serif; text-align: center; margin-top: 100px;">
            <h1 style="color: #4f46e5;">✅ Email подтверждён!</h1>
            <p>Вы успешно зарегистрировались в SaaSPro.</p>
            <a href="/login" style="background: #4f46e5; color: white; padding: 10px 20px; text-decoration: none; border-radius: 8px;">Войти</a>
        </body>
        </html>
    `)
}
