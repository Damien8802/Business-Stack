package handlers

import (
    "archive/zip"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)
// ========== СТРУКТУРЫ ==========
type BackupData struct {
    Customers    []Customer    `json:"customers"`
    Deals        []Deal        `json:"deals"`
    Tags         []Tag         `json:"tags"`
    Activities   []Activity    `json:"activities"`
    BackupDate   time.Time     `json:"backup_date"`
}

type FullBackupData struct {
    BackupDate      time.Time              `json:"backup_date"`
    Version         string                 `json:"version"`
    Database        map[string]interface{} `json:"database"`
    Files           []string               `json:"files"`
    CloudFiles      []string               `json:"cloud_files"`
    Settings        map[string]interface{} `json:"settings"`
}

// ========== CRM БЭКАП ==========
func CreateBackup(c *gin.Context) {
    backup := BackupData{
        BackupDate: time.Now(),
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, email, phone, company, status, responsible, 
               source, comment, user_id, lead_score, created_at, last_seen,
               city, social_media, birthday, notes
        FROM crm_customers
    `)
    if err == nil {
        for rows.Next() {
            var cst Customer
            var socialMedia []byte
            var birthday *time.Time
            rows.Scan(&cst.ID, &cst.Name, &cst.Email, &cst.Phone, &cst.Company,
                &cst.Status, &cst.Responsible, &cst.Source, &cst.Comment, &cst.UserID,
                &cst.LeadScore, &cst.CreatedAt, &cst.LastSeen, &cst.City,
                &socialMedia, &birthday, &cst.Notes)
            cst.Birthday = birthday
            backup.Customers = append(backup.Customers, cst)
        }
        rows.Close()
    }
    
    rows, err = database.Pool.Query(c.Request.Context(), `
        SELECT id, customer_id, title, value, stage, probability, responsible,
               source, comment, user_id, expected_close, created_at, closed_at,
               product_category, discount, next_action_date
        FROM crm_deals
    `)
    if err == nil {
        for rows.Next() {
            var d Deal
            var nextActionDate *time.Time
            rows.Scan(&d.ID, &d.CustomerID, &d.Title, &d.Value, &d.Stage,
                &d.Probability, &d.Responsible, &d.Source, &d.Comment, &d.UserID,
                &d.ExpectedClose, &d.CreatedAt, &d.ClosedAt, &d.ProductCategory,
                &d.Discount, &nextActionDate)
            d.NextActionDate = nextActionDate
            backup.Deals = append(backup.Deals, d)
        }
        rows.Close()
    }
    
    rows, err = database.Pool.Query(c.Request.Context(), `
        SELECT id, name, color, created_at FROM tags
    `)
    if err == nil {
        for rows.Next() {
            var t Tag
            rows.Scan(&t.ID, &t.Name, &t.Color, &t.CreatedAt)
            backup.Tags = append(backup.Tags, t)
        }
        rows.Close()
    }
    
    jsonData, err := json.MarshalIndent(backup, "", "  ")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания бэкапа"})
        return
    }
    
    filename := fmt.Sprintf("crm_backup_%s.json", time.Now().Format("20060102_150405"))
    c.Header("Content-Disposition", "attachment; filename="+filename)
    c.Header("Content-Type", "application/json")
    c.Data(http.StatusOK, "application/json", jsonData)
}

func RestoreBackup(c *gin.Context) {
    file, err := c.FormFile("backup")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Файл не загружен"})
        return
    }
    
    src, err := file.Open()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка чтения файла"})
        return
    }
    defer src.Close()
    
    var backup BackupData
    decoder := json.NewDecoder(src)
    if err := decoder.Decode(&backup); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат бэкапа"})
        return
    }
    
    tx, err := database.Pool.Begin(c.Request.Context())
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка БД"})
        return
    }
    defer tx.Rollback(c.Request.Context())
    
    tx.Exec(c.Request.Context(), "TRUNCATE crm_deals, crm_customers, tags, activities CASCADE")
    
    for _, cust := range backup.Customers {
        tx.Exec(c.Request.Context(), `
            INSERT INTO crm_customers (id, name, email, phone, company, status, 
                responsible, source, comment, user_id, lead_score, created_at, 
                last_seen, city, social_media, birthday, notes)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
        `, cust.ID, cust.Name, cust.Email, cust.Phone, cust.Company, cust.Status,
            cust.Responsible, cust.Source, cust.Comment, cust.UserID, cust.LeadScore,
            cust.CreatedAt, cust.LastSeen, cust.City, cust.SocialMedia, cust.Birthday, cust.Notes)
    }
    
    for _, deal := range backup.Deals {
        tx.Exec(c.Request.Context(), `
            INSERT INTO crm_deals (id, customer_id, title, value, stage, probability,
                responsible, source, comment, user_id, expected_close, created_at,
                closed_at, product_category, discount, next_action_date)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
        `, deal.ID, deal.CustomerID, deal.Title, deal.Value, deal.Stage, deal.Probability,
            deal.Responsible, deal.Source, deal.Comment, deal.UserID, deal.ExpectedClose,
            deal.CreatedAt, deal.ClosedAt, deal.ProductCategory, deal.Discount, deal.NextActionDate)
    }
    
    for _, tag := range backup.Tags {
        tx.Exec(c.Request.Context(), `
            INSERT INTO tags (id, name, color, created_at)
            VALUES ($1, $2, $3, $4)
        `, tag.ID, tag.Name, tag.Color, tag.CreatedAt)
    }
    
    if err := tx.Commit(c.Request.Context()); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка восстановления"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Бэкап восстановлен"})
}

// ========== НАСТРОЙКИ БЭКАПА ==========
func GetBackupSettings(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var id string
    var databaseBackup, filesBackup, isActive bool
    var backupFrequency, backupTime, storageType, storagePath string
    var retentionDays int
    var lastBackupAt *time.Time

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, database_backup, files_backup, backup_frequency, backup_time, 
               retention_days, storage_type, storage_path, last_backup_at, is_active
        FROM backup_settings
        WHERE company_id = $1
    `, companyID).Scan(
        &id, &databaseBackup, &filesBackup, &backupFrequency, &backupTime,
        &retentionDays, &storageType, &storagePath, &lastBackupAt, &isActive,
    )

    if err != nil {
        newID := uuid.New().String()
        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO backup_settings (id, company_id, database_backup, files_backup, backup_frequency, backup_time, retention_days, storage_type, is_active)
            VALUES ($1, $2, true, true, 'daily', '02:00:00', 30, 'local', true)
        `, newID, companyID)
        
        if err != nil {
            c.JSON(http.StatusOK, gin.H{
                "database_backup": true,
                "files_backup": true,
                "backup_frequency": "daily",
                "backup_time": "02:00",
                "retention_days": 30,
                "storage_type": "local",
            })
            return
        }
        
        c.JSON(http.StatusOK, gin.H{
            "id": newID,
            "database_backup": true,
            "files_backup": true,
            "backup_frequency": "daily",
            "backup_time": "02:00",
            "retention_days": 30,
            "storage_type": "local",
            "is_active": true,
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "id": id,
        "database_backup": databaseBackup,
        "files_backup": filesBackup,
        "backup_frequency": backupFrequency,
        "backup_time": backupTime[:5],
        "retention_days": retentionDays,
        "storage_type": storageType,
        "storage_path": storagePath,
        "last_backup_at": lastBackupAt,
        "is_active": isActive,
    })
}
// UpdateBackupSettings - обновить настройки бэкапа
func UpdateBackupSettings(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var req struct {
        DatabaseBackup  bool   `json:"database_backup"`
        FilesBackup     bool   `json:"files_backup"`
        BackupFrequency string `json:"backup_frequency"`
        BackupTime      string `json:"backup_time"`
        RetentionDays   int    `json:"retention_days"`
        StorageType     string `json:"storage_type"`
        StoragePath     string `json:"storage_path"`
        IsActive        bool   `json:"is_active"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Если время не указано, ставим 02:00:00
    backupTime := req.BackupTime
    if backupTime == "" {
        backupTime = "02:00:00"
    }
    // Добавляем секунды если их нет
    if len(backupTime) == 5 {
        backupTime = backupTime + ":00"
    }

    // Проверяем, есть ли запись
    var exists bool
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM backup_settings WHERE company_id = $1)
    `, companyID).Scan(&exists)

    if !exists {
        newID := uuid.New().String()
        _, err := database.Pool.Exec(c.Request.Context(), `
            INSERT INTO backup_settings (id, company_id, database_backup, files_backup, backup_frequency, backup_time, retention_days, storage_type, is_active, created_at)
            VALUES ($1, $2, $3, $4, $5, $6::time, $7, $8, $9, NOW())
        `, newID, companyID, req.DatabaseBackup, req.FilesBackup, req.BackupFrequency, backupTime,
            req.RetentionDays, req.StorageType, req.IsActive)
        
        if err != nil {
            log.Printf("❌ Ошибка создания настроек: %v", err)
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create settings"})
            return
        }
        
        c.JSON(http.StatusOK, gin.H{"message": "Настройки сохранены"})
        return
    }

    // Обновляем существующую запись
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE backup_settings 
        SET database_backup = $1, files_backup = $2, backup_frequency = $3, 
            backup_time = $4::time, retention_days = $5, storage_type = $6, 
            is_active = $7, updated_at = NOW()
        WHERE company_id = $8
    `, req.DatabaseBackup, req.FilesBackup, req.BackupFrequency, backupTime,
        req.RetentionDays, req.StorageType, req.IsActive, companyID)

    if err != nil {
        log.Printf("❌ Ошибка обновления настроек: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update settings"})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "Настройки сохранены"})
}
// ========== ПОЛНЫЙ БЭКАП ==========
func CreateFullBackup(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    backupID := uuid.New().String()
    backupDir := fmt.Sprintf("./backups/%s", companyID)
    os.MkdirAll(backupDir, 0755)
    
    backupFile := fmt.Sprintf("%s/full_backup_%s_%d.zip", backupDir, companyID[:8], time.Now().Unix())
    
    zipFile, err := os.Create(backupFile)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create backup file"})
        return
    }
    defer zipFile.Close()
    
    zipWriter := zip.NewWriter(zipFile)
    defer zipWriter.Close()
    
    dbBackupFile := fmt.Sprintf("%s/db_backup_%d.sql", backupDir, time.Now().Unix())
    cmd := exec.Command("pg_dump", "-U", "postgres", "-d", "Business Stack", "-f", dbBackupFile)
    if err := cmd.Run(); err == nil {
        addFileToZip(zipWriter, dbBackupFile, "database.sql")
        os.Remove(dbBackupFile)
    }
    
    if _, err := os.Stat(".env"); err == nil {
        addFileToZip(zipWriter, ".env", "config/.env")
    }
    
    cloudStoragePath := "./cloud_storage"
    if _, err := os.Stat(cloudStoragePath); err == nil {
        addDirToZip(zipWriter, cloudStoragePath, "cloud_storage")
    }
    
    zipWriter.Close()
    
    fileInfo, _ := os.Stat(backupFile)
    fileSize := int64(0)
    if fileInfo != nil {
        fileSize = fileInfo.Size()
    }
    
    database.Pool.Exec(c.Request.Context(), `
        INSERT INTO backup_history (id, company_id, backup_type, backup_size, file_path, status, completed_at)
        VALUES ($1, $2, 'full', $3, $4, 'success', NOW())
    `, backupID, companyID, fileSize, backupFile)
    
    database.Pool.Exec(c.Request.Context(), `
        UPDATE backup_settings SET last_backup_at = NOW() WHERE company_id = $1
    `, companyID)
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Полный бэкап создан",
        "file_path": backupFile,
        "size": fileSize,
    })
}

