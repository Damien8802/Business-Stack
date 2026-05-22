package handlers

import (
        "context"
	"log"
	"net/http"
	"time"

	"subscription-system/database"
	"subscription-system/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)
func GetAccountID(c *gin.Context) string {
    // Пробуем получить user_id из контекста
    if userID, exists := c.Get("userID"); exists {
        if str, ok := userID.(string); ok && str != "" {
            log.Printf("✅ GetAccountID: userID=%s", str)
            return str
        }
    }
    if userID, exists := c.Get("user_id"); exists {
        if str, ok := userID.(string); ok && str != "" {
            log.Printf("✅ GetAccountID: user_id=%s", str)
            return str
        }
    }
    // В режиме SkipAuth используем существующего пользователя из БД
    var userID string
    err := database.Pool.QueryRow(context.Background(), 
        "SELECT id FROM users WHERE email = 'admin@example.com' LIMIT 1").Scan(&userID)
    if err == nil && userID != "" {
        log.Printf("⚠️ GetAccountID: используем пользователя из БД: %s", userID)
        return userID
    }
    return "a65f3933-c84c-430d-a703-f1247897bbf4"
}
// CreateAgent - создание нового ИИ-агента
func CreateAgent(c *gin.Context) {
	accountID := GetAccountID(c)
	
	var agent models.AIAgent
	if err := c.ShouldBindJSON(&agent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Валидация
	if agent.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "имя агента обязательно"})
		return
	}
	
	// Заполняем обязательные поля
	agent.ID = uuid.New().String()
	agent.AccountID = accountID
	agent.CreatedAt = time.Now()
	agent.UpdatedAt = time.Now()
	
	if agent.Temperature == 0 {
		agent.Temperature = 0.7
	}
	if agent.Schedule == "" {
		agent.Schedule = "24/7"
	}
	if agent.Model == "" {
		agent.Model = "openrouter/auto"
	}
	
	// Сохраняем в БД
	_, err := database.Pool.Exec(c.Request.Context(), `
		INSERT INTO ai_agents (id, user_id, name, role, instructions, model, temperature, schedule, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, agent.ID, agent.AccountID, agent.Name, agent.Role, agent.Instructions,
		agent.Model, agent.Temperature, agent.Schedule, agent.IsActive,
		agent.CreatedAt, agent.UpdatedAt)
	
	if err != nil {
		log.Printf("❌ Ошибка создания агента: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Агент успешно создан",
		"agent":   agent,
	})
}

// GetAgents - список агентов
func GetAgents(c *gin.Context) {
	accountID := GetAccountID(c)
	
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT id, name, role, instructions, model, temperature, schedule, is_active, created_at, updated_at
		FROM ai_agents
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, accountID)
	
	if err != nil {
		log.Printf("❌ Ошибка получения агентов: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	
	var agents []gin.H
	for rows.Next() {
		var id, name, role, instructions, model, schedule string
		var temperature float64
		var isActive bool
		var createdAt, updatedAt time.Time
		
		err := rows.Scan(&id, &name, &role, &instructions, &model, &temperature, &schedule, &isActive, &createdAt, &updatedAt)
		if err != nil {
			continue
		}
		
		agents = append(agents, gin.H{
			"id":           id,
			"name":         name,
			"role":         role,
			"instructions": instructions,
			"model":        model,
			"temperature":  temperature,
			"schedule":     schedule,
			"is_active":    isActive,
			"created_at":   createdAt,
			"updated_at":   updatedAt,
		})
	}
	
	c.JSON(http.StatusOK, gin.H{
		"agents": agents,
	})
}

// UpdateAgent - обновление агента
func UpdateAgent(c *gin.Context) {
	agentID := c.Param("id")
	accountID := GetAccountID(c)
	
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Проверяем, что агент принадлежит аккаунту
	var exists bool
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM ai_agents WHERE id = $1 AND user_id = $2)
	`, agentID, accountID).Scan(&exists)
	
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "агент не найден"})
		return
	}
	
	// Добавляем updated_at
	updates["updated_at"] = time.Now()
	
	// Строим запрос динамически
	query := "UPDATE ai_agents SET "
	args := []interface{}{}
	i := 1
	
	for key, value := range updates {
		if key == "id" || key == "user_id" || key == "created_at" {
			continue
		}
		if i > 1 {
			query += ", "
		}
		query += key + " = $" + string(rune('0'+i))
		args = append(args, value)
		i++
	}
	query += " WHERE id = $" + string(rune('0'+i)) + " AND user_id = $" + string(rune('0'+i+1))
	args = append(args, agentID, accountID)
	
	_, err = database.Pool.Exec(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "агент обновлен"})
}

