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

// AddTrustedDevice добавляет устройство в доверенные
func AddTrustedDevice(c *gin.Context) {
    var req struct {
        UserID     string `json:"user_id"`
        DeviceID   string `json:"device_id"`
        DeviceName string `json:"device_name"`
        IP         string `json:"ip"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    expiresAt := time.Now().AddDate(0, 0, 30) // 30 дней

    _, err := database.Pool.Exec(c.Request.Context(),
        `INSERT INTO trusted_devices (user_id, device_id, device_name, ip_address, user_agent, expires_at)
         VALUES ($1, $2, $3, $4, $5, $6)
         ON CONFLICT (user_id, device_id) DO UPDATE 
         SET expires_at = $6, last_used_at = NOW()`,
        req.UserID, req.DeviceID, req.DeviceName, c.ClientIP(), c.GetHeader("User-Agent"), expiresAt)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add device"})
        return
    }

    // Конвертируем userID в UUID для уведомления
    userUUID, _ := uuid.Parse(req.UserID)
    
    // ОТПРАВЛЯЕМ УВЕДОМЛЕНИЕ
    go LogAndNotify(c, userUUID, NotifDeviceTrusted, map[string]interface{}{
        "device": req.DeviceName,
        "ip":     c.ClientIP(),
        "time":   time.Now().Format("02.01.2006 15:04"),
    })

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "message":    "Device added to trusted",
        "expires_at": expiresAt,
    })
}

// RevokeTrustedDevice отзывает доверенное устройство
func RevokeTrustedDevice(c *gin.Context) {
    var req struct {
        UserID   string `json:"user_id" binding:"required"`
        DeviceID string `json:"device_id" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Получаем имя устройства перед удалением
    var deviceName string
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT device_name FROM trusted_devices WHERE user_id = $1 AND device_id = $2",
        req.UserID, req.DeviceID).Scan(&deviceName)

    _, err := database.Pool.Exec(c.Request.Context(),
        "DELETE FROM trusted_devices WHERE user_id = $1 AND device_id = $2",
        req.UserID, req.DeviceID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke device"})
        return
    }

    // Конвертируем userID в UUID для уведомления
    userUUID, _ := uuid.Parse(req.UserID)
    
    // ОТПРАВЛЯЕМ УВЕДОМЛЕНИЕ
    go LogAndNotify(c, userUUID, NotifDeviceRevoked, map[string]interface{}{
        "device": deviceName,
        "time":   time.Now().Format("02.01.2006 15:04"),
    })

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Device access revoked",
    })
}

// GetTrustedDevices возвращает список доверенных устройств
func GetTrustedDevices(c *gin.Context) {
    userID := c.Query("user_id")
    if userID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
        return
    }

    rows, err := database.Pool.Query(c.Request.Context(),
        `SELECT device_id, device_name, ip_address, user_agent, expires_at, last_used_at 
         FROM trusted_devices 
         WHERE user_id = $1 AND expires_at > NOW()
         ORDER BY last_used_at DESC`,
        userID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
        return
    }
    defer rows.Close()

    var devices []gin.H
    for rows.Next() {
        var deviceID, deviceName, ipAddress, userAgent string
        var expiresAt, lastUsedAt time.Time
        rows.Scan(&deviceID, &deviceName, &ipAddress, &userAgent, &expiresAt, &lastUsedAt)
        
        devices = append(devices, gin.H{
            "device_id":   deviceID,
            "device_name": deviceName,
            "ip_address":  ipAddress,
            "user_agent":  userAgent,
            "expires_at":  expiresAt,
            "last_used":   lastUsedAt,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "devices": devices,
    })
}