func GetBackupHistory(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, backup_type, backup_size, file_path, status, completed_at, created_at
        FROM backup_history
        WHERE company_id = $1
        ORDER BY created_at DESC
        LIMIT 50
    `, companyID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load history"})
        return
    }
    defer rows.Close()

    var backups []gin.H
    for rows.Next() {
        var id, backupType, filePath, status string
        var backupSize int64
        var completedAt, createdAt time.Time

        rows.Scan(&id, &backupType, &backupSize, &filePath, &status, &completedAt, &createdAt)

        backups = append(backups, gin.H{
            "id": id,
            "type": backupType,
            "size": backupSize,
            "size_mb": float64(backupSize) / 1024 / 1024,
            "file_path": filePath,
            "status": status,
            "completed_at": completedAt.Format("2006-01-02 15:04:05"),
            "created_at": createdAt.Format("2006-01-02 15:04:05"),
        })
    }

    c.JSON(http.StatusOK, gin.H{"backups": backups})
}

func DownloadBackup(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    backupID := c.Param("id")
    
    var filePath string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT file_path FROM backup_history
        WHERE id = $1 AND company_id = $2
    `, backupID, companyID).Scan(&filePath)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Backup not found"})
        return
    }

    c.FileAttachment(filePath, filepath.Base(filePath))
}

