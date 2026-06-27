package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)

// TrustedDevicesHandler отображает страницу доверенных устройств
func TrustedDevicesHandler(c *gin.Context) {
    userID, exists := c.Get("userID")
    if !exists {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
        return
    }

    c.HTML(http.StatusOK, "trusted_devices.html", gin.H{
        "Title":  "Доверенные устройства | Business Stack",
        "UserID": userID,
    })
}

func AddTrustedDevice(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var req struct {
        DeviceID   string `json:"device_id" binding:"required"`
        DeviceName string `json:"device_name" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // ✅ Генерируем ID для устройства если не передан
    if req.DeviceID == "" {
        req.DeviceID = uuid.New().String()
    }

    expiresAt := time.Now().AddDate(0, 0, 30)

    _, err := database.Pool.Exec(c.Request.Context(),
        `INSERT INTO trusted_devices (id, user_id, device_id, device_name, ip_address, user_agent, expires_at, created_at)
         VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
         ON CONFLICT (user_id, device_id) DO UPDATE 
         SET expires_at = $7, last_used_at = NOW()`,
        uuid.New().String(), userID, req.DeviceID, req.DeviceName, c.ClientIP(), c.GetHeader("User-Agent"), expiresAt)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add device"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "message":    "Device added to trusted",
        "expires_at": expiresAt,
        "device_id":  req.DeviceID,
    })
}

func RevokeTrustedDevice(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
        return
    }

    var req struct {
        DeviceID string `json:"device_id" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // ✅ ИСПРАВЛЕНО: удаляем по device_id (а не по id)
    _, err := database.Pool.Exec(c.Request.Context(),
        "DELETE FROM trusted_devices WHERE device_id = $1 AND user_id = $2",
        req.DeviceID, userID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke device"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Device access revoked",
    })
}
// GetTrustedDevices возвращает список доверенных устройств
func GetTrustedDevices(c *gin.Context) {
    userID := c.GetString("user_id")
    if userID == "" {
        c.JSON(http.StatusOK, gin.H{"devices": []interface{}{}, "success": true})
        return
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT device_id, device_name, ip_address, user_agent, created_at, expires_at, last_used_at
        FROM trusted_devices 
        WHERE user_id = $1
        ORDER BY created_at DESC
    `, userID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"devices": []interface{}{}, "success": true})
        return
    }
    defer rows.Close()

    var devices []gin.H
    for rows.Next() {
        var deviceID, deviceName, ipAddress, userAgent string
        var createdAt, expiresAt, lastUsedAt time.Time

        err := rows.Scan(&deviceID, &deviceName, &ipAddress, &userAgent, &createdAt, &expiresAt, &lastUsedAt)
        if err != nil {
            continue
        }

        devices = append(devices, gin.H{
            "device_id":    deviceID,   // ← ЭТО ID для удаления
            "device_name":  deviceName,
            "ip_address":   ipAddress,
            "user_agent":   userAgent,
            "created_at":   createdAt,
            "expires_at":   expiresAt,
            "last_used_at": lastUsedAt,
            "browser":      parseBrowser(userAgent),
            "os":           parseOS(userAgent),
        })
    }

    c.JSON(http.StatusOK, gin.H{"devices": devices, "success": true})
}