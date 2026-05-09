package handlers

import (
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "subscription-system/database"
)

// UploadAvatar - загрузка аватарки пользователя
func UploadAvatar(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    // Получаем файл из формы
    file, err := c.FormFile("avatar")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Файл не загружен"})
        return
    }

    // Проверяем размер (макс 2MB)
    if file.Size > 2*1024*1024 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Файл слишком большой. Максимум 2MB"})
        return
    }

    // Проверяем тип файла
    allowedTypes := map[string]bool{
        "image/jpeg": true,
        "image/png":  true,
        "image/gif":  true,
        "image/webp": true,
    }
    contentType := file.Header.Get("Content-Type")
    if !allowedTypes[contentType] {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Разрешены только JPEG, PNG, GIF, WEBP"})
        return
    }

    // Открываем файл
    src, err := file.Open()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось прочитать файл"})
        return
    }
    defer src.Close()

    // Создаем директорию для аватаров
    avatarDir := "static/avatars"
    if err := os.MkdirAll(avatarDir, 0755); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось создать директорию"})
        return
    }

    // Генерируем уникальное имя файла
    ext := filepath.Ext(file.Filename)
    filename := userID + "_" + uuid.New().String()[:8] + ext
    filePath := filepath.Join(avatarDir, filename)

    // Сохраняем файл
    dst, err := os.Create(filePath)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось сохранить файл"})
        return
    }
    defer dst.Close()

    if _, err := io.Copy(dst, src); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка при сохранении"})
        return
    }

    // URL для доступа к аватарке
    avatarURL := "/static/avatars/" + filename

    // Удаляем старый аватар, если есть
    var oldAvatar string
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT avatar_url FROM users WHERE id = $1", userID,
    ).Scan(&oldAvatar)
    
    if oldAvatar != "" {
        oldPath := strings.TrimPrefix(oldAvatar, "/")
        os.Remove(oldPath)
    }

    // Обновляем в базе данных
    _, err = database.Pool.Exec(c.Request.Context(),
        "UPDATE users SET avatar_url = $1 WHERE id = $2",
        avatarURL, userID,
    )
    if err != nil {
        os.Remove(filePath)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось обновить аватар"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "avatar_url": avatarURL,
        "message":    "Аватар загружен",
    })
}

// DeleteAvatar - удалить аватар
func DeleteAvatar(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    // Получаем текущий аватар
    var avatarURL string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT avatar_url FROM users WHERE id = $1", userID,
    ).Scan(&avatarURL)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка"})
        return
    }

    // Удаляем файл
    if avatarURL != "" {
        filePath := strings.TrimPrefix(avatarURL, "/")
        os.Remove(filePath)
    }

    // Очищаем в базе
    _, err = database.Pool.Exec(c.Request.Context(),
        "UPDATE users SET avatar_url = NULL WHERE id = $1", userID,
    )
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Аватар удален",
    })
}

// GetUserAvatar - получить аватар пользователя (для других)
func GetUserAvatar(c *gin.Context) {
    userID := c.Param("id")
    
    var avatarURL *string
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT avatar_url FROM users WHERE id = $1", userID,
    ).Scan(&avatarURL)

    if err != nil || avatarURL == nil {
        c.JSON(http.StatusOK, gin.H{
            "avatar_url": "",
            "initials":   true,
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "avatar_url": *avatarURL,
    })
}