func DeleteBackup(c *gin.Context) {
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    backupID := c.Param("id")
    
    var filePath string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT file_path FROM backup_history
        WHERE id = $1 AND company_id = $2
    `, backupID, companyID).Scan(&filePath)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Backup not found"})
        return
    }

    os.Remove(filePath)
    
    database.Pool.Exec(c.Request.Context(), `
        DELETE FROM backup_history WHERE id = $1 AND company_id = $2
    `, backupID, companyID)

    c.JSON(http.StatusOK, gin.H{"message": "Backup deleted"})
}

// ========== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ==========
func addFileToZip(zipWriter *zip.Writer, filePath, zipPath string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    info, err := file.Stat()
    if err != nil {
        return err
    }

    header, err := zip.FileInfoHeader(info)
    if err != nil {
        return err
    }
    header.Name = zipPath
    header.Method = zip.Deflate

    writer, err := zipWriter.CreateHeader(header)
    if err != nil {
        return err
    }

    _, err = io.Copy(writer, file)
    return err
}

func addDirToZip(zipWriter *zip.Writer, dirPath, zipPath string) error {
    return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if info.IsDir() {
            return nil
        }
        relPath, err := filepath.Rel(dirPath, path)
        if err != nil {
            return err
        }
        return addFileToZip(zipWriter, path, filepath.Join(zipPath, relPath))
    })
}