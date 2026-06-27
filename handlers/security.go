package handlers

import (
    "net/http"
     "strings"
    "time"
    "subscription-system/database"
    "github.com/gin-gonic/gin"
)

// GetUserSessions - получение активных сессий пользователя
func GetUserSessions(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusOK, gin.H{"sessions": []interface{}{}})
        return
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, user_agent, ip, created_at, last_active, is_current
        FROM user_sessions
        WHERE user_id = $1
        ORDER BY last_active DESC
    `, userID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"sessions": []interface{}{}})
        return
    }
    defer rows.Close()

    var sessions []gin.H
    for rows.Next() {
        var id string           // ← ИСПРАВЛЕНО: string вместо int
        var userAgent, ip string
        var createdAt, lastActive time.Time
        var isCurrent bool

        err := rows.Scan(&id, &userAgent, &ip, &createdAt, &lastActive, &isCurrent)
        if err != nil {
            continue
        }

        sessions = append(sessions, gin.H{
            "id":          id,
            "browser":     parseBrowser(userAgent),
            "os":          parseOS(userAgent),
            "ip":          ip,
            "created_at":  createdAt,
            "last_active": lastActive,
            "is_current":  isCurrent,
        })
    }

    c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

// TerminateSession - завершение конкретной сессии
func TerminateSession(c *gin.Context) {
    sessionID := c.Param("id")
    userID := c.GetString("user_id")

    _, err := database.Pool.Exec(c.Request.Context(),
        "DELETE FROM user_sessions WHERE id = $1 AND user_id = $2",
        sessionID, userID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}
// TerminateAllSessions - завершение всех сессий пользователя
func TerminateAllSessions(c *gin.Context) {
    userID := c.GetString("user_id")

    _, err := database.Pool.Exec(c.Request.Context(),
        "DELETE FROM user_sessions WHERE user_id = $1",
        userID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}
// GetLoginHistory - история входов пользователя
func GetLoginHistory(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusOK, gin.H{"history": []interface{}{}})
        return
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, ip_address, user_agent, login_time
        FROM login_history
        WHERE user_id = $1
        ORDER BY login_time DESC
        LIMIT 50
    `, userID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"history": []interface{}{}})
        return
    }
    defer rows.Close()

    var history []gin.H
    for rows.Next() {
        var id string           // ← ИСПРАВЛЕНО: string вместо int
        var ip, userAgent string
        var loginTime time.Time

        err := rows.Scan(&id, &ip, &userAgent, &loginTime)
        if err != nil {
            continue
        }

        history = append(history, gin.H{
            "id":         id,
            "ip":         ip,
            "browser":    parseBrowser(userAgent),
            "os":         parseOS(userAgent),
            "success":    true,
            "created_at": loginTime,
        })
    }

    c.JSON(http.StatusOK, gin.H{"history": history})
}
// GetUserSettings - настройки пользователя
func GetUserSettings(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusOK, gin.H{"max_sessions": 5})
        return
    }

    var maxSessions int
    err := database.Pool.QueryRow(c.Request.Context(),
        "SELECT COALESCE(max_sessions, 5) FROM users WHERE id = $1", userID).Scan(&maxSessions)
    if err != nil {
        maxSessions = 5
    }

    c.JSON(http.StatusOK, gin.H{"max_sessions": maxSessions})
}

// SetMaxSessions - установка лимита сессий
func SetMaxSessions(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var req struct {
        MaxSessions int `json:"max_sessions"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(),
        "UPDATE users SET max_sessions = $1 WHERE id = $2",
        req.MaxSessions, userID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"success": true})
}

func parseBrowser(userAgent string) string {
    ua := strings.ToLower(userAgent)
    if strings.Contains(ua, "chrome") && !strings.Contains(ua, "edg") && !strings.Contains(ua, "opr") {
        return "Chrome"
    }
    if strings.Contains(ua, "firefox") {
        return "Firefox"
    }
    if strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome") {
        return "Safari"
    }
    if strings.Contains(ua, "edg") {
        return "Edge"
    }
    if strings.Contains(ua, "opera") || strings.Contains(ua, "opr") {
        return "Opera"
    }
    if strings.Contains(ua, "yandex") {
        return "Yandex"
    }
    return "Другой"
}
func parseOS(userAgent string) string {
    ua := strings.ToLower(userAgent)
    if strings.Contains(ua, "windows") {
        return "Windows"
    }
    if strings.Contains(ua, "mac") || strings.Contains(ua, "macintosh") {
        return "macOS"
    }
    if strings.Contains(ua, "linux") && !strings.Contains(ua, "android") {
        return "Linux"
    }
    if strings.Contains(ua, "android") {
        return "Android"
    }
    if strings.Contains(ua, "ios") || strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") {
        return "iOS"
    }
    return "Другая"
}
func stringContains(s, substr string) bool {
    return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr))
}

