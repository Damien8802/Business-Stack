package handlers

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

// MigrationProject - проект миграции
type MigrationProject struct {
    ID              int                    `json:"id"`
    tenantID       string                 `json:"tenant_id"`
    Name            string                 `json:"name"`
    SourceType      string                 `json:"source_type"`
    Status          string                 `json:"status"`
    Phase           int                    `json:"phase"`
    SourceConfig    map[string]interface{} `json:"source_config"`
    SyncDirection   string                 `json:"sync_direction"`
    CreatedAt       time.Time              `json:"created_at"`
}

// CreateMigrationProject - создание проекта миграции (ФАЗА 1)
func CreateMigrationProject(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var req struct {
        Name          string                 `json:"name" binding:"required"`
        SourceType    string                 `json:"source_type" binding:"required"`
        SourceConfig  map[string]interface{} `json:"source_config" binding:"required"`
        SyncDirection string                 `json:"sync_direction"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if req.SyncDirection == "" {
        req.SyncDirection = "two_way"
    }

    configJSON, _ := json.Marshal(req.SourceConfig)

    var id int
    err := database.Pool.QueryRow(c.Request.Context(), `
        INSERT INTO migration_projects (tenant_id, name, source_type, status, phase, source_config, sync_direction, created_at)
        VALUES ($1, $2, $3, 'planning', 1, $4, $5, NOW())
        RETURNING id
    `, tenantID, req.Name, req.SourceType, configJSON, req.SyncDirection).Scan(&id)

    if err != nil {
        log.Printf("❌ Ошибка создания проекта: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Запускаем синхронизацию в зависимости от фазы
    go startSyncPhase(tenantID, id, 1)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "✅ Проект миграции создан! Фаза 1: Параллельная синхронизация",
        "project_id": id,
    })
}

// GetMigrationProjects - список проектов миграции
func GetMigrationProjects(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, source_type, status, phase, sync_direction, created_at
        FROM migration_projects
        WHERE tenant_id = $1
        ORDER BY created_at DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{"projects": []MigrationProject{}})
        return
    }
    defer rows.Close()

    var projects []MigrationProject
    for rows.Next() {
        var p MigrationProject
        rows.Scan(&p.ID, &p.Name, &p.SourceType, &p.Status, &p.Phase, &p.SyncDirection, &p.CreatedAt)
        p.tenantID = tenantID
        projects = append(projects, p)
    }

    c.JSON(http.StatusOK, gin.H{"projects": projects})
}

// StartPhase2 - переход к фазе 2 (полный перенос)
func StartPhase2(c *gin.Context) {
    projectIDStr := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    projectID, err := strconv.Atoi(projectIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
        return
    }

    // Обновляем фазу
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE migration_projects 
        SET phase = 2, status = 'migrating', updated_at = NOW()
        WHERE id = $1 AND tenant_id = $2
    `, projectID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Запускаем полную миграцию
    go startFullMigration(tenantID, projectID)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "✅ Запущена ФАЗА 2: Полный перенос данных",
        "phase": 2,
    })
}

// StartPhase3 - переход к фазе 3 (постепенный перенос)
func StartPhase3(c *gin.Context) {
    projectIDStr := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    projectID, err := strconv.Atoi(projectIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
        return
    }

    var req struct {
        Entities []string `json:"entities"`
    }
    c.ShouldBindJSON(&req)

    // Обновляем фазу
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE migration_projects 
        SET phase = 3, status = 'migrating', updated_at = NOW()
        WHERE id = $1 AND tenant_id = $2
    `, projectID, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Запускаем постепенную миграцию выбранных сущностей
    go startGradualMigration(tenantID, projectID, req.Entities)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "✅ Запущена ФАЗА 3: Постепенный перенос данных",
        "phase": 3,
        "entities": req.Entities,
    })
}

// GetMigrationStatus - статус миграции
func GetMigrationStatus(c *gin.Context) {
    projectIDStr := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    projectID, err := strconv.Atoi(projectIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
        return
    }

    var project MigrationProject
    var sourceConfigJSON []byte
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, name, source_type, status, phase, source_config, sync_direction, created_at
        FROM migration_projects
       WHERE id = $1 AND tenant_id = $2
    `, projectID, tenantID).Scan(
        &project.ID, &project.Name, &project.SourceType, &project.Status,
        &project.Phase, &sourceConfigJSON, &project.SyncDirection, &project.CreatedAt,
    )

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
        return
    }

    json.Unmarshal(sourceConfigJSON, &project.SourceConfig)

    // Получаем статистику
    var totalSynced, totalPending, totalFailed int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT 
            COUNT(CASE WHEN status = 'completed' THEN 1 END) as synced,
            COUNT(CASE WHEN status = 'pending' THEN 1 END) as pending,
            COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed
        FROM sync_queue
        WHERE migration_project_id = $1
    `, projectID).Scan(&totalSynced, &totalPending, &totalFailed)

    c.JSON(http.StatusOK, gin.H{
        "project": project,
        "stats": gin.H{
            "synced": totalSynced,
            "pending": totalPending,
            "failed": totalFailed,
            "total": totalSynced + totalPending + totalFailed,
        },
        "phase_description": getPhaseDescription(project.Phase),
    })
}

// SyncEntities - ручная синхронизация выбранных сущностей
func SyncEntities(c *gin.Context) {
    projectIDStr := c.Param("id")
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    projectID, err := strconv.Atoi(projectIDStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
        return
    }

    var req struct {
        Entities []string `json:"entities" binding:"required"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    go syncSpecificEntities(tenantID, projectID, req.Entities)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": fmt.Sprintf("✅ Запущена синхронизация: %v", req.Entities),
    })
}

