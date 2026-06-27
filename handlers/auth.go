package handlers

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "log"
    "net/http"
    "os"  
    "strings"
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

    // ===== ПРОВЕРКА: существует ли пользователь =====
    var existingUserID uuid.UUID
    var existingName string
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, name FROM users WHERE phone = $1
    `, req.Phone).Scan(&existingUserID, &existingName)

    if err == nil {
        // Пользователь уже существует
        var tenantID string
        database.Pool.QueryRow(c.Request.Context(), `
            SELECT tenant_id FROM users WHERE id = $1
        `, existingUserID).Scan(&tenantID)

        accessToken, refreshToken, err := utils.GenerateTokens(
            existingUserID.String(), existingName, "", "client", tenantID)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
            return
        }

        database.Pool.Exec(c.Request.Context(), "DELETE FROM phone_auth_codes WHERE phone = $1", req.Phone)

        c.JSON(http.StatusOK, gin.H{
            "access_token":  accessToken,
            "refresh_token": refreshToken,
            "user": gin.H{
                "id":   existingUserID,
                "name": existingName,
            },
        })
        return
    }

    // ===== СОЗДАЕМ НОВОГО ПОЛЬЗОВАТЕЛЯ =====
    userName := req.Name
    if userName == "" {
        userName = "Пользователь_" + req.Phone
    }

    email := fmt.Sprintf("%s@phone.businessstack.ru", generateRandomStringAuth(8))

    tenantID := uuid.New().String()
    tenantName := userName + "'s Company"
    subdomain := "user_" + req.Phone

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tenants (id, name, subdomain, status, settings, created_at, updated_at)
        VALUES ($1, $2, $3, 'active', '{}', NOW(), NOW())
    `, tenantID, tenantName, subdomain)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create tenant"})
        return
    }

    var userID uuid.UUID
    err = database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO users (phone, name, email, role, tenant_id, password_changed_at, email_verified) 
        VALUES ($1, $2, $3, 'client', $4, NOW(), true) 
        RETURNING id
    `, req.Phone, userName, email, tenantID).Scan(&userID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
        return
    }

    accessToken, refreshToken, err := utils.GenerateTokens(
        userID.String(), userName, email, "client", tenantID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
        return
    }

    database.Pool.Exec(c.Request.Context(), "DELETE FROM phone_auth_codes WHERE phone = $1", req.Phone)

    c.JSON(http.StatusOK, gin.H{
        "access_token":  accessToken,
        "refresh_token": refreshToken,
        "user": gin.H{
            "id":   userID,
            "name": userName,
        },
    })
}

// LoginHandler обрабатывает вход пользователя
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
            tenant_id,
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

    if req.Email == "dev@businessstack.ru" {
        user.Role = "owner"
        log.Printf("[LOGIN] 👑 ВЛАДЕЛЕЦ %s авторизован", req.Email)
    }

    var accessExpiry, refreshExpiry time.Duration
    if req.Remember {
        accessExpiry = 30 * 24 * time.Hour
        refreshExpiry = 90 * 24 * time.Hour
    } else {
        accessExpiry = 24 * time.Hour
        refreshExpiry = 30 * 24 * time.Hour
    }

    accessToken, refreshToken, err := utils.GenerateTokensWithExpiry(
        user.ID.String(), user.Name, user.Email, user.Role, tenantID, accessExpiry, refreshExpiry)
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

    // ✅ ЗАПИСЬ В ИСТОРИЮ ВХОДОВ - ИСПРАВЛЕНО (login_time вместо created_at)
_, err = database.Pool.Exec(c.Request.Context(),
    `INSERT INTO login_history (user_id, ip_address, user_agent, login_time, tenant_id) 
     VALUES ($1, $2, $3, NOW(), $4)`,
    user.ID.String(), c.ClientIP(), c.GetHeader("User-Agent"), user.TenantID)
if err != nil {
    log.Printf("⚠️ Ошибка записи в login_history: %v", err)
}
    if err != nil {
        log.Printf("⚠️ Ошибка записи в login_history: %v", err)
    }

    // ✅ СОЗДАЕМ СЕССИЮ
    deviceName := c.GetHeader("X-Device-Name")
    if deviceName == "" {
        deviceName = c.GetHeader("User-Agent")
        if len(deviceName) > 50 {
            deviceName = deviceName[:50] + "..."
        }
    }
    
    if err := CreateUserSession(
        c.Request.Context(),
        user.ID.String(),
        user.TenantID,
        c.ClientIP(),
        c.GetHeader("User-Agent"),
        deviceName,
    ); err != nil {
        log.Printf("⚠️ Ошибка создания сессии: %v", err)
    }

 // ✅ СОЗДАЕМ ДОВЕРЕННОЕ УСТРОЙСТВО (ВСЕГДА)
deviceName = c.GetHeader("X-Device-Name")
if deviceName == "" {
    deviceName = parseBrowser(c.GetHeader("User-Agent"))
}

err = CreateTrustedDevice(
    c.Request.Context(),
    user.ID.String(),
    user.TenantID,
    c.ClientIP(),
    c.GetHeader("User-Agent"),
    deviceName,
)
if err != nil {
    log.Printf("⚠️ Ошибка создания доверенного устройства: %v", err)
}

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

    // Получаем данные пользователя из refresh токена
    var tenantID, userID, userName, email, role string

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT u.tenant_id, u.id, u.name, u.email, u.role
        FROM user_tokens ut
        JOIN users u ON ut.user_id = u.id
        WHERE ut.token = $1 AND ut.expires_at > NOW()
    `, req.RefreshToken).Scan(&tenantID, &userID, &userName, &email, &role)

    if err != nil || tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token"})
        return
    }

    // Генерируем НОВЫЙ access token с правильным tenant_id
    newAccessToken, _, err := utils.GenerateTokens(
        userID,
        userName,
        email,
        role,
        tenantID,
    )
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate new token"})
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

    accessToken, refreshToken, err := utils.GenerateTokens(user.ID.String(), user.Name, user.Email, user.Role, user.TenantID)
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
        "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1 OR email = $2)", req.ID, req.ID+"@id.businessstack.ru").Scan(&exists)

    if exists {
        c.JSON(http.StatusBadRequest, gin.H{"error": "ID already registered"})
        return
    }

    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
        return
    }

    // ===== СОЗДАЕМ ОТДЕЛЬНЫЙ TENANT =====
    tenantID := uuid.New().String()
    subdomain := strings.ToLower(req.ID)
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tenants (id, name, subdomain, status, settings, created_at, updated_at)
        VALUES ($1, $2, $3, 'active', '{}', NOW(), NOW())
    `, tenantID, req.Name+"'s Company", subdomain)

    if err != nil {
        // Если subdomain занят, генерируем уникальный
        subdomain = subdomain + "_" + uuid.New().String()[:4]
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO tenants (id, name, subdomain, status, settings, created_at, updated_at)
            VALUES ($1, $2, $3, 'active', '{}', NOW(), NOW())
        `, tenantID, req.Name+"'s Company", subdomain)
        
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create tenant"})
            return
        }
    }

    // ===== СОЗДАЕМ ПОЛЬЗОВАТЕЛЯ С ПРИВЯЗКОЙ К TENANT =====
    userID := uuid.New()
    email := req.ID + "@id.businessstack.ru"

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO users (id, name, email, password_hash, role, tenant_id, created_at, updated_at)
        VALUES ($1, $2, $3, $4, 'client', $5, NOW(), NOW())
    `, userID, req.Name, email, string(hashedPassword), tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
        return
    }

    accessToken, refreshToken, err := utils.GenerateTokens(userID.String(), req.Name, email, "client", tenantID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "access_token":  accessToken,
        "refresh_token": refreshToken,
        "user":          gin.H{"id": userID, "email": email, "name": req.Name, "role": "client"},
        "message":       "Registration by ID successful",
        "tenant_id":     tenantID,
    })
}

// RegisterHandler отправляет ссылку для подтверждения
func RegisterHandler(c *gin.Context) {
    var req struct {
        Email    string `json:"email" binding:"required,email"`
        Password string `json:"password" binding:"required,min=6"`
        Name     string `json:"name"`
        Company  string `json:"company"`
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

    metadata := fmt.Sprintf(`{"company":"%s"}`, req.Company)
    
    _, err := database.Pool.Exec(c.Request.Context(),
        `INSERT INTO pending_users (email, password_hash, name, token, expires_at, metadata)
         VALUES ($1, $2, $3, $4, NOW() + INTERVAL '24 hours', $5)
         ON CONFLICT (email) DO UPDATE SET 
            password_hash = $2, name = $3, token = $4, expires_at = NOW() + INTERVAL '24 hours', metadata = $5`,
        req.Email, string(hashedPassword), req.Name, token, metadata)

    if err != nil {
        log.Printf("❌ Ошибка сохранения в pending_users: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения"})
        return
    }

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
    var metadata []byte

    err := database.Pool.QueryRow(c.Request.Context(),
        `SELECT email, password_hash, name, expires_at, COALESCE(metadata, '{}') FROM pending_users WHERE token = $1`,
        token).Scan(&email, &passwordHash, &name, &expiresAt, &metadata)

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

    // Извлекаем название компании из metadata (если есть)
    companyName := name + "'s Company"

    // СОЗДАЕМ TENANT ДЛЯ ПОЛЬЗОВАТЕЛЯ
    tenantID := uuid.New()
    subdomain := strings.ToLower(strings.Split(email, "@")[0])

    // Проверяем уникальность subdomain
    var existingTenantID uuid.UUID
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT id FROM tenants WHERE subdomain = $1
    `, subdomain).Scan(&existingTenantID)

    if err == nil {
        subdomain = subdomain + "_" + uuid.New().String()[:4]
        log.Printf("⚠️ Subdomain занят, используем: %s", subdomain)
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tenants (id, name, subdomain, status, settings, created_at, updated_at)
        VALUES ($1, $2, $3, 'active', '{}', NOW(), NOW())
    `, tenantID, companyName, subdomain)

    if err != nil {
        log.Printf("❌ Ошибка создания tenant: %v", err)
        c.String(http.StatusInternalServerError, "Ошибка при создании организации")
        return
    }

    // РОЛЬ - "client" (обычный клиент)
    userID := uuid.New()
    _, err = database.Pool.Exec(c.Request.Context(),
        `INSERT INTO users (id, tenant_id, email, password_hash, name, role, email_verified, created_at, updated_at)
         VALUES ($1, $2, $3, $4, $5, 'client', true, NOW(), NOW())`,
        userID, tenantID, email, passwordHash, name)

    if err != nil {
        log.Printf("❌ Ошибка при создании пользователя: %v", err)
        database.Pool.Exec(c.Request.Context(), "DELETE FROM tenants WHERE id = $1", tenantID)
        c.String(http.StatusInternalServerError, "Ошибка при создании пользователя")
        return
    }

    database.Pool.Exec(c.Request.Context(), "DELETE FROM pending_users WHERE token = $1", token)
    log.Printf("✅ Пользователь %s успешно подтверждён и зарегистрирован! Tenant: %s, Роль: client", email, tenantID)

    c.Header("Content-Type", "text/html")
    c.String(http.StatusOK, `
        <html>
        <body style="font-family: sans-serif; text-align: center; margin-top: 100px;">
            <h1 style="color: #4f46e5;">✅ Email подтверждён!</h1>
            <p>Вы успешно зарегистрировались в Business Stack.</p>
            <p>ID вашей организации: <strong>`+tenantID.String()+`</strong></p>
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

    resetLink := fmt.Sprintf("https://businessstack.ru/reset-password?token=%s", resetToken)

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
// GenerateResetQRHandler - генерация QR кода для сброса пароля
func GenerateResetQRHandler(c *gin.Context) {
    qrToken := uuid.New().String()

    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO qr_reset_sessions (session_token, user_id, expires_at, created_at)
        VALUES ($1, NULL, NOW() + INTERVAL '5 minutes', NOW())
    `, qrToken)

    if err != nil {
        log.Printf("❌ [QR] Ошибка генерации: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка генерации QR"})
        return
    }

    // ✅ БЕРЁМ ИЗ .ENV
    baseURL := os.Getenv("BASE_URL")
    if baseURL == "" {
        baseURL = "http://localhost:8080"  // fallback для разработки
    }
    qrURL := fmt.Sprintf("%s/qr/confirm-reset?token=%s", baseURL, qrToken)
    qrImageUrl := fmt.Sprintf("https://api.qrserver.com/v1/create-qr-code/?size=300x300&data=%s", qrURL)

    log.Printf("🔗 [QR] Ссылка: %s", qrURL)

    c.JSON(http.StatusOK, gin.H{
        "session_token": qrToken,
        "qr_data_url":   qrImageUrl,
        "deeplink":      qrURL,
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

    log.Printf("🔍 [QR] Проверка токена: %s", token)

    var status string
    var userID string // ← Используем string, а не uuid.UUID
    var expiresAt time.Time

    // ✅ ИСПРАВЛЕНО: Используем правильный запрос
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(status, 'pending') as status, 
               COALESCE(user_id::text, '') as user_id, 
               expires_at
        FROM qr_reset_sessions 
        WHERE session_token = $1
    `, token).Scan(&status, &userID, &expiresAt)

    if err != nil {
        log.Printf("❌ [QR] Ошибка поиска сессии: %v", err)
        c.JSON(http.StatusOK, gin.H{
            "status":  "expired",
            "message": "Session not found",
        })
        return
    }

    log.Printf("✅ [QR] Найдена сессия: status=%s, user_id=%s, expires_at=%s", status, userID, expiresAt)

    // Проверяем истекла ли сессия
    if expiresAt.Before(time.Now()) {
        log.Printf("⏰ [QR] Сессия истекла")
        c.JSON(http.StatusOK, gin.H{
            "status":  "expired",
            "message": "QR code expired",
        })
        return
    }

    // Если статус "approved" и есть user_id
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
                "redirect":    "/reset-password?token=" + resetToken,
            })
            return
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "status":  status,
        "user_id": userID,
    })
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

