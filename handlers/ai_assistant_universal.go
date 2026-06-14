package handlers

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "strings"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
    
    "subscription-system/services"
)

// Простое хранилище сессий (в памяти)
var chatSessions = make(map[string]map[string]interface{})

// UniversalAIAssistant - универсальный AI ассистент
type UniversalAIAssistant struct {
    yandexAPIKey     string
    yandexFolderID   string
    telegramBotToken string
    telegramChatID   string
    adminChatID      string
    db               *pgxpool.Pool
    knowledgeBase    *services.KnowledgeBase
    actionExecutor   *services.AIActionExecutor
}

// NewUniversalAIAssistant - создаёт нового AI ассистента
func NewUniversalAIAssistant(yandexAPIKey, yandexFolderID, telegramBotToken, telegramChatID, adminChatID string, db *pgxpool.Pool) *UniversalAIAssistant {
    return &UniversalAIAssistant{
        yandexAPIKey:     yandexAPIKey,
        yandexFolderID:   yandexFolderID,
        telegramBotToken: telegramBotToken,
        telegramChatID:   telegramChatID,
        adminChatID:      adminChatID,
        db:               db,
        knowledgeBase:    services.NewKnowledgeBase(db),
        actionExecutor:   services.NewAIActionExecutor(db),
    }
}

// getTenantID - безопасное получение tenant ID из контекста
func (ai *UniversalAIAssistant) getTenantID(c *gin.Context) string {
    if tenantID := c.GetString("tenant_id_string"); tenantID != "" && tenantID != "default" {
        if _, err := uuid.Parse(tenantID); err == nil {
            return tenantID
        }
    }
    
    if val, exists := c.Get("tenant_id"); exists {
        switch v := val.(type) {
        case uuid.UUID:
            if v != uuid.Nil {
                return v.String()
            }
        case string:
            if v != "" && v != "default" {
                if _, err := uuid.Parse(v); err == nil {
                    return v
                }
            }
        }
    }
    
    if headerID := c.GetHeader("X-Tenant-ID"); headerID != "" && headerID != "default" {
        if _, err := uuid.Parse(headerID); err == nil {
            return headerID
        }
    }
    
    if queryID := c.Query("tenant_id"); queryID != "" && queryID != "default" {
        if _, err := uuid.Parse(queryID); err == nil {
            return queryID
        }
    }
    
    return ""
}

// ChatHandler - обрабатывает чат сообщения
func (ai *UniversalAIAssistant) ChatHandler(c *gin.Context) {
    var req struct {
        Message string `json:"message"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    tenantID := ai.getTenantID(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{
            "success": false,
            "error":   "Ошибка авторизации",
        })
        return
    }
    
    userID := c.GetString("user_id")
    if userID == "" {
        userID = c.GetString("user_id_from_token")
    }
    if userID == "" {
        userID = "system"
    }

    log.Printf("🔍 [ChatHandler] message: %s", req.Message)

    // Проверяем является ли сообщение командой
    msg := strings.ToLower(req.Message)
    isCommand := strings.Contains(msg, "создай") || 
                 strings.Contains(msg, "покажи") ||
                 strings.Contains(msg, "архивируй") ||
                 strings.Contains(msg, "восстанови") ||
                 strings.Contains(msg, "экспортируй") ||
                 strings.Contains(msg, "закрой") ||
                 strings.Contains(msg, "рассчитай") ||
                 strings.Contains(msg, "сформируй") ||
                 strings.Contains(msg, "скачай")

    if isCommand {
        intent := services.DetectIntent(req.Message)
        
        if intent.Action != "" && intent.Action != "chat" {
            log.Printf("⚡ [ChatHandler] Действие: %s", intent.Action)
            
            entities := services.ExtractEntities(req.Message)
            
            sessionKey := "chat_" + userID
            if session, exists := chatSessions[sessionKey]; exists {
                if recipient, ok := session["recipient"].(string); ok && recipient != "" && entities["recipient"] == "" {
                    entities["recipient"] = recipient
                }
                if amount, ok := session["amount"].(string); ok && amount != "" && entities["amount"] == "" {
                    entities["amount"] = amount
                }
                if purpose, ok := session["purpose"].(string); ok && purpose != "" && entities["purpose"] == "" {
                    entities["purpose"] = purpose
                }
            }
            
            result := ai.actionExecutor.ExecuteAction(tenantID, userID, intent, entities)
            
            if intent.Action == "create_payment" && result.Data != nil {
                if dataMap, ok := result.Data.(map[string]interface{}); ok {
                    if step, hasStep := dataMap["step"]; hasStep && step != nil {
                        chatSessions[sessionKey] = map[string]interface{}{
                            "step":      dataMap["step"],
                            "recipient": dataMap["recipient"],
                            "amount":    dataMap["amount"],
                            "purpose":   dataMap["purpose"],
                        }
                    } else if _, completed := dataMap["completed"]; completed {
                        delete(chatSessions, sessionKey)
                    }
                }
            }
            
            c.JSON(200, gin.H{
                "response": result.Message,
                "success":  result.Success,
            })
            return
        }
    }

    // ========== ДЛЯ ВСЕХ ОСТАЛЬНЫХ ВОПРОСОВ - YANDEX GPT ==========
    
    // Получаем историю диалога
    rows, err := ai.db.Query(context.Background(), `
        SELECT message, response 
        FROM ai_chat_history 
        WHERE tenant_id = $1 AND user_id = $2
        ORDER BY created_at DESC 
        LIMIT 10
    `, tenantID, userID)
    if err == nil {
        defer rows.Close()
        var history []map[string]string
        for rows.Next() {
            var message, response string
            rows.Scan(&message, &response)
            history = append(history, map[string]string{"role": "user", "message": message})
            history = append(history, map[string]string{"role": "assistant", "message": response})
        }
        // Переворачиваем
        for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
            history[i], history[j] = history[j], history[i]
        }
        
        // Формируем контекст
        contextHistory := ""
        for _, msg := range history {
            contextHistory += fmt.Sprintf("%s: %s\n", msg["role"], msg["message"])
        }
        
        if contextHistory != "" {
            log.Printf("📚 История диалога загружена: %d сообщений", len(history))
        }
    }
    
    smartPrompt := `Ты - умный, дружелюбный AI ассистент Business Stack.

ТЫ ДОЛЖЕН:
- Отвечать на ЛЮБЫЕ вопросы пользователя
- Быть разговорчивым и живым
- Шутить, когда уместно
- Использовать эмодзи для эмоций
- Если не знаешь ответа - честно сказать "Я не знаю"

НЕЛЬЗЯ:
- Отвечать "я тебя не понимаю"
- Выводить список команд без просьбы

Примеры:
Пользователь: привет
Ты: Привет! 😊 Чем могу помочь?

Пользователь: расскажи шутку
Ты: Почему программисты путают Хэллоуин и Рождество? Потому что 31 Oct = 25 Dec! 🎄

Будь полезным и вежливым!`
    
    // Отправляем в YandexGPT
    yandexService := services.NewYandexServiceWithKeys(ai.yandexAPIKey, ai.yandexFolderID)
    answer, err := yandexService.Ask(smartPrompt, req.Message, 0.8)
    if err != nil {
        log.Printf("❌ YandexGPT error: %v", err)
        c.JSON(200, gin.H{
            "response": "Извините, я временно недоступен. Попробуйте позже.",
            "success": true,
        })
        return
    }
    
    // Сохраняем историю
    ai.saveChatHistory(tenantID, userID, req.Message, answer)
    
    c.JSON(200, gin.H{
        "response": answer,
        "success":  true,
    })
}

// saveChatHistory - сохраняет историю чата
func (ai *UniversalAIAssistant) saveChatHistory(tenantID, userID, message, response string) {
    ctx := context.Background()
    
    var exists bool
    err := ai.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = 'ai_chat_history'
        )
    `).Scan(&exists)
    
    if err != nil || !exists {
        ai.db.Exec(ctx, `
            CREATE TABLE IF NOT EXISTS ai_chat_history (
                id SERIAL PRIMARY KEY,
                tenant_id VARCHAR(100),
                user_id VARCHAR(100),
                message TEXT,
                response TEXT,
                created_at TIMESTAMP DEFAULT NOW()
            )
        `)
    }
    
    ai.db.Exec(ctx, `
        INSERT INTO ai_chat_history (tenant_id, user_id, message, response, created_at)
        VALUES ($1, $2, $3, $4, NOW())
    `, tenantID, userID, message, response)
}