// DeleteAgent - удаление агента
func DeleteAgent(c *gin.Context) {
	agentID := c.Param("id")
	accountID := GetAccountID(c)
	
	_, err := database.Pool.Exec(c.Request.Context(), `
		DELETE FROM ai_agents WHERE id = $1 AND user_id = $2
	`, agentID, accountID)
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "агент удален"})
}

// AddAgentAction - добавление действия
func AddAgentAction(c *gin.Context) {
	agentID := c.Param("id")
	accountID := GetAccountID(c)
	
	var action struct {
		Action    string `json:"action"`
		Condition string `json:"condition"`
		Config    map[string]interface{} `json:"config"`
	}
	
	if err := c.ShouldBindJSON(&action); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Проверяем агента
	var exists bool
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT EXISTS(SELECT 1 FROM ai_agents WHERE id = $1 AND user_id = $2)
	`, agentID, accountID).Scan(&exists)
	
	if err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "агент не найден"})
		return
	}
	
	_, err = database.Pool.Exec(c.Request.Context(), `
		INSERT INTO ai_agent_actions (id, agent_id, action, condition, config, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, uuid.New().String(), agentID, action.Action, action.Condition, action.Config, time.Now())
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "действие добавлено"})
}

// GetAgentLogs - логи агентов
func GetAgentLogs(c *gin.Context) {
	accountID := GetAccountID(c)
	
	rows, err := database.Pool.Query(c.Request.Context(), `
		SELECT l.id, l.action, l.result, l.status, l.created_at,
		       a.name as agent_name,
		       c.name as customer_name
		FROM ai_agent_logs l
		JOIN ai_agents a ON a.id = l.agent_id
		LEFT JOIN customers c ON c.id = l.customer_id
		WHERE a.user_id = $1
		ORDER BY l.created_at DESC
		LIMIT 100
	`, accountID)
	
	if err != nil {
		log.Printf("❌ Ошибка получения логов: %v", err)
		c.JSON(http.StatusOK, gin.H{"logs": []gin.H{}})
		return
	}
	defer rows.Close()
	
	var logs []gin.H
	for rows.Next() {
		var id, action, result, status, agentName, customerName string
		var createdAt time.Time
		
		err := rows.Scan(&id, &action, &result, &status, &createdAt, &agentName, &customerName)
		if err != nil {
			continue
		}
		
		logs = append(logs, gin.H{
			"id":            id,
			"action":        action,
			"result":        result,
			"status":        status,
			"created_at":    createdAt,
			"agent_name":    agentName,
			"customer_name": customerName,
		})
	}
	
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// GetAgentStats - статистика
func GetAgentStats(c *gin.Context) {
	accountID := GetAccountID(c)
	
	var totalActions, successActions, errorActions, dealsCreated int
	
	err := database.Pool.QueryRow(c.Request.Context(), `
		SELECT 
			COALESCE(COUNT(*), 0) as total,
			COALESCE(COUNT(CASE WHEN status = 'success' THEN 1 END), 0) as success,
			COALESCE(COUNT(CASE WHEN status = 'error' THEN 1 END), 0) as error,
			COALESCE(COUNT(CASE WHEN action = 'create_deal' AND status = 'success' THEN 1 END), 0) as deals
		FROM ai_agent_logs l
		JOIN ai_agents a ON a.id = l.agent_id
		WHERE a.user_id = $1
	`, accountID).Scan(&totalActions, &successActions, &errorActions, &dealsCreated)
	
	if err != nil {
		log.Printf("❌ Ошибка получения статистики: %v", err)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"total_actions":   totalActions,
		"success_actions": successActions,
		"error_actions":   errorActions,
		"deals_created":   dealsCreated,
	})
}

// AIAgentsPage - отображение страницы с агентами
func AIAgentsPage(c *gin.Context) {
	userEmail := c.GetString("userEmail")
	if userEmail == "" {
		userEmail = "admin@example.com"
	}

	userName := c.GetString("userName")
	if userName == "" {
		userName = "Администратор"
	}

	userRole := c.GetString("userRole")
	if userRole == "" {
		userRole = "admin"
	}

	c.HTML(http.StatusOK, "ai_agents.html", gin.H{
		"Title":     "ИИ-агенты - Business Stack",
		"Version":   "3.0",
		"UserEmail": userEmail,
		"UserName":  userName,
		"IsAdmin":   userRole == "admin",
	})
}