// RegisterEmailHandler - обычная регистрация по email с созданием tenant
func RegisterEmailHandler(c *gin.Context) {
    var req struct {
        Email    string `json:"email" binding:"required,email"`
        Password string `json:"password" binding:"required,min=6"`
        Name     string `json:"name" binding:"required"`
        Company  string `json:"company" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error":   "Неверные данные",
            "details": err.Error(),
        })
        return
    }

    log.Printf("📝 Регистрация нового пользователя: email=%s, company=%s", req.Email, req.Company)

    // Проверяем, не существует ли уже такой email
    var existingUserID uuid.UUID
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id FROM users WHERE email = $1
    `, req.Email).Scan(&existingUserID)

    if err == nil {
        log.Printf("❌ Регистрация отклонена: email %s уже существует", req.Email)
        c.JSON(http.StatusConflict, gin.H{
            "error": "Пользователь с таким email уже зарегистрирован",
        })
        return
    }

    // Создаем tenant
    tenantID := uuid.New()
    subdomain := strings.ToLower(strings.Split(req.Email, "@")[0])
    
    // Проверяем уникальность subdomain
    var existingTenantID uuid.UUID
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT id FROM tenants WHERE subdomain = $1
    `, subdomain).Scan(&existingTenantID)
    
    if err == nil {
        // Если subdomain занят, добавляем случайные цифры
        subdomain = subdomain + "_" + uuid.New().String()[:4]
        log.Printf("⚠️ Subdomain занят, используем: %s", subdomain)
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO tenants (id, name, subdomain, status, settings, created_at, updated_at)
        VALUES ($1, $2, $3, 'active', '{}', NOW(), NOW())
    `, tenantID, req.Company, subdomain)

    if err != nil {
        log.Printf("❌ Ошибка создания tenant: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Ошибка регистрации. Попробуйте позже.",
        })
        return
    }

    // Хешируем пароль
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    if err != nil {
        log.Printf("❌ Ошибка хеширования пароля: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Ошибка регистрации",
        })
        return
    }

    // Создаем пользователя с привязкой к tenant
    userID := uuid.New()
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO users (id, tenant_id, email, password_hash, name, role, email_verified, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, 'client', true, NOW(), NOW())
    `, userID, tenantID, req.Email, string(hashedPassword), req.Name)

    if err != nil {
        log.Printf("❌ Ошибка создания пользователя: %v", err)
        // Откатываем создание tenant
        database.Pool.Exec(c.Request.Context(), `DELETE FROM tenants WHERE id = $1`, tenantID)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Ошибка регистрации",
        })
        return
    }

    // Генерируем токены
    accessToken, refreshToken, err := utils.GenerateTokens(
        userID.String(), req.Name, req.Email, "client", tenantID.String())
    if err != nil {
        log.Printf("❌ Ошибка генерации токенов: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Ошибка регистрации",
        })
        return
    }

    log.Printf("✅ Зарегистрирован новый пользователь: %s (tenant: %s, company: %s)", 
        req.Email, tenantID, req.Company)

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "message":       "Регистрация успешна",
        "access_token":  accessToken,
        "refresh_token": refreshToken,
        "tenant_id":     tenantID.String(),
        "user": gin.H{
            "id":    userID.String(),
            "email": req.Email,
            "name":  req.Name,
            "role":  "client",
        },
    })
}
// GetCurrentUserForQR - получение текущего пользователя для QR-подтверждения
func GetCurrentUserForQR(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var email, name string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT email, COALESCE(name, '') as name 
        FROM users 
        WHERE id = $1
    `, userID).Scan(&email, &name)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "user": gin.H{
            "id":    userID,
            "email": email,
            "name":  name,
        },
    })
}

