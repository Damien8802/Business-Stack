package handlers

import (
    //"context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "net/url"
    "os"
    "sort"
    "strconv"
    "strings"
    "time"

    "subscription-system/database"
    "subscription-system/models"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

// ValidateWebAppData проверяет подпись данных от Telegram
func ValidateWebAppData(initData string, botToken string) (int64, error) {
    values, err := url.ParseQuery(initData)
    if err != nil {
        return 0, err
    }

    hash := values.Get("hash")
    values.Del("hash")

    keys := make([]string, 0, len(values))
    for k := range values {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    var dataCheck []string
    for _, k := range keys {
        for _, v := range values[k] {
            dataCheck = append(dataCheck, fmt.Sprintf("%s=%s", k, v))
        }
    }
    dataCheckString := strings.Join(dataCheck, "\n")

    secret := hmac.New(sha256.New, []byte("WebAppData"))
    secret.Write([]byte(botToken))
    secretKey := secret.Sum(nil)

    h := hmac.New(sha256.New, secretKey)
    h.Write([]byte(dataCheckString))
    expectedHash := hex.EncodeToString(h.Sum(nil))

    if expectedHash != hash {
        return 0, fmt.Errorf("invalid hash")
    }

    authDate, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
    if err != nil {
        return 0, err
    }
    if time.Now().Unix()-authDate > 86400 {
        return 0, fmt.Errorf("auth date too old")
    }

    userStr := values.Get("user")
    var user struct {
        ID int64 `json:"id"`
    }
    if err := json.Unmarshal([]byte(userStr), &user); err != nil {
        return 0, err
    }

    return user.ID, nil
}

// WebAppAuthHandler обрабатывает авторизацию через Telegram Mini App
func WebAppAuthHandler(c *gin.Context) {
    var req struct {
        InitData string `json:"initData"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    var telegramID int64
    var err error

    // Если initData присутствует, проверяем подпись
    if req.InitData != "" {
        botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
        if botToken == "" {
            c.JSON(500, gin.H{"error": "bot token not configured"})
            return
        }
        telegramID, err = ValidateWebAppData(req.InitData, botToken)
        if err != nil {
            c.JSON(401, gin.H{"error": "invalid initData"})
            return
        }
    } else {
        // Если initData нет, проверяем режим разработки
        devMode := os.Getenv("DEV_MODE") == "true"
        if !devMode {
            c.JSON(400, gin.H{"error": "initData required in production mode"})
            return
        }
        // В режиме разработки используем фиксированный Telegram ID
        telegramID = 1977550186
    }

    var userID string
    err = database.Pool.QueryRow(c.Request.Context(),
        `SELECT id FROM users WHERE telegram_id = $1`, telegramID).Scan(&userID)

    if err != nil {
        // Пользователь не найден — создаём нового
        email := fmt.Sprintf("tg_%d@placeholder.com", telegramID)
        name := fmt.Sprintf("user_%d", telegramID)
        randomPass := uuid.New().String()[:12]

        // Создаём пользователя
        tenantID := c.GetString("tenant_id")
if tenantID == "" {
    tenantID = uuid.New().String()
}
newUser, err := models.CreateUser(email, randomPass, name, tenantID)

        if err != nil {
            c.JSON(500, gin.H{"error": "failed to create user"})
            return
        }
        userID = newUser.ID.String()

        // Обновляем telegram_id
        _, err = database.Pool.Exec(c.Request.Context(),
            `UPDATE users SET telegram_id = $1 WHERE id = $2`, telegramID, userID)
        if err != nil {
            c.JSON(500, gin.H{"error": "failed to set telegram_id"})
            return
        }
    }

    // Получаем данные пользователя
    user, err := models.GetUserByID(userID)
    if err != nil {
        c.JSON(500, gin.H{"error": "user not found"})
        return
    }

    // Временно отключаем проверку подписки
    var subscription interface{}
    // TODO: добавить проверку подписки позже

    c.JSON(200, gin.H{
        "user": gin.H{
            "id":         user.ID,
            "email":      user.Email,
            "name":       user.Name,
            "role":       user.Role,
            "created_at": user.CreatedAt,
        },
        "subscription": subscription,
    })
}
