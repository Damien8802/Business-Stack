package handlers

import (
    "fmt"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"
    
    "github.com/gin-gonic/gin"
    
    "subscription-system/database"
    "subscription-system/middleware"
)

// GetCompanySettings - получение настроек компании
func GetCompanySettings(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    
    var companyName, inn, kpp, ogrn, address, phone, email, stampURL string
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(company_name, ''),
               COALESCE(inn, ''),
               COALESCE(kpp, ''),
               COALESCE(ogrn, ''),
               COALESCE(address, ''),
               COALESCE(phone, ''),
               COALESCE(email, ''),
               COALESCE(stamp_url, '')
        FROM company_settings
        WHERE tenant_id = $1
    `, tenantID).Scan(&companyName, &inn, &kpp, &ogrn, &address, &phone, &email, &stampURL)
    
    if err != nil {
        // Возвращаем пустые значения если нет настроек
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "data": gin.H{
                "company_name": "",
                "inn":          "",
                "kpp":          "",
                "ogrn":         "",
                "address":      "",
                "phone":        "",
                "email":        "",
                "stamp_url":    "",
            },
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data": gin.H{
            "company_name": companyName,
            "inn":          inn,
            "kpp":          kpp,
            "ogrn":         ogrn,
            "address":      address,
            "phone":        phone,
            "email":        email,
            "stamp_url":    stampURL,
        },
    })
}

// UpdateCompanyDetails - обновление реквизитов компании
func UpdateCompanyDetails(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    
    var req struct {
        CompanyName string `json:"company_name"`
        INN         string `json:"inn"`
        KPP         string `json:"kpp"`
        OGRN        string `json:"ogrn"`
        Address     string `json:"address"`
        Phone       string `json:"phone"`
        Email       string `json:"email"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO company_settings (tenant_id, company_name, inn, kpp, ogrn, address, phone, email, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
        ON CONFLICT (tenant_id) DO UPDATE SET
            company_name = $2,
            inn = $3,
            kpp = $4,
            ogrn = $5,
            address = $6,
            phone = $7,
            email = $8,
            updated_at = NOW()
    `, tenantID, req.CompanyName, req.INN, req.KPP, req.OGRN, req.Address, req.Phone, req.Email)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения настроек"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Настройки сохранены"})
}

// UploadCompanyStamp - загрузка печати/логотипа компании
func UploadCompanyStamp(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    
    // Получаем файл
    file, err := c.FormFile("stamp")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Файл не загружен"})
        return
    }
    
    // Проверяем расширение
    ext := strings.ToLower(filepath.Ext(file.Filename))
    allowedExt := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".svg": true}
    if !allowedExt[ext] {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Разрешены только PNG, JPG, JPEG, SVG"})
        return
    }
    
    // Проверяем размер (макс 2MB)
    if file.Size > 2*1024*1024 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Максимальный размер файла 2MB"})
        return
    }
    
    // Создаем папку если нет
    stampDir := "./static/uploads/stamps"
    if err := os.MkdirAll(stampDir, 0755); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания папки"})
        return
    }
    
    // Генерируем имя файла
    filename := fmt.Sprintf("%s_%d%s", tenantID, time.Now().Unix(), ext)
    filePath := filepath.Join(stampDir, filename)
    
    // Сохраняем файл
    if err := c.SaveUploadedFile(file, filePath); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения файла"})
        return
    }
    
    stampURL := fmt.Sprintf("/static/uploads/stamps/%s", filename)
    
    // Сохраняем в БД
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO company_settings (tenant_id, stamp_url, stamp_updated_at, updated_at)
        VALUES ($1, $2, NOW(), NOW())
        ON CONFLICT (tenant_id) DO UPDATE SET
            stamp_url = $2,
            stamp_updated_at = NOW(),
            updated_at = NOW()
    `, tenantID, stampURL)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сохранения в БД"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":   true,
        "stamp_url": stampURL,
        "message":   "Печать загружена",
    })
}