// ==================== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ДЛЯ СЕССИЙ ====================

// parseOS - парсит операционную систему из User-Agent
func parseOSAuth(userAgent string) string {
    userAgent = strings.ToLower(userAgent)
    if strings.Contains(userAgent, "windows") {
        return "Windows"
    }
    if strings.Contains(userAgent, "mac") || strings.Contains(userAgent, "macintosh") {
        return "macOS"
    }
    if strings.Contains(userAgent, "linux") && !strings.Contains(userAgent, "android") {
        return "Linux"
    }
    if strings.Contains(userAgent, "android") {
        return "Android"
    }
    if strings.Contains(userAgent, "ios") || strings.Contains(userAgent, "iphone") || strings.Contains(userAgent, "ipad") {
        return "iOS"
    }
    return "Unknown"
}
// CreateUserSession - создает сессию для пользователя
func CreateUserSession(ctx context.Context, userID, tenantID, ip, userAgent, deviceName string) error {
    sessionID := uuid.New().String()
    token := uuid.New().String() // ← Генерируем токен для сессии (обязательно!)
    
    browser := parseBrowser(userAgent)
    os := parseOSAuth(userAgent)
    
    if deviceName == "" {
        deviceName = fmt.Sprintf("%s на %s", browser, os)
    }
    
    // ✅ Используем все колонки которые есть в таблице
    _, err := database.Pool.Exec(ctx, `
        INSERT INTO user_sessions (
            id, user_id, token, device_name, device_type, ip, location, 
            user_agent, created_at, expires_at, last_active, revoked, is_current, tenant_id
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, 
            $8, NOW(), NOW() + INTERVAL '30 days', NOW(), false, true, $9
        )
    `, sessionID, userID, token, deviceName, "browser", ip, "Локальный", userAgent, tenantID)
    
    if err != nil {
        log.Printf("⚠️ Ошибка создания сессии: %v", err)
        return err
    }
    
    log.Printf("✅ Создана сессия %s для пользователя %s (устройство: %s)", sessionID, userID, deviceName)
    return nil
}
// CreateTrustedDevice - создает доверенное устройство
func CreateTrustedDevice(ctx context.Context, userID, tenantID, ip, userAgent, deviceName string) error {
    if deviceName == "" {
        browser := parseBrowser(userAgent)
        os := parseOSAuth(userAgent)
        deviceName = fmt.Sprintf("%s на %s", browser, os)
    }
    
    recordID := uuid.New().String()
    deviceID := "device-" + uuid.New().String()[:8] // Генерируем device_id
    
    _, err := database.Pool.Exec(ctx, `
        INSERT INTO trusted_devices (
            id, user_id, device_id, device_name, ip_address, 
            user_agent, expires_at, last_used_at, created_at, tenant_id
        ) VALUES ($1, $2, $3, $4, $5, $6, NOW() + INTERVAL '365 days', NOW(), NOW(), $7)
    `, recordID, userID, deviceID, deviceName, ip, userAgent, tenantID)
    
    if err != nil {
        log.Printf("⚠️ Ошибка создания доверенного устройства: %v", err)
        return err
    }
    
    log.Printf("✅ Создано доверенное устройство '%s' для пользователя %s", deviceName, userID)
    return nil
}