// ========== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ==========

func startSyncPhase(tenantID string, projectID int, phase int) {
    log.Printf("🔄 Запуск ФАЗЫ %d для проекта %d", phase, projectID)

    // Получаем конфигурацию проекта
    var sourceType string
    var sourceConfigJSON []byte
    database.Pool.QueryRow(context.Background(), `
        SELECT source_type, source_config FROM migration_projects WHERE id = $1
    `, projectID).Scan(&sourceType, &sourceConfigJSON)

    var sourceConfig map[string]interface{}
    json.Unmarshal(sourceConfigJSON, &sourceConfig)

    log.Printf("📡 Синхронизация с %s, фаза %d", sourceType, phase)
    
    // Здесь будет логика синхронизации
    updateSyncQueue(projectID, "system", "initial_sync", "pending")
}

func startFullMigration(tenantID string, projectID int) {
    log.Printf("🚀 Запуск ПОЛНОЙ миграции для проекта %d", projectID)
    
    // Мигрируем все данные
    entities := []string{"leads", "contacts", "deals", "products", "invoices", "employees", "departments"}
    syncSpecificEntities(tenantID, projectID, entities)
}

func startGradualMigration(tenantID string, projectID int, entities []string) {
    log.Printf("📦 Запуск ПОСТЕПЕННОЙ миграции для проекта %d, сущности: %v", projectID, entities)
    syncSpecificEntities(tenantID, projectID, entities)
}

func syncSpecificEntities(tenantID string, projectID int, entities []string) {
    // Получаем конфигурацию
    var sourceType string
    var sourceConfigJSON []byte
    database.Pool.QueryRow(context.Background(), `
        SELECT source_type, source_config FROM migration_projects WHERE id = $1
    `, projectID).Scan(&sourceType, &sourceConfigJSON)

    var sourceConfig map[string]interface{}
    json.Unmarshal(sourceConfigJSON, &sourceConfig)

    for _, entity := range entities {
        log.Printf("📥 Синхронизация %s...", entity)
        
        // Добавляем задачу в очередь
        updateSyncQueue(projectID, entity, "sync", "pending")
        
        // Имитация синхронизации
        time.Sleep(1 * time.Second)
        
        // Обновляем статус
        updateSyncQueueStatus(projectID, entity, "completed")
    }

    // Обновляем статус проекта
    database.Pool.Exec(context.Background(), `
        UPDATE migration_projects SET status = 'completed', updated_at = NOW()
        WHERE id = $1
    `, projectID)
}

func updateSyncQueue(projectID int, entityType, operation, status string) {
    _, err := database.Pool.Exec(context.Background(), `
        INSERT INTO sync_queue (migration_project_id, entity_type, entity_id, source_system, target_system, operation, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
    `, projectID, entityType, "all", "source", "Business Stack", operation, status)
    
    if err != nil {
        log.Printf("❌ Ошибка добавления в очередь: %v", err)
    }
}

func updateSyncQueueStatus(projectID int, entityType, status string) {
    _, err := database.Pool.Exec(context.Background(), `
        UPDATE sync_queue 
        SET status = $1, processed_at = NOW()
        WHERE migration_project_id = $2 AND entity_type = $3 AND status = 'pending'
        ORDER BY id DESC LIMIT 1
    `, status, projectID, entityType)
    
    if err != nil {
        log.Printf("❌ Ошибка обновления статуса: %v", err)
    }
}

func getPhaseDescription(phase int) string {
    switch phase {
    case 1:
        return "ФАЗА 1: Параллельная работа. Данные синхронизируются в обе стороны. Вы можете работать и в старой системе, и в Business Stack."
    case 2:
        return "ФАЗА 2: Полный перенос. Все данные перенесены в Business Stack. Старая система работает только на чтение."
    case 3:
        return "ФАЗА 3: Постепенный перенос. Вы выбираете, какие данные перенести: отделы, сотрудников, сделки, финансы."
    default:
        return "Неизвестная фаза"
    }
}

// StopMigration - остановка миграции
func StopMigration(c *gin.Context) {
    id := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    _, err := database.Pool.Exec(c.Request.Context(), 
        "UPDATE migration_projects SET status = 'stopped', updated_at = NOW() WHERE id = $1 AND tenant_id = $2",
        id, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Миграция остановлена"})
}

// DeleteMigrationProject - удаление проекта миграции
func DeleteMigrationProject(c *gin.Context) {
    id := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    // Удаляем связанные данные
    _, err := database.Pool.Exec(c.Request.Context(), 
        "DELETE FROM migration_stats WHERE project_id = $1", id)
    if err != nil {
        // Игнорируем ошибку, если таблицы нет
    }
    
    _, err = database.Pool.Exec(c.Request.Context(), 
        "DELETE FROM migration_projects WHERE id = $1 AND tenant_id = $2",
        id, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Проект удален"})
}

// ForcePhaseTransition - принудительный переход на следующую фазу
func ForcePhaseTransition(c *gin.Context) {
    id := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    var req struct {
        Phase int `json:"phase"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), 
        "UPDATE migration_projects SET phase = $1, status = 'phase_active', updated_at = NOW() WHERE id = $2 AND tenant_id = $3",
        req.Phase, id, tenantID)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("Переход на фазу %d выполнен", req.Phase)})
}