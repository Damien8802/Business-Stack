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
    "subscription-system/database"
)

type ConsultantRequest struct {
    SessionID string `json:"session_id"`
    Message   string `json:"message"`
}

type ConsultantResponse struct {
    Message   string `json:"message"`
    SessionID string `json:"session_id"`
    Completed bool   `json:"completed"`
}

// Состояния диалога
type ConsultantSession struct {
    SessionID   string
    UserID      *string
    Mode        string // "general" - общий режим, "collecting" - сбор заявки
    Step        string // для режима сбора
    Data        map[string]string
    StartedAt   time.Time
    LastMessage time.Time
}

var sessions = make(map[string]*ConsultantSession)

// База знаний о проекте
var knowledgeBase = map[string]string{
    "тариф": "У нас есть три тарифа: Базовый (990₽/мес), Профессиональный (2990₽/мес) и Предприятие (9990₽/мес). Подробнее на странице /pricing",
    "цена": "Цены начинаются от 990₽ в месяц. На индивидуальные проекты рассчитываются отдельно.",
    "crm": "У нас есть мощная CRM-система с клиентами, сделками, аналитикой, канбан-доской и календарём. Доступна в тарифах Профессиональный и выше.",
    "интеграция": "Поддерживаем интеграции с Telegram, WhatsApp, amoCRM, Bitrix24, Google Sheets, 1С. Для индивидуальных проектов возможны любые интеграции.",
    "api": "У нас есть API для управления подписками и доступа к данным. Документация по Swagger доступна по адресу /swagger/index.html",
    "оплата": "Принимаем карты РФ, криптовалюту (USDT, BTC), СБП. Для корпоративных клиентов возможна оплата по счету.",
    "поддержка": "Поддержка работает 24/7 через Telegram @IDamieN66I или email support@Business Stack.ru",
    "бот": "Мы разрабатываем Telegram-ботов под ключ. Это индивидуальная услуга — оставьте заявку, и мы обсудим детали.",
    "индивидуальный": "Для индивидуальных проектов мы предлагаем полный цикл разработки: от идеи до внедрения. Расскажите, что хотите создать, и я соберу все детали.",
    "сайт": "Да, мы разрабатываем сайты и интернет-магазины под ключ. Это индивидуальная услуга.",
    "ai": "У нас есть встроенный AI-ассистент в CRM, а также мы разрабатываем кастомных AI-ботов для бизнеса.",
    "скидка": "Скидки предусмотрены при оплате за год, а также для партнёров и при заказе комплексных индивидуальных проектов.",
}

// Ключевые слова для перехода в режим сбора заявки
var collectingKeywords = []string{
    "индивидуальн", "под ключ", "разработк", "созда", "бот", "интернет-магазин",
    "telegram", "mini app", "интеграц", "ai", "чат-бот", "автоматизац",
    "партнёрск", "дашборд", "seo", "маркетинг", "свой проект", "заказать",
    "сделать", "нужен бот", "хочу сайт", "разработать", "написать бота",
}

// Очистка старых сессий
func init() {
    go func() {
        for {
            time.Sleep(1 * time.Hour)
            for id, sess := range sessions {
                if time.Since(sess.LastMessage) > 30*time.Minute {
                    delete(sessions, id)
                }
            }
        }
    }()
}

func AIConsultantHandler(c *gin.Context) {
    var req ConsultantRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный запрос"})
        return
    }

    // Получаем или создаём сессию
    var session *ConsultantSession
    if req.SessionID != "" {
        session = sessions[req.SessionID]
    }

    if session == nil {
        // Новая сессия
        sessionID := uuid.New().String()
        session = &ConsultantSession{
            SessionID:   sessionID,
            Mode:        "general", // начинаем в общем режиме
            Data:        make(map[string]string),
            StartedAt:   time.Now(),
            LastMessage: time.Now(),
        }
        if userID, exists := c.Get("userID"); exists {
            if uid, ok := userID.(string); ok {
                session.UserID = &uid
            }
        }
        sessions[sessionID] = session

        // Отправляем приветствие
        c.JSON(http.StatusOK, ConsultantResponse{
            SessionID: sessionID,
            Message:   "Здравствуйте! Я AI-помощник платформы Business Stack. Могу ответить на вопросы о тарифах, функциях, интеграциях, а также помочь оформить заявку на индивидуальную разработку. О чём вы хотели бы узнать?",
            Completed: false,
        })
        return
    }

    session.LastMessage = time.Now()
    userMessage := strings.ToLower(strings.TrimSpace(req.Message))

    // Если мы в режиме сбора заявки
    if session.Mode == "collecting" {
        handleCollectingMode(session, userMessage, c)
        return
    }

    // Проверяем, не хочет ли пользователь перейти в режим сбора заявки
    isCollecting := false
    for _, kw := range collectingKeywords {
        if strings.Contains(userMessage, kw) {
            isCollecting = true
            break
        }
    }

    if isCollecting {
        // Переключаемся в режим сбора заявки
        session.Mode = "collecting"
        session.Step = "greeting"
        session.Data = make(map[string]string)
        c.JSON(http.StatusOK, ConsultantResponse{
            SessionID: session.SessionID,
            Message:   "Отлично! Я помогу вам оформить заявку на индивидуальную разработку. Расскажите, что именно вы хотите создать? (Например: Telegram-бота, интернет-магазин, интеграцию с CRM, AI-ассистента и т.д.)",
            Completed: false,
        })
        return
    }

    // Ищем ответ в базе знаний
    answer := findAnswer(userMessage)
    c.JSON(http.StatusOK, ConsultantResponse{
        SessionID: session.SessionID,
        Message:   answer,
        Completed: false,
    })
}