// GetHistory - получает историю чата
func (ai *UniversalAIAssistant) GetHistory(c *gin.Context) {
    tenantID := ai.getTenantID(c)
    if tenantID == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Tenant not found"})
        return
    }
    
    rows, err := ai.db.Query(context.Background(),
        `SELECT id, message, response, created_at 
         FROM ai_chat_history 
         WHERE tenant_id = $1 
         ORDER BY created_at DESC 
         LIMIT 50`,
        tenantID)
    if err != nil {
        c.JSON(http.StatusOK, []gin.H{})
        return
    }
    defer rows.Close()
    
    var history []gin.H
    for rows.Next() {
        var id int
        var message, response string
        var createdAt time.Time
        rows.Scan(&id, &message, &response, &createdAt)
        
        history = append(history, gin.H{
            "id":         id,
            "message":    message,
            "response":   response,
            "created_at": createdAt,
        })
    }
    
    c.JSON(http.StatusOK, history)
}

// GetActions - получает список доступных действий
func (ai *UniversalAIAssistant) GetActions(c *gin.Context) {
    actions := []gin.H{
        {"name": "create_customer", "description": "Создание клиента", "example": "Создай клиента ООО Ромашка"},
        {"name": "create_deal", "description": "Создание сделки", "example": "Создай сделку для Ромашки на 500 000"},
        {"name": "create_invoice", "description": "Выставление счёта", "example": "Выставь счёт Ромашке на 500 000"},
        {"name": "create_task", "description": "Создание задачи", "example": "Создай задачу Позвонить клиенту для Иванова"},
        {"name": "archive_account", "description": "Архивация счёта", "example": "архивируй счёт 51"},
        {"name": "restore_account", "description": "Восстановление счёта", "example": "восстанови счёт 51"},
        {"name": "show_archive", "description": "Показать архив счетов", "example": "покажи архив счетов"},
        {"name": "get_osv", "description": "Показать ОСВ", "example": "покажи ОСВ за январь"},
        {"name": "close_month", "description": "Закрыть месяц", "example": "закрой месяц"},
        {"name": "export_to_excel", "description": "Экспорт в Excel", "example": "экспортируй ОСВ в Excel"},
        {"name": "get_balance_sheet", "description": "Бухгалтерский баланс", "example": "покажи бухгалтерский баланс"},
    }
    
    c.JSON(http.StatusOK, actions)
}

// GetSettings - получает настройки AI
func (ai *UniversalAIAssistant) GetSettings(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{
        "yandex_configured":   ai.yandexAPIKey != "",
        "telegram_configured": ai.telegramBotToken != "",
        "version":             "1.0.0",
    })
}