// CloneAgent - клонирование агента
func CloneAgent(c *gin.Context) {
    agentID := c.Param("id")
    accountID := GetAccountID(c)

    // Получаем исходного агента
    var agent models.AIAgent
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT name, role, instructions, model, temperature, schedule, is_active
        FROM ai_agents
        WHERE id = $1 AND user_id = $2
    `, agentID, accountID).Scan(&agent.Name, &agent.Role, &agent.Instructions,
        &agent.Model, &agent.Temperature, &agent.Schedule, &agent.IsActive)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "агент не найден"})
        return
    }

    // Создаём копию
    agent.ID = uuid.New().String()
    agent.AccountID = accountID
    agent.Name = agent.Name + " (копия)"
    agent.CreatedAt = time.Now()
    agent.UpdatedAt = time.Now()

    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO ai_agents (id, user_id, name, role, instructions, model, temperature, schedule, is_active, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
    `, agent.ID, agent.AccountID, agent.Name, agent.Role, agent.Instructions,
        agent.Model, agent.Temperature, agent.Schedule, agent.IsActive,
        agent.CreatedAt, agent.UpdatedAt)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "агент скопирован", "agent_id": agent.ID})
}

// ToggleAgentStatus - включение/выключение агента
func ToggleAgentStatus(c *gin.Context) {
    agentID := c.Param("id")
    accountID := GetAccountID(c)

    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE ai_agents 
        SET is_active = NOT is_active, updated_at = NOW()
        WHERE id = $1 AND user_id = $2
    `, agentID, accountID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{"message": "статус агента изменён"})
}

// GetAgentDetails - детальная информация об агенте
func GetAgentDetails(c *gin.Context) {
    agentID := c.Param("id")
    accountID := GetAccountID(c)

    var agent models.AIAgent
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, name, role, instructions, model, temperature, schedule, is_active, created_at, updated_at
        FROM ai_agents
        WHERE id = $1 AND user_id = $2
    `, agentID, accountID).Scan(&agent.ID, &agent.Name, &agent.Role, &agent.Instructions,
        &agent.Model, &agent.Temperature, &agent.Schedule, &agent.IsActive,
        &agent.CreatedAt, &agent.UpdatedAt)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "агент не найден"})
        return
    }

    // Получаем действия агента
    actions, _ := database.Pool.Query(c.Request.Context(), `
        SELECT action, condition, config, is_active, created_at
        FROM ai_agent_actions
        WHERE agent_id = $1
    `, agentID)
    defer actions.Close()

    var actionList []gin.H
    for actions.Next() {
        var action, condition string
        var config interface{}
        var isActive bool
        var createdAt time.Time
        actions.Scan(&action, &condition, &config, &isActive, &createdAt)
        actionList = append(actionList, gin.H{
            "action":    action,
            "condition": condition,
            "config":    config,
            "is_active": isActive,
            "created_at": createdAt,
        })
    }

    // Получаем статистику агента
    var stats struct {
        TotalActions int `json:"total_actions"`
        SuccessRate  float64 `json:"success_rate"`
    }
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT 
            COUNT(*) as total_actions,
            COALESCE(ROUND(AVG(CASE WHEN status = 'success' THEN 100 ELSE 0 END)), 0) as success_rate
        FROM ai_agent_logs
        WHERE agent_id = $1
    `, agentID).Scan(&stats.TotalActions, &stats.SuccessRate)

    c.JSON(http.StatusOK, gin.H{
        "agent":   agent,
        "actions": actionList,
        "stats":   stats,
    })
}

// ExportAgents - экспорт всех агентов в JSON
func ExportAgents(c *gin.Context) {
    accountID := GetAccountID(c)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, role, instructions, model, temperature, schedule, is_active, created_at
        FROM ai_agents
        WHERE user_id = $1
        ORDER BY created_at DESC
    `, accountID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var agents []gin.H
    for rows.Next() {
        var id, name, role, instructions, model, schedule string
        var temperature float64
        var isActive bool
        var createdAt time.Time
        rows.Scan(&id, &name, &role, &instructions, &model, &temperature, &schedule, &isActive, &createdAt)

        agents = append(agents, gin.H{
            "id":           id,
            "name":         name,
            "role":         role,
            "instructions": instructions,
            "model":        model,
            "temperature":  temperature,
            "schedule":     schedule,
            "is_active":    isActive,
            "created_at":   createdAt,
        })
    }

    c.Header("Content-Disposition", "attachment; filename=agents_export.json")
    c.Header("Content-Type", "application/json")
    c.JSON(http.StatusOK, gin.H{"agents": agents})
}
