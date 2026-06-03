package handlers

import (
    "context"
    "fmt"
    "net/http"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5/pgxpool"
    
    "subscription-system/services"  // 👈 ДОБАВИТЬ ЭТОТ ИМПОРТ!
)

// UniversalAIAssistant - универсальный AI ассистент
type UniversalAIAssistant struct {
    yandexAPIKey     string
    yandexFolderID   string
    telegramBotToken string
    telegramChatID   string
    adminChatID      string
    db               *pgxpool.Pool
    knowledgeBase    *services.KnowledgeBase  // 👈 ИСПРАВЛЕНО (было knowledgeBase)
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
        knowledgeBase:    services.NewKnowledgeBase(db),  // 👈 ИСПРАВЛЕНО (добавлена запятая выше и точка с запятой)
    }
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

    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "default"
    }
    userID := c.GetString("user_id")
    if userID == "" {
        userID = "system"
    }

    // ========== НОВОЕ: ПОИСК В БАЗЕ ЗНАНИЙ ==========
    var contextDocs string
    var foundDocs []string

    if ai.knowledgeBase != nil {
        docs, err := ai.knowledgeBase.SearchByKeyword(req.Message, 3)
        if err == nil && len(docs) > 0 {
            for i, doc := range docs {
                contextDocs += fmt.Sprintf("\n--- Документ %d: %s ---\n%s\n", i+1, doc.Title, doc.Content)
                foundDocs = append(foundDocs, doc.Title)
            }
        }
    }

    // Формируем системный промпт (с контекстом или без)
    var systemPrompt string
    if contextDocs != "" {
        systemPrompt = fmt.Sprintf(`Ты — AI-ассистент платформы Business Stack.
Отвечай на вопросы пользователей, используя ТОЛЬКО предоставленный контекст документации.
Если ответа нет в контексте — скажи, что не знаешь, и предложи обратиться в поддержку.
Никогда не придумывай ответы.
Будь вежливым, полезным и конкретным.
Отвечай на русском языке.

КОНТЕКСТ ИЗ ДОКУМЕНТАЦИИ:
%s

ВАЖНО: Отвечай ТОЛЬКО по контексту!`, contextDocs)
    } else {
        systemPrompt = `Ты — AI-ассистент платформы Business Stack.
Помогай пользователям с вопросами по платформе.
Если не знаешь ответа — предложи обратиться в поддержку.
Будь полезным и конкретным.
Отвечай на русском языке.`
    }


fmt.Printf("🔍 DEBUG: yandexAPIKey=%v, yandexFolderID=%v\n", ai.yandexAPIKey != "", ai.yandexFolderID != "")
fmt.Printf("🔍 DEBUG: systemPrompt length=%d, userMessage=%s\n", len(systemPrompt), req.Message)

    // Отправляем запрос в YandexGPT
   yandexService := services.NewYandexServiceWithKeys(ai.yandexAPIKey, ai.yandexFolderID)
   answer, err := yandexService.Ask(systemPrompt, req.Message, 0.7)
    if err != nil {
 fmt.Printf("❌ YandexGPT ERROR: %v\n", err)
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    // Сохраняем запрос в историю для обучения
    if ai.knowledgeBase != nil && len(foundDocs) > 0 {
        ai.knowledgeBase.SaveQuery(userID, req.Message, answer, foundDocs)
    }

    // Сохраняем историю чата
    ai.saveChatHistory(tenantID, userID, req.Message, answer)

    c.JSON(200, gin.H{
        "response":  answer,
        "success":   true,
        "source":    map[bool]string{true: "rag", false: "ai"}[contextDocs != ""],
        "documents": foundDocs,
    })
}

// saveChatHistory - сохраняет историю чата
func (ai *UniversalAIAssistant) saveChatHistory(tenantID, userID, message, response string) {
    ctx := context.Background()
    
    // Проверяем существование таблицы
    var exists bool
    err := ai.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = 'ai_chat_history'
        )
    `).Scan(&exists)
    
    if err != nil || !exists {
        // Создаём таблицу если её нет
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
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "default"
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