// Обработка режима сбора заявки
func handleCollectingMode(session *ConsultantSession, userMessage string, c *gin.Context) {
    switch session.Step {
    case "greeting", "":
        session.Data["service_type"] = userMessage
        session.Step = "description"
        c.JSON(http.StatusOK, ConsultantResponse{
            SessionID: session.SessionID,
            Message:   "Отлично! А теперь подробнее опишите, что должно быть в этом проекте? Какие функции, особенности?",
            Completed: false,
        })

    case "description":
        session.Data["description"] = userMessage
        session.Step = "budget"
        c.JSON(http.StatusOK, ConsultantResponse{
            SessionID: session.SessionID,
            Message:   "Понял. Какой бюджет вы рассматриваете на этот проект? (Примерно в рублях или у.е.)",
            Completed: false,
        })

    case "budget":
        session.Data["budget"] = userMessage
        session.Step = "deadline"
        c.JSON(http.StatusOK, ConsultantResponse{
            SessionID: session.SessionID,
            Message:   "Хорошо. А какие сроки? Когда нужно завершить проект?",
            Completed: false,
        })

    case "deadline":
        session.Data["deadline"] = userMessage
        session.Step = "contacts"
        c.JSON(http.StatusOK, ConsultantResponse{
            SessionID: session.SessionID,
            Message:   "Почти готово! Как к вам обращаться и как с вами связаться? (Напишите в формате: Имя, контакт. Например: Иван, @ivan или Иван, ivan@mail.ru)",
            Completed: false,
        })

    case "contacts":
        // Разбираем имя и контакт из сообщения
        parts := strings.Split(userMessage, ",")
        name := strings.TrimSpace(parts[0])
        contact := ""
        if len(parts) > 1 {
            contact = strings.TrimSpace(parts[1])
        } else {
            contact = userMessage
        }

        session.Data["name"] = name
        session.Data["contact"] = contact

        // Сохраняем в базу данных
        go saveConsultationToDB(session)

        // Отправляем уведомление администратору
        notifyAdminAboutConsultation(session)

        // Возвращаемся в общий режим
        session.Mode = "general"
        session.Step = ""

        c.JSON(http.StatusOK, ConsultantResponse{
            SessionID: session.SessionID,
            Message:   "✅ Спасибо за подробную информацию! Администратор свяжется с вами в течение 15 минут для уточнения деталей. Если у вас есть другие вопросы о платформе, я готов на них ответить.",
            Completed: true, // диалог по заявке завершён, но сессия остаётся для общих вопросов
        })

    default:
        session.Mode = "general"
        session.Step = ""
        c.JSON(http.StatusOK, ConsultantResponse{
            SessionID: session.SessionID,
            Message:   "Извините, произошла ошибка. Я переключился в общий режим. О чём вы хотели бы узнать?",
            Completed: false,
        })
    }
}

// Поиск ответа в базе знаний
func findAnswer(question string) string {
    // Сначала ищем по ключевым словам
    for key, answer := range knowledgeBase {
        if strings.Contains(question, key) {
            return answer
        }
    }

    // Если ничего не нашли, предлагаем помощь
    return "Я могу рассказать о тарифах, функциях CRM, интеграциях, оплате, поддержке, а также помочь оформить заявку на индивидуальную разработку. Что вас интересует?"
}

// Сохранение в базу данных
func saveConsultationToDB(session *ConsultantSession) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    _, err := database.Pool.Exec(ctx, `
        INSERT INTO individual_consultations 
        (session_id, user_id, name, contact, service_type, description, budget, deadline, status, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW())
    `, session.SessionID, session.UserID, session.Data["name"], session.Data["contact"],
        session.Data["service_type"], session.Data["description"], session.Data["budget"],
        session.Data["deadline"], "new")

    if err != nil {
        log.Printf("Ошибка сохранения консультации: %v", err)
    } else {
        log.Printf("✅ Консультация сохранена в БД, session_id: %s", session.SessionID)
    }
}

// Уведомление администратора
func notifyAdminAboutConsultation(session *ConsultantSession) {
    message := "\n📋 НОВАЯ ЗАЯВКА НА ИНДИВИДУАЛЬНУЮ РАЗРАБОТКУ!\n"
    message += "══════════════════════════════════════════\n"
    message += fmt.Sprintf("👤 Имя: %s\n", session.Data["name"])
    message += fmt.Sprintf("📱 Контакт: %s\n", session.Data["contact"])
    message += fmt.Sprintf("🔧 Услуга: %s\n", session.Data["service_type"])
    message += fmt.Sprintf("📝 Описание: %s\n", session.Data["description"])
    message += fmt.Sprintf("💰 Бюджет: %s\n", session.Data["budget"])
    message += fmt.Sprintf("⏱️ Срок: %s\n", session.Data["deadline"])
    message += fmt.Sprintf("🕐 Время заявки: %s\n", time.Now().Format("2006-01-02 15:04:05"))
    message += "══════════════════════════════════════════"

    log.Println(message)
}