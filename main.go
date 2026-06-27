package main

import (
    "context"
    "encoding/json"
    "fmt"
    "html/template"
    "log"
    "net/http"
    "strings"
    "time"
    //"io"
    //"net"
    //"strconv"
     "os" 
     
    "github.com/redis/go-redis/v9"  
    "github.com/jmoiron/sqlx"  
    "github.com/gin-gonic/gin"
    "github.com/jackc/pgx/v5"  // ДОБАВЛЕНО ДЛЯ pgx.Rows
    "github.com/joho/godotenv"
    swaggerFiles "github.com/swaggo/files"
    ginSwagger "github.com/swaggo/gin-swagger"
    


    "subscription-system/config"
    "subscription-system/database"
    "subscription-system/handlers"
    "subscription-system/middleware"
    "subscription-system/services"
     "subscription-system/cleanup"
    _ "subscription-system/docs"
)

type ServiceOrder struct {
    Name        string `json:"name"`
    Contact     string `json:"contact"`
    ServiceType string `json:"service_type"`
    Deadline    string `json:"deadline"`
    Budget      string `json:"budget"`
    Description string `json:"description"`
}
func serviceOrderHandler(c *gin.Context) {
    var order ServiceOrder
    if err := c.ShouldBindJSON(&order); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные"})
        return
    }

    if order.Name == "" || order.Contact == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Имя и контакт обязательны"})
        return
    }

    // Получаем tenant_id из контекста
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("token_tenant_id")
    }
    if tenantID == "" {
        userID := c.GetString("user_id")
        if userID != "" {
            database.Pool.QueryRow(c.Request.Context(), 
                "SELECT tenant_id FROM users WHERE id = $1", userID).Scan(&tenantID)
        }
    }

    // ✅ ИСПРАВЛЕНО: используем additional_info вместо description
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO service_orders (client_name, client_contact, service_type, deadline, budget, additional_info, created_at, tenant_id, status)
        VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7, 'new')
    `, order.Name, order.Contact, order.ServiceType, order.Deadline, order.Budget, order.Description, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка сохранения заявки: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка базы данных"})
        return
    }

    log.Printf("📦 Новая заявка на услуги: %s (%s): %s", order.Name, order.Contact, order.Description)
    
    // ✅ Отвечаем сразу
    c.JSON(http.StatusOK, gin.H{"status": "ok"})
    
    // ✅ Уведомление отправляем асинхронно
    go func() {
        services.NotifyAdminServiceOrder(order.Name, order.Contact, order.Description)
    }()
}
func main() {
    if err := godotenv.Load(); err != nil {
        log.Println("⚠️ .env file not found, using system environment")
    } else {
        fmt.Println("✅ .env file loaded and applied")
    }
    cfg := config.Load()

    if err := database.InitDB(cfg); err != nil {
        log.Fatalf("❌ Ошибка подключения к БД: %v", err)
    }
    defer database.CloseDB()

// ✅ ЗАПУСКАЕМ ПЛАНИРОВЩИК ОЧИСТКИ
    cleanup.StartCleanupScheduler()   

   // ========== СОЗДАНИЕ ТАБЛИЦ VPN ==========
ctx := context.Background()

_, err := database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS vpn_plans (
        id SERIAL PRIMARY KEY,
        name VARCHAR(50) NOT NULL,
        price DECIMAL(10,2) NOT NULL,
        days INTEGER NOT NULL,
        speed VARCHAR(50),
        devices INTEGER DEFAULT 1,
        tenant_id UUID NOT NULL,
        created_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания vpn_plans: %v", err)
} else {
    log.Println("✅ Таблица vpn_plans готова")
}


// ========== ДОБАВЛЯЕМ tenant_id В ТАБЛИЦЫ (ЕСЛИ НЕТ) ==========
_, err = database.Pool.Exec(ctx, `
    ALTER TABLE service_orders ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL;
    ALTER TABLE feature_requests ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL;
    ALTER TABLE employees ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL;
    ALTER TABLE candidates ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL;
    ALTER TABLE vacancies ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL;
`)
if err != nil {
    log.Printf("⚠️ Ошибка добавления tenant_id: %v", err)
} else {
    log.Println("✅ tenant_id добавлен во все таблицы")
}


// ========== СОЗДАНИЕ ТАБЛИЦЫ ЗАЯВОК ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS service_orders (
        id SERIAL PRIMARY KEY,
        client_name VARCHAR(255) NOT NULL,
        client_contact VARCHAR(255) NOT NULL,
        service_type VARCHAR(255),
        design_requirements TEXT,
        deadline VARCHAR(100),
        budget VARCHAR(100),
        additional_info TEXT,
        status VARCHAR(50) DEFAULT 'new',
        created_at TIMESTAMP DEFAULT NOW(),
        viewed_at TIMESTAMP,
        tenant_id UUID NOT NULL,
        deposit_status VARCHAR(50) DEFAULT 'not_paid',
        deposit_amount DECIMAL(10,2) DEFAULT 0,
        deposit_date TIMESTAMP,
        remaining_amount DECIMAL(10,2) DEFAULT 0,
        remaining_status VARCHAR(50) DEFAULT 'not_paid',
        remaining_date TIMESTAMP,
        work_status VARCHAR(50) DEFAULT 'waiting_deposit'
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания service_orders: %v", err)
} else {
    log.Println("✅ Таблица service_orders готова")
}

// ========== ТАБЛИЦА ЖУРНАЛА ОПЕРАЦИЙ ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS journal_entries (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        tenant_id UUID NOT NULL,
        operation_date DATE NOT NULL,
        document_number VARCHAR(100),
        document_type VARCHAR(50),
        counterparty_name VARCHAR(255),
        counterparty_inn VARCHAR(12),
        debit_account VARCHAR(20),
        credit_account VARCHAR(20),
        debit_amount DECIMAL(15,2) DEFAULT 0,
        credit_amount DECIMAL(15,2) DEFAULT 0,
        description TEXT,
        created_at TIMESTAMP DEFAULT NOW(),
        updated_at TIMESTAMP DEFAULT NOW()
    );
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания journal_entries: %v", err)
} else {
    log.Println("✅ Таблица journal_entries готова")
}


// ========== СОЗДАНИЕ ТАБЛИЦЫ ДОРАБОТОК ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS feature_requests (
        id SERIAL PRIMARY KEY,
        user_id UUID,
        user_name VARCHAR(255),
        user_email VARCHAR(255),
        title VARCHAR(500) NOT NULL,
        description TEXT NOT NULL,
        priority VARCHAR(50) DEFAULT 'medium',
        status VARCHAR(50) DEFAULT 'new',
        created_at TIMESTAMP DEFAULT NOW(),
        updated_at TIMESTAMP,
        tenant_id UUID NOT NULL
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания feature_requests: %v", err)
} else {
    log.Println("✅ Таблица feature_requests готова")
}

// ========== ТАБЛИЦА ОЖИДАЮЩИХ ПОДТВЕРЖДЕНИЯ ПОЛЬЗОВАТЕЛЕЙ ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS pending_users (
        id SERIAL PRIMARY KEY,
        email VARCHAR(255) NOT NULL UNIQUE,
        password_hash TEXT NOT NULL,
        name VARCHAR(255),
        token VARCHAR(255) NOT NULL,
        expires_at TIMESTAMP NOT NULL,
        created_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания pending_users: %v", err)
} else {
    log.Println("✅ Таблица pending_users готова")
}
// Добавьте в main.go после создания других таблиц:
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS month_closing (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        user_id UUID NOT NULL,
        year INT NOT NULL,
        month INT NOT NULL,
        closed_at TIMESTAMP NOT NULL,
        status VARCHAR(20) DEFAULT 'closed'
    );
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания month_closing: %v", err)
} else {
    log.Println("✅ Таблица month_closing готова")
}

// ========== СОЗДАНИЕ ТАБЛИЦЫ PAYROLL_HISTORY ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS payroll_history (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        tenant_id UUID NOT NULL,
        employee_id UUID,
        employee_name VARCHAR(255) NOT NULL,
        month INTEGER NOT NULL,
        year INTEGER NOT NULL,
        gross DECIMAL(15,2) DEFAULT 0,
        tax DECIMAL(15,2) DEFAULT 0,
        net DECIMAL(15,2) DEFAULT 0,
        status VARCHAR(50) DEFAULT 'accrued',
        created_at TIMESTAMP DEFAULT NOW(),
        updated_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания payroll_history: %v", err)
} else {
    log.Println("✅ Таблица payroll_history готова")
}

// Создаём индексы
_, err = database.Pool.Exec(ctx, `
    CREATE INDEX IF NOT EXISTS idx_payroll_history_tenant ON payroll_history(tenant_id);
    CREATE INDEX IF NOT EXISTS idx_payroll_history_employee ON payroll_history(employee_id);
    CREATE INDEX IF NOT EXISTS idx_payroll_history_period ON payroll_history(year, month);
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания индексов payroll_history: %v", err)
}

// ========== СОЗДАНИЕ ТАБЛИЦЫ PAYROLL ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS payroll (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        tenant_id UUID NOT NULL,
        employee_id UUID NOT NULL,
        period_month INTEGER NOT NULL,
        period_year INTEGER NOT NULL,
        salary DECIMAL(15,2) DEFAULT 0,
        tax DECIMAL(15,2) DEFAULT 0,
        net_amount DECIMAL(15,2) DEFAULT 0,
        status VARCHAR(50) DEFAULT 'calculated',
        paid_at TIMESTAMP,
        created_at TIMESTAMP DEFAULT NOW(),
        updated_at TIMESTAMP DEFAULT NOW(),
        CONSTRAINT unique_employee_period UNIQUE(employee_id, period_month, period_year)
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания payroll: %v", err)
} else {
    log.Println("✅ Таблица payroll готова")
}

// ========== СОЗДАНИЕ ТАБЛИЦЫ АРХИВА PAYSLIP ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS payslip_archive (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        tenant_id UUID NOT NULL,
        employee_id UUID,
        employee_name VARCHAR(255) NOT NULL,
        position VARCHAR(255),
        month INTEGER NOT NULL,
        year INTEGER NOT NULL,
        content TEXT,
        created_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания payslip_archive: %v", err)
} else {
    log.Println("✅ Таблица payslip_archive готова")
}

// Создаём индексы
_, err = database.Pool.Exec(ctx, `
    CREATE INDEX IF NOT EXISTS idx_payslip_archive_tenant ON payslip_archive(tenant_id);
    CREATE INDEX IF NOT EXISTS idx_payslip_archive_period ON payslip_archive(year, month);
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания индексов payslip_archive: %v", err)
}

// ========== ТАБЛИЦА МОДУЛЕЙ В РАЗРАБОТКЕ ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS dev_modules (
        id SERIAL PRIMARY KEY,
        name VARCHAR(255) NOT NULL,
        route VARCHAR(255) NOT NULL UNIQUE,
        icon VARCHAR(50) DEFAULT '🔧',
        description TEXT,
        created_at TIMESTAMP DEFAULT NOW(),
        updated_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания dev_modules: %v", err)
} else {
    log.Println("✅ Таблица dev_modules готова")
}
    
// Добавить колонку deleted_at в chart_of_accounts (если нет)
_, err = database.Pool.Exec(ctx, `
    ALTER TABLE chart_of_accounts ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP;
`)
if err != nil {
    log.Printf("⚠️ Ошибка добавления deleted_at: %v", err)
} else {
    log.Println("✅ Колонка deleted_at добавлена в chart_of_accounts")
}


// Таблица статусов модулей
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS module_statuses (
        id SERIAL PRIMARY KEY,
        route VARCHAR(255) NOT NULL UNIQUE,
        status VARCHAR(50) DEFAULT 'development',
        message TEXT,
        updated_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания module_statuses: %v", err)
} else {
    log.Println("✅ Таблица module_statuses готова")
}

// Таблица обратной связи (если еще нет)
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS module_feedback (
        id SERIAL PRIMARY KEY,
        user_id UUID,
        user_email VARCHAR(255),
        user_name VARCHAR(255),
        module VARCHAR(255) NOT NULL,
        issue TEXT NOT NULL,
        status VARCHAR(50) DEFAULT 'new',
        url TEXT,
        user_agent TEXT,
        resolved BOOLEAN DEFAULT FALSE,
        created_at TIMESTAMP DEFAULT NOW(),
        updated_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания module_feedback: %v", err)
} else {
    log.Println("✅ Таблица module_feedback готова")
}

// Таблица комментариев к заявкам
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS request_comments (
        id SERIAL PRIMARY KEY,
        request_id INTEGER NOT NULL,
        user_id VARCHAR(255) NOT NULL,
        comment TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания request_comments: %v", err)
} else {
    log.Println("✅ Таблица request_comments готова")
}

// Таблица уведомлений
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS notifications (
        id SERIAL PRIMARY KEY,
        user_id VARCHAR(255) NOT NULL,
        message TEXT NOT NULL,
        type VARCHAR(50) NOT NULL,
        is_read BOOLEAN DEFAULT FALSE,
        created_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания notifications: %v", err)
} else {
    log.Println("✅ Таблица notifications готова")
}

// ========== ТАБЛИЦА СЕССИЙ ПОЛЬЗОВАТЕЛЕЙ ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS user_sessions (
        id VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid(),
        user_id UUID NOT NULL,
        device_name VARCHAR(255),
        ip VARCHAR(45),
        location VARCHAR(255),
        last_active TIMESTAMP DEFAULT NOW(),
        is_current BOOLEAN DEFAULT false,
        revoked BOOLEAN DEFAULT false,
        created_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания user_sessions: %v", err)
} else {
    log.Println("✅ Таблица user_sessions готова")
}

// ========== ТАБЛИЦА АКТИВНОСТИ ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS activity_logs (
        id VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid(),
        user_id UUID NOT NULL,
        action VARCHAR(100) NOT NULL,
        resource VARCHAR(100),
        resource_id VARCHAR(36),
        details JSONB DEFAULT '{}',
        ip VARCHAR(45),
        user_agent TEXT,
        status VARCHAR(50) DEFAULT 'success',
        created_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания activity_logs: %v", err)
} else {
    log.Println("✅ Таблица activity_logs готова")
}

// ========== ТАБЛИЦА ДОВЕРЕННЫХ УСТРОЙСТВ ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS trusted_devices (
        id VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid(),
        user_id UUID NOT NULL,
        device_name VARCHAR(255),
        browser VARCHAR(255),
        ip VARCHAR(45),
        created_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания trusted_devices: %v", err)
} else {
    log.Println("✅ Таблица trusted_devices готова")
}

// ========== ТАБЛИЦА ИСТОРИИ ВХОДОВ ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS login_history (
        id VARCHAR(36) PRIMARY KEY DEFAULT gen_random_uuid(),
        user_id UUID NOT NULL,
        ip VARCHAR(45),
        browser VARCHAR(255),
        os VARCHAR(100),
        location VARCHAR(255),
        success BOOLEAN DEFAULT true,
        created_at TIMESTAMP DEFAULT NOW()
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания login_history: %v", err)
} else {
    log.Println("✅ Таблица login_history готова")
}

// ========== СОЗДАНИЕ ТАБЛИЦЫ ЗАЯВОК НА УСЛУГИ ==========
_, err = database.Pool.Exec(ctx, `
    CREATE TABLE IF NOT EXISTS service_requests (
        id SERIAL PRIMARY KEY,
        name VARCHAR(255) NOT NULL,
        contact VARCHAR(255) NOT NULL,
        description TEXT,
        status VARCHAR(50) DEFAULT 'new',
        created_at TIMESTAMP DEFAULT NOW(),
        viewed_at TIMESTAMP,
        tenant_id UUID
    )
`)
if err != nil {
    log.Printf("⚠️ Ошибка создания service_requests: %v", err)
} else {
    log.Println("✅ Таблица service_requests готова")
}
// ========== ИНИЦИАЛИЗАЦИЯ REDIS ==========
var redisClient *redis.Client
redisAddr := os.Getenv("REDIS_ADDR")
if redisAddr == "" {
    redisAddr = "localhost:6379"
}

redisClient = redis.NewClient(&redis.Options{
    Addr:     redisAddr,
    Password: os.Getenv("REDIS_PASSWORD"),
    DB:       0,
})

// Проверяем подключение к Redis

if err := redisClient.Ping(ctx).Err(); err != nil {
    log.Printf("⚠️ Redis не доступен: %v (кеширование будет отключено)", err)
    redisClient = nil
} else {
    log.Println("✅ Redis подключен для кеширования")
}

// ========== ИНИЦИАЛИЗАЦИЯ УЛУЧШЕННЫХ ХЕНДЛЕРОВ ==========
var db *sqlx.DB
if database.Pool != nil {
   db, err = sqlx.Open("pgx", os.Getenv("DATABASE_URL"))
}

var inventoryHandler *handlers.InventoryHandler  // ← РАСКОММЕНТИРОВАТЬ!
var supplierHandler *handlers.SupplierHandler
var receiptHandler *handlers.GoodsReceiptHandler

if db != nil && redisClient != nil {
    inventoryHandler = handlers.NewInventoryHandler(db, redisClient)
    supplierHandler = handlers.NewSupplierHandler(db, redisClient)
    receiptHandler = handlers.NewGoodsReceiptHandler(db, redisClient)
    log.Println("✅ Улучшенные хендлеры инициализированы с Redis кешированием")
} else {
    log.Println("⚠️ Redis не доступен, используются стандартные хендлеры")
}
// ========== ИНИЦИАЛИЗАЦИЯ WEBSOCKET ==========
handlers.InitInventoryWS()
log.Println("✅ WebSocket хаб инициализирован")
    handlers.InitVPNWithDB(database.Pool)
    // Инициализация Stealth VPN сервиса
    handlers.InitStealthVPN(database.Pool)
    log.Println("✅ Stealth VPN сервис инициализирован")

    handlers.InitAuthHandler(cfg)
    handlers.InitNotifier(cfg)

    var yandexService *services.YandexAdapter
    var aiAgentService *services.AIAgentService
    var speechKitService *services.SpeechKitService

    yandexService = services.NewYandexService(cfg)
    aiAgentService = services.NewAIAgentService(yandexService)
    aiAgentService.StartAgentScheduler()
    log.Println("🤖 Сервис ИИ-агентов запущен с YandexGPT")

    speechKitService = services.NewSpeechKitService(cfg)
    _ = speechKitService
    log.Println("🎙️ Сервис транскрибации SpeechKit инициализирован")

// ========== ИНИЦИАЛИЗАЦИЯ TELEGRAM БОТА ==========
services.InitTelegramServices()
log.Println("✅ Telegram боты инициализированы")

    // ========== НОВЫЕ СЕРВИСЫ ==========
    // Получаем API ключи для новых сервисов
    //yandexSearchAPIKey := os.Getenv("YANDEX_SEARCH_API_KEY")
    yandexFolderID := os.Getenv("YANDEX_FOLDER_ID")
    yandexAPIKey := os.Getenv("YANDEX_API_KEY")
    telegramBotToken := os.Getenv("TELEGRAM_BOT_TOKEN")
    telegramChatID := os.Getenv("TELEGRAM_CHAT_ID")
    adminChatID := os.Getenv("ADMIN_CHAT_ID")
    
    log.Printf("🤖 AI Assistant: YandexAPI=%v, Telegram=%v", 
        yandexAPIKey != "", telegramBotToken != "")
    
      // Универсальный AI ассистент
universalAI := handlers.NewUniversalAIAssistant(
    yandexAPIKey,
    yandexFolderID,
    telegramBotToken,
    telegramChatID,
    adminChatID,
    database.Pool,
)
    log.Println("✅ Universal AI Assistant инициализирован")

aiExecutor := services.NewAIActionExecutor(database.Pool)
log.Println("✅ AI Action Executor инициализирован")

// Workflow engine для автономных цепочек
workflowEngine := services.NewWorkflowEngine(database.Pool)
log.Println("✅ AI Workflow Engine инициализирован")

// Запускаем фоновый планировщик автономных задач
go services.StartAutonomousScheduler(database.Pool)
log.Println("✅ AI Autonomous Scheduler запущен")
    
    // Обработчик заказов на разработку
  individualOrdersHandler := handlers.NewIndividualOrdersHandler(
    yandexAPIKey,
    yandexFolderID,
    telegramBotToken,
    telegramChatID,
    adminChatID,
)
    log.Println("✅ Individual Orders Handler инициализирован")

    if cfg.Env == "release" {
        gin.SetMode(gin.ReleaseMode)
    }

  r := gin.New()



// ========== БАЗОВЫЕ MIDDLEWARE ==========
r.Use(middleware.MegaSecurityMiddleware())
r.Use(middleware.AuditMiddleware())
r.Use(middleware.Fail2BanMiddleware())
r.Use(middleware.ForcePasswordChangeMiddleware())

r.Use(gin.Logger())
r.Use(gin.Recovery())
r.Use(middleware.Logger())
r.SetTrustedProxies(cfg.TrustedProxies)
r.Use(middleware.SetupCORS(cfg))

// ========== ОСНОВНЫЕ MIDDLEWARE (ТОЛЬКО ОДИН РАЗ!) ==========
 
r.Use(middleware.AuthMiddleware(cfg))
r.Use(middleware.TenantMiddleware(database.Pool))  // ← ТОЛЬКО ОДИН РАЗ!
r.Use(middleware.DevModulesMiddleware())           // ← DevModules после Auth!

// ========== ЛИМИТЕРЫ ==========
rateLimiter := middleware.NewRateLimiter(60, time.Minute)
r.Use(middleware.SecurityMonitor())
authLimiter := middleware.NewRateLimiter(60, time.Minute)



    // ========== ЗАГРУЗКА ШАБЛОНОВ ==========
    // Загружаем шаблоны из файловой системы
    tmpl, err := template.New("").Funcs(template.FuncMap{
        "jsonParse": func(s json.RawMessage) []interface{} {
            var arr []interface{}
            json.Unmarshal(s, &arr)
            return arr
        },
        "firstLetter": func(s string) string {
            if len(s) == 0 {
                return "?"
            }
            return strings.ToUpper(string(s[0]))
        },
        "sub": func(a, b int) int { return a - b },
        "add": func(a, b int) int { return a + b },
        "seq": func(n int) []int {
            s := make([]int, n)
            for i := 0; i < n; i++ {
                s[i] = i + 1
            }
            return s
        },
        "float": func(i int64) float64 { return float64(i) },
        "mul":   func(a, b float64) float64 { return a * b },
        "div": func(a, b float64) float64 {
            if b == 0 {
                return 0
            }
            return a / b
        },
        "default": func(defaultVal, val interface{}) interface{} {
            if val == nil {
                return defaultVal
            }
            if str, ok := val.(string); ok && str == "" {
                return defaultVal
            }
            return val
        },
    }).ParseGlob("templates/*.html")
    if err != nil {
        log.Fatalf("❌ Не удалось загрузить шаблоны: %v", err)
    }

 // Добавляем HR шаблоны
log.Println("🔍 Загружаем HR шаблоны из templates/hr/")
hrTmpl, err := template.ParseGlob("templates/hr/*.html")
if err != nil {
    log.Printf("❌ Ошибка ParseGlob: %v", err)
} else if hrTmpl == nil {
    log.Println("❌ hrTmpl == nil")
} else {
    log.Printf("✅ Найдено HR шаблонов: %d", len(hrTmpl.Templates()))
    for _, t := range hrTmpl.Templates() {
        tmpl.AddParseTree(t.Name(), t.Tree)
        log.Printf("   ✅ Загружен: %s", t.Name())
    }
}

    // Добавляем MARKETPLACE шаблоны
    marketplaceTmpl, err := template.ParseGlob("templates/marketplace/*.html")
    if err == nil && marketplaceTmpl != nil {
        for _, t := range marketplaceTmpl.Templates() {
            tmpl.AddParseTree(t.Name(), t.Tree)
        }
    }

    r.SetHTMLTemplate(tmpl)

  // Публичные маршруты
public := r.Group("/")
{
    public.GET("/", handlers.HomeHandler)
    public.GET("/about", handlers.AboutHandler)
    public.GET("/contact", handlers.ContactHandler)
    public.GET("/info", handlers.InfoHandler)
    public.GET("/pricing", handlers.PricingPageHandler)
    public.GET("/partner", handlers.PartnerHandler)
    public.GET("/fusion-api", handlers.FusionAPIPortalHandler)
    public.GET("/identity-landing", handlers.IdentityHubLandingHandler)
    public.GET("/sign-act/:id", handlers.TheirSignPage)
  // Identity Hub отдельные страницы
    public.GET("/identity-login", handlers.IdentityLoginPageHandler)




    // ========== БЛОГ ==========
    r.GET("/blog", func(c *gin.Context) {
        c.HTML(200, "blog.html", gin.H{
            "title": "Блог | Business Stack - новости и статьи",
        })
    })
}

// Страница скрам-доски
r.GET("/scrum", func(c *gin.Context) {
    c.HTML(http.StatusOK, "scrum_board", gin.H{
        "title": "Scrum Board | TeamSphere",
    })
})

// Страница налоговой отчётности
r.GET("/tax-reports", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("tax-reporting"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "tax_reports", gin.H{
        "title": "Налоговая отчётность | Business Stack",
    })
})
// Страница расчёта зарплаты
r.GET("/payroll", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("payroll"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "payroll.html", gin.H{
        "title": "Расчёт зарплаты | Business Stack",
    })
})
// ========== СТРАНИЦЫ ДОКУМЕНТОВ ==========
r.GET("/offer", func(c *gin.Context) {
    c.HTML(http.StatusOK, "offer.html", gin.H{
        "title": "Договор оферты | Business Stack",
    })
})
r.GET("/privacy", func(c *gin.Context) {
    c.HTML(http.StatusOK, "privacy.html", gin.H{
        "title": "Политика конфиденциальности | Business Stack",
    })
})
r.GET("/terms", func(c *gin.Context) {
    c.HTML(http.StatusOK, "terms.html", gin.H{
        "title": "Условия использования | Business Stack",
    })
})
r.GET("/faq", func(c *gin.Context) {
    c.HTML(http.StatusOK, "faq.html", gin.H{
        "title": "FAQ | Business Stack",
    })
})
r.GET("/docs", func(c *gin.Context) {
    c.HTML(http.StatusOK, "docs.html", gin.H{
        "title": "Документация | Business Stack",
    })
})
    // ========== СТАТИКА, РЕДИРЕКТЫ ==========

// ========== СКАЧИВАНИЕ Business Stack VPN ==========
r.GET("/download", func(c *gin.Context) {
    c.HTML(http.StatusOK, "download.html", gin.H{
        "title": "Business Stack VPN - Установить",
    })
})

r.GET("/downloads/Business Stack.apk", func(c *gin.Context) {
    c.Header("Content-Type", "application/vnd.android.package-archive")
    c.Header("Content-Disposition", "attachment; filename=Business Stack.apk")
    c.File("./static/downloads/Business Stack.apk")
})

r.GET("/get-vpn", func(c *gin.Context) {
    c.Redirect(http.StatusFound, "/download")
})
    r.Static("/static", cfg.StaticPath)
    r.Static("/frontend", cfg.FrontendPath)
    r.Static("/app", "C:/Projects/subscription-system/telegram-mini-app")
    r.GET("/telegram/manifest.json", func(c *gin.Context) { c.File("./telegram-mini-app/manifest.json") })
    r.GET("/telegram/sw.js", func(c *gin.Context) { c.File("./telegram-mini-app/service-worker.js") })
    r.GET("/app", func(c *gin.Context) { c.File("C:/Projects/subscription-system/telegram-mini-app/index.html") })
    r.GET("/dashboard_improved", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/dashboard-improved") })
    r.GET("/dashboard", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/dashboard-improved") })
    r.GET("/delivery", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/logistics") })
    r.GET("/ai", handlers.AIChatPageHandler)
    r.GET("/my-keys", handlers.MyKeysPageHandler)
    r.GET("/api-keys", handlers.APIKeysPageHandler)
    r.GET("/support", handlers.SupportPageHandler)
    r.GET("/referral", handlers.ReferralPageHandler)
    r.GET("/ai-settings", handlers.AISettingsPageHandler)
    r.GET("/transcriptions", handlers.TranscriptionsPage)
    r.GET("/ai-agents", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("ai-agents"), handlers.AIAgentsPage)
    r.GET("/advanced-analytics", handlers.AdvancedAnalyticsPage)


// Акты сверки
reconciliationAPI := r.Group("/api/reconciliation")
reconciliationAPI.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("reconciliation"))
{
    reconciliationAPI.POST("/generate", handlers.GenerateReconciliationAct)
    reconciliationAPI.GET("/acts", handlers.GetReconciliationActs)
    reconciliationAPI.GET("/acts/:id", handlers.GetReconciliationActByID)
    reconciliationAPI.GET("/acts/:id/history", handlers.GetActHistory)
    reconciliationAPI.GET("/statistics", handlers.GetActStatistics)
    reconciliationAPI.POST("/bulk-delete", handlers.BulkDeleteReconciliationActs)
    reconciliationAPI.POST("/acts/:id/sign", handlers.SignReconciliationAct)
    reconciliationAPI.PUT("/acts/:id", handlers.UpdateReconciliationAct)
    reconciliationAPI.DELETE("/acts/:id", handlers.DeleteReconciliationAct)
    reconciliationAPI.GET("/download/:id", handlers.DownloadReconciliationAct)
    reconciliationAPI.POST("/acts/:id/restore", handlers.RestoreReconciliationAct) // восстановить
    reconciliationAPI.DELETE("/trash", handlers.ClearTrashReconciliationActs)      // очистить корзину
    reconciliationAPI.GET("/trash", handlers.GetTrashReconciliationActs)           // список корзины
    reconciliationAPI.DELETE("/trash/selected", handlers.PermanentDeleteSelectedActs)
    reconciliationAPI.POST("/generate-pdf", handlers.GeneratePDF)

    reconciliationAPI.GET("/qr/:id", handlers.GenerateQRCodeForAct)
    reconciliationAPI.GET("/ai-verify/:id", handlers.AIVerifySignature)
    reconciliationAPI.GET("/compare/:id", handlers.CompareWithPrevious)
    reconciliationAPI.POST("/telegram/:id", handlers.SendToTelegram)
    reconciliationAPI.POST("/whatsapp/:id", handlers.SendToWhatsApp)

    reconciliationAPI.POST("/send-to-counterparty/:id", handlers.SendSignLinkToCounterparty)
    reconciliationAPI.POST("/their-sign/:id", handlers.TheirSignAct)
    reconciliationAPI.GET("/dashboard", handlers.GetReconciliationDashboard)
    reconciliationAPI.POST("/batch-create", handlers.BatchCreateReconciliationActs)

    // Банковские выписки
reconciliationAPI.POST("/import-bank-statement", handlers.BankStatementImport)
reconciliationAPI.POST("/acts/:id/reconcile-bank", handlers.AutoReconcileWithBank)
reconciliationAPI.GET("/acts/:id/bank-status", handlers.GetBankReconciliationStatus)
reconciliationAPI.GET("/banks", handlers.GetAvailableBanks)
reconciliationAPI.POST("/mass-create-acts", handlers.MassAutoCreateActs)

// Детализация актов
reconciliationAPI.GET("/acts/:id/details", handlers.GetActDetails)
reconciliationAPI.POST("/acts/:id/details", handlers.AddActDetail)
reconciliationAPI.DELETE("/acts/:id/details/:detail_id", handlers.DeleteActDetail)

// Экспорт в XML FinCore
reconciliationAPI.GET("/acts/:id/export/xml", handlers.ExportToFinCoreXML)

// Комментарии к актам
reconciliationAPI.GET("/acts/:id/comments", handlers.GetActComments)
reconciliationAPI.POST("/acts/:id/comments", handlers.AddActComment)
reconciliationAPI.DELETE("/acts/:id/comments/:comment_id", handlers.DeleteActComment)
}

// Настройки компании (для FinCore)
companyAPI := r.Group("/api/company")
companyAPI.Use(middleware.AuthMiddleware(cfg))
{
    companyAPI.GET("/settings", handlers.GetCompanySettings)
    companyAPI.PUT("/settings", handlers.UpdateCompanyDetails)
    companyAPI.POST("/upload-stamp", handlers.UploadCompanyStamp)
}

journalAPI := r.Group("/api/journal")
journalAPI.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("journal"))
{
    journalAPI.POST("/entry", handlers.CreateJournalEntry)
    journalAPI.GET("/entries", handlers.GetJournalEntries)
    journalAPI.PUT("/entry/:id", handlers.UpdateJournalEntry)
    journalAPI.DELETE("/entry/:id", handlers.DeleteJournalEntry)

// ДОБАВИТЬ ВНУТРЬ journalAPI группы:
journalAPI.POST("/entries/bulk", handlers.BulkCreateJournalEntries)
journalAPI.POST("/entries/import", handlers.ImportJournalEntries)
journalAPI.GET("/entries/export", handlers.ExportJournalEntries)
}

// ========== НАЛОГОВАЯ ОТЧЁТНОСТЬ API ==========
taxAPI := r.Group("/api/tax")
taxAPI.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("tax-reporting"))
{
    taxAPI.POST("/generate/usn", handlers.GenerateUSN)
    taxAPI.POST("/generate/ndfl", handlers.GenerateNDFL)
    taxAPI.POST("/generate/rsv", handlers.GenerateRSV)
    taxAPI.POST("/generate/nds", handlers.GenerateNDS)
    taxAPI.GET("/reports", handlers.GetTaxReports)
    taxAPI.GET("/report/:id", handlers.GetTaxReportByID)
    taxAPI.DELETE("/report/:id", handlers.DeleteTaxReport)
    taxAPI.GET("/export/xml/:id", handlers.ExportTaxReportXML)
    taxAPI.GET("/view/:id", handlers.ViewTaxReport)
    taxAPI.POST("/send/:id", handlers.SendTaxReport)
    taxAPI.POST("/create-tables", handlers.CreateTaxTables)
    taxAPI.GET("/check/:id", handlers.CheckTaxReportStatus)
    taxAPI.GET("/view-xml/:id", handlers.ViewXMLReport)
    taxAPI.GET("/view-pretty/:id", handlers.ViewPrettyReport)
    taxAPI.PUT("/report/:id", handlers.UpdateTaxReport)


taxAPI.GET("/penalty/:id", handlers.CalculateReportPenalty)
taxAPI.GET("/validate/:id", handlers.ValidateReport)
taxAPI.POST("/clone/:id", handlers.CloneReport)
taxAPI.GET("/compare", handlers.CompareReports)
taxAPI.GET("/export-excel/:id", handlers.ExportToExcel)
taxAPI.GET("/deadlines", handlers.GetDeadlineNotifications)
}

// ========== НАСТРОЙКИ ФНС ==========
fnsAPI := r.Group("/api/fns")
fnsAPI.Use(middleware.AuthMiddleware(cfg))
{
    fnsAPI.GET("/settings", handlers.GetFNSSettings)
    fnsAPI.POST("/settings", handlers.SaveFNSSettings)
    fnsAPI.GET("/report-status/:id", handlers.CheckTaxReportStatus)
}

r.GET("/api/tax/diagnose", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), handlers.DiagnoseTaxReports)


    r.GET("/marketplace", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("marketplace"), handlers.MarketplacePageHandler)
        // ========== НОВЫЕ РОУТЫ ==========
    
    // Universal AI Assistant - страница
    r.GET("/ai-assistant", func(c *gin.Context) {
        c.HTML(http.StatusOK, "ai_assistant_page.html", gin.H{
            "title": "AI-ассистент Business Stack",
        })
    })

    // Universal AI Assistant API
    r.POST("/api/ai/universal/chat", universalAI.ChatHandler)
    r.GET("/api/ai/universal/history", universalAI.GetHistory)
    r.GET("/api/ai/universal/actions", universalAI.GetActions)
    r.GET("/api/ai/universal/settings", universalAI.GetSettings)

// ========== НОВЫЕ РОУТЫ ДЛЯ АВТОНОМНОГО AI ==========
// AI Executor - выполняет действия
r.POST("/api/ai/executor/chat", func(c *gin.Context) {
    // Получаем tenant_id из контекста
    tenantID := c.GetString("tenant_id")
if tenantID == "" {
    tenantID = c.GetString("tenant_id_string")
}
if tenantID == "" {
    c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
    return
}
    userID := c.GetString("user_id")
    if userID == "" {
        userID = "system"
    }
    
    var req struct {
        Message string `json:"message"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // Анализируем намерение
    intent, entities := services.AnalyzeIntentExtended(req.Message)
    
    // Выполняем действие
    result := aiExecutor.ExecuteAction(tenantID, userID, intent, entities)
    
    // Сохраняем историю
    aiExecutor.SaveActionHistory(tenantID, userID, intent.Action, entities, result.Data, result.Error)
    
    // Запускаем workflows
    if result.Success {
        // Приводим result.Data к map[string]interface{}
var resultData map[string]interface{}
if result.Data != nil {
    if data, ok := result.Data.(map[string]interface{}); ok {
        resultData = data
    }
}
workflowResults := workflowEngine.ExecuteWorkflows(tenantID, intent.Action, resultData)
        if len(workflowResults) > 0 {
            result.Message += "\n\n📋 Автоматически выполнено:\n" + strings.Join(workflowResults, "\n")
        }
    }
    
    c.JSON(200, gin.H{
        "response": result.Message,
        "success":  result.Success,
        "action":   intent.Action,
        "module":   intent.Module,
        "data":     result.Data,
    })
})

// Получить список workflows
r.GET("/api/ai/workflows", func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
if tenantID == "" {
    tenantID = c.GetString("tenant_id_string")
}
if tenantID == "" {
    c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
    return
}
    workflows, err := workflowEngine.GetWorkflows(tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, workflows)
})

// Создать workflow
r.POST("/api/ai/workflows", middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
if tenantID == "" {
    tenantID = c.GetString("tenant_id_string")
}
if tenantID == "" {
    c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
    return
}
    
    var req struct {
        Name         string          `json:"name"`
        TriggerEvent string          `json:"trigger_event"`
        Actions      json.RawMessage `json:"actions"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    err := workflowEngine.CreateWorkflow(tenantID, req.Name, req.TriggerEvent, req.Actions)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Получить историю действий AI
r.GET("/api/ai/history", func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
if tenantID == "" {
    tenantID = c.GetString("tenant_id_string")
}
if tenantID == "" {
    c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
    return
}
    history, err := aiExecutor.GetActionHistory(tenantID, 50)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, history)
})

// Получить рекомендации AI
r.GET("/api/ai/recommendations", func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = c.GetString("tenant_id_string")
    }
    if tenantID == "" {
        c.JSON(401, gin.H{"error": "Unauthorized - tenant not found"})
        return
    }
    recommendations, err := workflowEngine.GetRecommendations(tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, recommendations)
})    
    // Individual Orders - страницы
    r.GET("/individual-order", individualOrdersHandler.OrderPage)
    r.GET("/admin/orders", individualOrdersHandler.AdminOrdersPage)
    
    // Individual Orders API (публичные)
    r.GET("/api/price", individualOrdersHandler.GetPrice)
    r.GET("/api/services", individualOrdersHandler.GetServices)
    r.GET("/api/categories", individualOrdersHandler.GetCategories)
    r.POST("/api/individual-order", individualOrdersHandler.CreateOrder)
    
    // Individual Orders API (админские - защищенные)
    adminOrdersAPI := r.Group("/api/admin/orders")
    adminOrdersAPI.Use(middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg))
    {
        adminOrdersAPI.GET("", individualOrdersHandler.GetOrders)
        adminOrdersAPI.GET("/:id", individualOrdersHandler.GetOrder)
        adminOrdersAPI.PUT("/:id/status", individualOrdersHandler.UpdateOrderStatus)
        adminOrdersAPI.DELETE("/:id", individualOrdersHandler.DeleteOrder)
        adminOrdersAPI.GET("/stats", individualOrdersHandler.GetOrderStats)
    }




    // QR код авторизация
    r.GET("/qr-login", handlers.QRLoginPageHandler)
    r.POST("/api/qr/generate", handlers.GenerateQRCode)
    r.GET("/api/qr/status", handlers.QRStatusWebSocket)
    r.POST("/api/qr/scan", handlers.ScanQRCode)
    r.POST("/api/qr/approve", handlers.ApproveQRLogin)

r.GET("/qr/approve-page", handlers.QRApprovePageHandler)
r.GET("/api/qr/reset-status", handlers.QRResetStatusWebSocket)


    r.GET("/logout", handlers.LogoutHandler)

    // Телефонная авторизация
    r.POST("/api/auth/send-code", handlers.SendPhoneCode)
    r.POST("/api/auth/verify-code", handlers.VerifyPhoneCode)

    // Push уведомления
    r.POST("/api/push/register", handlers.RegisterPushDevice)
    r.GET("/api/push/devices", handlers.GetUserDevices)
    r.DELETE("/api/push/devices/:id", handlers.RemovePushDevice)
    
    r.GET("/api-sales", handlers.APISalesPageHandler)           
    r.GET("/api/user/plan", handlers.GetUserPlan)                
    r.POST("/api/create-key", handlers.CreateAPIKey)             
    r.POST("/api/upgrade-key", handlers.UpgradeAPIKey)           
    r.GET("/api/user/usage", handlers.GetAPIUsage)  

  

// Поставщики (улучшенные с кешированием)
if supplierHandler != nil {
    supplierAPI := r.Group("/api/suppliers")
    supplierAPI.Use(middleware.AuthMiddleware(cfg))
    {
        supplierAPI.GET("", supplierHandler.GetSuppliers)
        supplierAPI.GET("/:id", supplierHandler.GetSupplier)
        supplierAPI.POST("", supplierHandler.CreateSupplier)
        supplierAPI.PUT("/:id", supplierHandler.UpdateSupplier)
        supplierAPI.DELETE("/:id", supplierHandler.DeleteSupplier)
        supplierAPI.GET("/stats", supplierHandler.GetSupplierStats)
    }
    
    // Заказы поставщикам
    purchaseAPI := r.Group("/api/purchase-orders")
    purchaseAPI.Use(middleware.AuthMiddleware(cfg))
    {
        purchaseAPI.GET("", supplierHandler.GetPurchaseOrders)
        purchaseAPI.GET("/:id", supplierHandler.GetPurchaseOrder)
        purchaseAPI.POST("", supplierHandler.CreatePurchaseOrder)
        purchaseAPI.PUT("/:id/status", supplierHandler.UpdatePurchaseOrderStatus)
        purchaseAPI.DELETE("/:id", supplierHandler.DeletePurchaseOrder)
    }
} else {
    // Fallback на старые хендлеры - закомментированы
    // r.GET("/api/suppliers", handlers.GetSuppliers)
    // r.GET("/api/suppliers/:id", handlers.GetSupplier)
    // r.POST("/api/suppliers", handlers.CreateSupplier)
    // r.PUT("/api/suppliers/:id", handlers.UpdateSupplier)
    // r.DELETE("/api/suppliers/:id", handlers.DeleteSupplier)
    // 
    // r.GET("/api/purchase-orders", handlers.GetPurchaseOrders)
    // r.GET("/api/purchase-orders/:id", handlers.GetPurchaseOrder)
    // r.POST("/api/purchase-orders", handlers.CreatePurchaseOrder)
    // r.PUT("/api/purchase-orders/:id/status", handlers.UpdatePurchaseOrderStatus)
    // r.DELETE("/api/purchase-orders/:id", handlers.DeletePurchaseOrder)
}

// Приемка товаров (улучшенная с кешированием)
if receiptHandler != nil {
    receiptAPI := r.Group("/api/goods-receipts")
    receiptAPI.Use(middleware.AuthMiddleware(cfg))
    {
        receiptAPI.GET("", receiptHandler.GetGoodsReceipts)
        receiptAPI.GET("/:id", receiptHandler.GetGoodsReceipt)
        receiptAPI.POST("", receiptHandler.CreateGoodsReceipt)
        receiptAPI.GET("/stats", receiptHandler.GetReceiptStats)
    }
} else {
    // Fallback на старые хендлеры - закомментированы
    // r.GET("/api/goods-receipts", handlers.GetGoodsReceipts)
    // r.GET("/api/goods-receipts/:id", handlers.GetGoodsReceipt)
    // r.POST("/api/goods-receipts", handlers.CreateGoodsReceipt)
}

// Инвентаризация (улучшенная с кешированием)
if inventoryHandler != nil {
    // API для товаров и складов
    inventoryAPI := r.Group("/api/inventory")
    inventoryAPI.Use(middleware.AuthMiddleware(cfg))
    {
        // Товары
        inventoryAPI.GET("", inventoryHandler.GetProducts)
        inventoryAPI.POST("", inventoryHandler.CreateProduct)
        inventoryAPI.PUT("/:id", inventoryHandler.UpdateProduct)
        inventoryAPI.DELETE("/:id", inventoryHandler.DeleteProduct)
        inventoryAPI.GET("/stats", inventoryHandler.GetInventoryStats)
        inventoryAPI.GET("/low-stock", inventoryHandler.GetLowStock)
        inventoryAPI.GET("/export", inventoryHandler.ExportProductsCSV)
        inventoryAPI.POST("/bulk", inventoryHandler.BulkUpdateInventory)
        inventoryAPI.GET("/:id/movements", inventoryHandler.GetProductMovements)
        
               // СКЛАДЫ (ТЕРМИНАЛЫ)
        inventoryAPI.GET("/warehouses", inventoryHandler.GetWarehouses)
        inventoryAPI.POST("/warehouses", inventoryHandler.CreateWarehouse)
        inventoryAPI.PUT("/warehouses/:id", inventoryHandler.UpdateWarehouse)
        inventoryAPI.DELETE("/warehouses/:id", inventoryHandler.DeleteWarehouse)
        inventoryAPI.GET("/warehouses/:id/stats", inventoryHandler.GetWarehouseStats)
        
        // ПОДКЛЮЧЕНИЕ ТЕРМИНАЛОВ (управление)
        inventoryAPI.POST("/terminals/connect", inventoryHandler.ConnectTerminal)
        inventoryAPI.POST("/terminals/disconnect/:id", inventoryHandler.DisconnectTerminal)
        
        // ТЕРМИНАЛЫ (полноценное API для клиентов)
        inventoryAPI.GET("/terminals", inventoryHandler.GetTerminals)
        inventoryAPI.POST("/terminals/register", inventoryHandler.RegisterTerminal)
        inventoryAPI.POST("/terminals/scan", inventoryHandler.TerminalScan)
        inventoryAPI.GET("/terminals/:id/stats", inventoryHandler.GetTerminalStats)
        
        // СОЗДАНИЕ ТАБЛИЦ (админ)
        inventoryAPI.POST("/create-tables", middleware.AdminMiddleware(cfg), inventoryHandler.CreateWarehouseTables)
    }
}

// Страница склада (всегда одна)
r.GET("/inventory", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("inventory"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "inventory.html", gin.H{
        "title": "Инвентаризация | Business Stack",
    })
})
// Страница приемки (всегда одна)
r.GET("/goods-receipts", func(c *gin.Context) {
    c.HTML(http.StatusOK, "goods_receipts.html", gin.H{
        "title": "Приемка товаров | Business Stack",
    })
})
  r.GET("/finance", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("finance"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "finance.html", gin.H{
        "title": "Финансовый учет | Business Stack",
    })
})
 
// Расширенные отчёты
r.GET("/api/reports/advanced-osv", handlers.GetAdvancedTurnoverBalance)
r.GET("/api/reports/profit-loss-detailed", handlers.GetProfitLossDetailed)
r.GET("/api/reports/cash-flow", handlers.GetCashFlowReport)

// ==========  РАСШИРЕННЫЕ ОТЧЁТЫ  ==========
r.GET("/api/reports/turnover-balance-grouped", handlers.GetTurnoverBalanceSheetGrouped)
r.GET("/api/reports/balance-sheet-detailed", handlers.GetDetailedBalanceSheet)
r.GET("/api/reports/cash-flow-full", handlers.GetFullCashFlowReport)
r.GET("/api/reports/profit-loss-detailed-new", handlers.GetDetailedProfitLoss)
r.GET("/api/reports/compare-periods", handlers.ComparePeriods)


// Управленческий учёт (дополнительные)
fincoreBudgets := r.Group("/api/fincore/budgets")
fincoreBudgets.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"))
{
    fincoreBudgets.GET("", handlers.GetBudgets)
    fincoreBudgets.POST("", handlers.UpdateBudget)
    fincoreBudgets.GET("/plan-fact", handlers.GetPlanFactAnalysis) // НОВЫЙ
}

// План-факт анализ
r.GET("/api/fincore/plan-fact", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"), handlers.GetPlanFactAnalysis)

// Быстрые шаблоны проводок
r.POST("/api/fincore/template-posting", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"), handlers.CreateTemplatePosting)
r.GET("/api/fincore/templates", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"), handlers.GetPostingTemplates)

// Закрытие месяца (улучшенное)
r.POST("/api/fincore/close-month", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"), handlers.CloseMonth)
r.GET("/api/fincore/month-closing-status", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"), handlers.GetMonthClosingStatus)

    r.GET("/api/admin/create-inventory-tables", handlers.CreateInventoryTables)
    r.GET("/api/current-user", middleware.AuthMiddleware(cfg), handlers.GetCurrentUserID)

    r.GET("/api/admin/create-vpn-tables", handlers.CreateVPNTables)

    r.GET("/api/backup", handlers.CreateBackup)
    r.POST("/api/restore", handlers.RestoreBackup)
    
    // Страница поставщиков
r.GET("/suppliers", func(c *gin.Context) {
    // Проверяем существование шаблона
    c.HTML(http.StatusOK, "suppliers.html", gin.H{
        "title": "Поставщики | Business Stack",
        "message": "Управление поставщиками",
    })
})
    r.GET("/inventory/products", func(c *gin.Context) {
        c.HTML(http.StatusOK, "inventory_products.html", gin.H{
            "title": "Товары - Business Stack",
        })
    })

    // Страница закупок
    r.GET("/purchases", func(c *gin.Context) {
        c.Header("Cache-Control", "no-cache, no-store, must-revalidate, private")
        c.Header("Pragma", "no-cache")
        c.Header("Expires", "0")
        c.Header("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
        c.Header("ETag", "")
        c.HTML(http.StatusOK, "purchases.html", gin.H{
            "title": "Закупки | Business Stack",
            "cacheBuster": time.Now().UnixNano(),
        })
    })

    // Уведомления
    r.GET("/api/notifications", handlers.GetNotifications)
    r.PUT("/api/notifications/:id/read", handlers.MarkNotificationRead)
    r.GET("/api/notifications/unread", handlers.GetUnreadCount)

    // Экспорт отчетов
    r.GET("/api/reports/export/osv", handlers.ExportOSVToExcel)
    r.GET("/api/reports/export/profit-loss", handlers.ExportProfitLossToExcel)
    r.GET("/api/reports/export/month-closure", middleware.AuthMiddleware(cfg), handlers.ExportMonthClosureReport)

    // Гант-диаграмма
    r.GET("/api/gantt", handlers.GetGanttData)

    // Обновление статуса заказа
    //r.PUT("/api/inventory/orders/:id/status", handlers.UpdateOrderStatus)

    // Отчеты
    //r.GET("/api/inventory/reports/sales", handlers.GetSalesReport)
    //r.GET("/api/inventory/reports/top-products", handlers.GetTopProducts)

    // OAuth2 / OpenID Connect маршруты
    r.GET("/.well-known/openid-configuration", handlers.OIDCConfigurationHandler)
    r.GET("/oauth/jwks", handlers.JWKSHander)
    r.GET("/oauth/authorize", handlers.OAuthAuthorizeHandler)
    r.POST("/oauth/token", handlers.OAuthTokenHandler)
    r.GET("/oauth/userinfo", handlers.OAuthUserInfoHandler)
  r.GET("/identity-hub", 
    middleware.AuthMiddleware(cfg), 
    handlers.IdentityHubRouter)
    // Developer Portal - доступен всем, но контент зависит от роли
r.GET("/developer-portal", middleware.AuthMiddleware(cfg), handlers.DeveloperPortalHandler)
  

    // ========== РАСШИРЕННЫЕ API ДЛЯ IDENTITY HUB ==========
    identityAPI := r.Group("/api/identity")
    identityAPI.Use(middleware.AuthMiddleware(cfg))
    {
        identityAPI.GET("/stats", handlers.GetIdentityHubStats)
        identityAPI.GET("/sessions", handlers.GetUserSessionsList)
        identityAPI.DELETE("/sessions/:id", handlers.RevokeUserSession)
        identityAPI.GET("/apps", handlers.GetConnectedApps)
        identityAPI.DELETE("/apps/:id", handlers.RevokeAppAccess)
        identityAPI.GET("/activity", handlers.GetActivityLog)
        identityAPI.GET("/activity/export", handlers.ExportActivityLog)
    }

    // Админские API для OAuth клиентов
    adminOAuthAPI := r.Group("/api/admin/oauth")
    adminOAuthAPI.Use(middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg))
    {
        adminOAuthAPI.GET("/clients", handlers.GetOAuthClientsList)
    }

    // ========== ОТЧЕТЫ И АНАЛИТИКА ==========
    r.GET("/api/reports/turnover-balance", handlers.GetTurnoverBalanceSheet)
    r.GET("/api/reports/profit-loss", handlers.GetProfitAndLoss)
    r.GET("/api/reports/dashboard-stats", handlers.GetDashboardStats)
    r.GET("/api/reports/sales-chart", handlers.GetSalesChart)

// ========== ДОПОЛНИТЕЛЬНЫЕ ОТЧЁТЫ ==========
r.GET("/api/reports/balance-sheet", handlers.GetBalanceSheet)
r.GET("/api/reports/cash-flow-detailed", handlers.GetCashFlowReport)


    r.GET("/reports", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("reports-analytics"), func(c *gin.Context) {
        c.HTML(http.StatusOK, "reports.html", gin.H{
            "title": "Отчеты и аналитика | Business Stack",
        })
    })

// Страница журнала проводок
r.GET("/journal", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("journal"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "journal_entries.html", gin.H{
        "title": "Журнал проводок | FinCore",
    })
})
    // ========== ИНТЕГРАЦИЯ С 1С ==========
    r.GET("/api/1c/export/products", handlers.ExportProductsTo1C)
    r.GET("/api/1c/export/orders", handlers.ExportOrdersTo1C)
    r.POST("/api/1c/import/products", handlers.ImportProductsFrom1C)
    r.GET("/api/1c/logs", handlers.GetSyncLogs)
    r.GET("/api/1c/settings", handlers.GetSyncSettings)
    r.POST("/api/1c/settings", handlers.UpdateSyncSettings)
    r.GET("/integration/1c", func(c *gin.Context) {
        c.HTML(http.StatusOK, "integration_1c.html", gin.H{
            "title": "Интеграция с 1С | Business Stack",
        })
    })
    r.POST("/api/1c/webhook", handlers.AddWebhookHandler)

    // ========== BITRIX24 ==========
    r.GET("/api/bitrix/settings", handlers.GetBitrixSettings)
    r.POST("/api/bitrix/settings", handlers.SaveBitrixSettings)
    r.POST("/api/bitrix/export/lead", handlers.ExportLeadToBitrix)
    r.GET("/api/bitrix/import/leads", handlers.ImportLeadsFromBitrix)
    r.POST("/api/bitrix/sync/contacts", handlers.SyncBitrixContacts)
    r.GET("/api/bitrix/logs", handlers.GetBitrixSyncLogs)
    r.GET("/integration/bitrix", func(c *gin.Context) {
        c.HTML(http.StatusOK, "integration_bitrix.html", gin.H{
            "title": "Интеграция с Bitrix24 | Business Stack",
        })
    })
    r.POST("/api/bitrix/task", handlers.SyncTasksToBitrix)
    r.GET("/api/bitrix/tasks", handlers.GetBitrixTasks)
    r.POST("/api/bitrix/webhook", handlers.BitrixWebhookHandler)

    // TeamSphere - Bitrix24 Alternative
    r.GET("/teamsphere", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("teamsphere"), func(c *gin.Context) {
        c.HTML(http.StatusOK, "teamsphere_welcome.html", gin.H{
            "title": "TeamSphere | Добро пожаловать",
        })
    })

    r.GET("/teamsphere/dashboard", handlers.TeamSphereDashboard)
    r.GET("/integrations", handlers.IntegrationsHandler)
    
    // Projects page
    r.GET("/projects", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("projects"), handlers.ProjectsPageHandler)

 // HR маршруты
hr := r.Group("/hr")
hr.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("hr"))
{
    hr.GET("/", handlers.HRDashboardHandler)
    
    // Employees
    hr.GET("/api/employees", handlers.GetEmployeesHandler)
    hr.POST("/api/employees", handlers.AddEmployeeHandler)
    hr.PUT("/api/employees/:id", handlers.UpdateEmployeeHandler)
    hr.DELETE("/api/employees/:id", handlers.DeleteEmployeeHandler)
    
    // Vacations
    hr.GET("/api/vacations", handlers.GetVacationRequestsHandler)
    hr.POST("/api/vacations", handlers.AddVacationRequestHandler)
    hr.POST("/api/vacations/:id/approve", handlers.ApproveRequestHandler)
    hr.POST("/api/vacations/:id/reject", handlers.RejectRequestHandler)
    
    // Candidates
    hr.GET("/api/candidates", handlers.GetCandidatesHandler)
    hr.POST("/api/candidates", handlers.AddCandidateHandler)
    hr.PUT("/api/candidates/:id/status", handlers.UpdateCandidateStatusHandler)
    hr.DELETE("/api/candidates/:id", handlers.DeleteCandidateHandler)
    
    // Vacancies (НОВЫЕ МАРШРУТЫ)
    hr.GET("/api/vacancies", handlers.GetVacanciesHandler)
    hr.POST("/api/vacancies", handlers.AddVacancyHandler)
    hr.PUT("/api/vacancies/:id", handlers.UpdateVacancyHandler)
    hr.DELETE("/api/vacancies/:id", handlers.DeleteVacancyHandler)
   
    
    // Statistics
    hr.GET("/api/statistics", handlers.GetStatisticsHandler)
    hr.POST("/api/candidates/:id/analyze", handlers.AnalyzeCandidateHandler)
    hr.POST("/api/ai/chat", handlers.AIChatHandler)
    hr.GET("/api/training/suggestions", handlers.SuggestTrainingHandler)
    hr.GET("/api/turnover/predict", handlers.PredictTurnoverHandler)
    hr.POST("/api/orders/generate", handlers.GenerateOrderHandler)
    hr.GET("/api/departments", handlers.GetDepartmentsHandler)
}

// HH Integration маршруты
hhAPI := r.Group("/api/hh")
hhAPI.Use(middleware.AuthMiddleware(cfg))
{
    hhAPI.GET("/references", handlers.GetHHReferences)
    hhAPI.POST("/vacancies/publish", handlers.PublishVacancyToHHClient)
    hhAPI.GET("/vacancies/:id/status", handlers.GetVacancyPublishStatus)


}

// Платформы для публикации
platformAPI := r.Group("/api/platforms")
platformAPI.Use(middleware.AuthMiddleware(cfg))
{
    platformAPI.POST("/avito/publish", handlers.PublishToAvito)
    platformAPI.POST("/rabota/publish", handlers.PublishToRabotaRu)
    platformAPI.GET("/:platform/:id/status", handlers.GetPlatformPublishStatus)
}

// ========== ОТКЛИКИ И ПЕРЕПИСКА ==========
responsesAPI := r.Group("/api/responses")
responsesAPI.Use(middleware.AuthMiddleware(cfg))
{
    responsesAPI.POST("/sync", handlers.SyncAllPlatformResponses)
    responsesAPI.GET("/", handlers.GetPlatformResponses)
    responsesAPI.PUT("/:id/status", handlers.UpdateResponseStatus)
    responsesAPI.POST("/:id/view", handlers.MarkResponseViewed)
    responsesAPI.GET("/new/count", handlers.GetNewResponsesCount)
}
    // ========== АРХИВ ==========
    archiveGroup := r.Group("/archive")
    archiveGroup.Use(middleware.AuthMiddleware(cfg))
    {
        archiveGroup.GET("/", handlers.ArchivePageHandler)
        archiveGroup.GET("/api/stats", handlers.GetArchiveStats)
        archiveGroup.GET("/api/items", handlers.GetArchiveItems)
        archiveGroup.POST("/api/restore/:type/:id", handlers.RestoreFromArchive)
        archiveGroup.POST("/api/upgrade", handlers.UpgradeArchiveQuota)
        archiveGroup.GET("/api/notifications", handlers.GetNotifications)
        archiveGroup.POST("/api/notifications/:id/read", handlers.MarkNotificationRead)
        archiveGroup.GET("/api/auto-settings", handlers.GetAutoArchiveSettings)
        archiveGroup.POST("/api/auto-settings", handlers.UpdateAutoArchiveSettings)
        archiveGroup.POST("/api/run-auto-archive", handlers.RunAutoArchive)
        archiveGroup.GET("/api/trash", handlers.GetTrashItems)
        archiveGroup.POST("/api/trash/:type/:id", handlers.MoveToTrash)
        archiveGroup.POST("/api/trash/restore/:id", handlers.RestoreFromTrash)
        archiveGroup.GET("/api/logs", handlers.GetArchiveLogs)
        archiveGroup.GET("/api/export", handlers.ExportArchiveToExcel)
        archiveGroup.GET("/api/plan", handlers.GetCurrentPlan)
        archiveGroup.DELETE("/api/trash/:id", handlers.DeleteFromTrashPermanently)
        archiveGroup.DELETE("/api/trash/clear", handlers.ClearTrashBin)

 // ===== НОВЫЕ МАРШРУТЫ ДЛЯ АРХИВА ЖУРНАЛА ПРОВОДОК (FINCORE) =====
    // Эти маршруты будут доступны по /archive/fincore/...
    fincore := archiveGroup.Group("/fincore")
    {
        // Архивировать проводку (переместить в архив)
        fincore.POST("/archive", handlers.ArchiveFincoreEntity)
        
        // Получить список архивированных проводок с фильтрацией
        // Параметры: ?status=draft|posted|all&search=текст&date_from=2026-01-01&date_to=2026-12-31&days=0_7|8_14|15_21|22_30&page=1&limit=20
        fincore.GET("/list", handlers.GetFincoreArchiveList)
        
        // Восстановить проводку из архива (только если не прошло 30 дней)
        fincore.POST("/restore/:id", handlers.RestoreFincoreFromArchive)
        
        // Удалить проводку из архива навсегда
        fincore.DELETE("/permanent/:id", handlers.PermanentDeleteFincoreFromArchive)
        
        // Получить статистику архива
        fincore.GET("/stats", handlers.GetFincoreArchiveStats)
        
        // Очистить весь архив (удалить все записи навсегда)
        fincore.DELETE("/clear-all", handlers.ClearAllFincoreArchive)
        
        // Массовое восстановление (массив ID в теле запроса: {"ids": ["id1", "id2"]})
        fincore.POST("/mass-restore", handlers.MassRestoreFromArchive)
        
        // Массовое удаление навсегда (массив ID в теле запроса: {"ids": ["id1", "id2"]})
        fincore.POST("/mass-delete", handlers.MassPermanentDeleteFromArchive)
    }
    }
// ========== БАНК-КЛИЕНТ ==========
bankAPI := r.Group("/api/bank")
bankAPI.Use(middleware.AuthMiddleware(cfg))
{
    // Основные счета
    bankAPI.GET("/accounts", handlers.GetBankAccounts)
    bankAPI.POST("/connect", handlers.ConnectBankAccount)
    bankAPI.POST("/sync/:id", handlers.SyncBankStatements)
    bankAPI.POST("/match/:id", handlers.MatchTransactionsByAccount)
    
    // Выписки и транзакции
    bankAPI.GET("/statements", handlers.GetBankStatementsByAccount)
    bankAPI.GET("/statements/:id", handlers.GetBankStatementsByAccount)
    bankAPI.GET("/statements/export", handlers.ExportBankStatementsToExcel)
    bankAPI.POST("/transactions", handlers.AddTestTransaction)
    bankAPI.DELETE("/transactions/:id", handlers.DeleteTransaction)
    bankAPI.DELETE("/transactions/delete-all/:account_id", handlers.DeleteAllTransactions)
    
    // Импорт/экспорт
    bankAPI.POST("/import-statement", handlers.ImportStatementHandler)
    bankAPI.GET("/export-1c/:account_id", handlers.ExportTo1CFormat)
    bankAPI.GET("/test-excel", handlers.GenerateValidExcelFile)
    
    // Категории
    bankAPI.GET("/categories", handlers.GetPaymentCategories)
    bankAPI.POST("/categories", handlers.CreatePaymentCategory)
    bankAPI.DELETE("/categories/:id", handlers.DeletePaymentCategory)
    
    // Автоплатежи
    bankAPI.GET("/recurring", handlers.GetRecurringPayments)
    bankAPI.POST("/recurring", handlers.CreateRecurringPayment)
    bankAPI.DELETE("/recurring/:id", handlers.DeleteRecurringPayment)
    
    // API банка
    bankAPI.POST("/connect-api", handlers.ConnectBankAPI)
    bankAPI.POST("/sync-api", handlers.SyncViaBankAPI)
    
    // Статусы и категоризация
    bankAPI.PUT("/payments/:id/status", handlers.UpdatePaymentStatus)
    bankAPI.POST("/payments/auto-categorize", handlers.AutoCategorizePayments)
    
    // СВЕРКА (без дубликатов)
    bankAPI.GET("/reconciliation/data", handlers.GetReconciliationData)
    bankAPI.GET("/reconciliation/stats", handlers.GetReconciliationStats)
    bankAPI.GET("/reconciliation/reconciled", handlers.GetReconciledTransactions)
    bankAPI.POST("/reconcile-all", handlers.MassReconcile)
    bankAPI.POST("/reconcile-all/:account_id", handlers.MassReconcileAll)
    bankAPI.POST("/reconcile/:id", handlers.ReconcileTransaction) 
    bankAPI.POST("/undo-reconciliation/:id", handlers.UndoReconciliation)
    
    // Массовые операции
    bankAPI.POST("/transactions/delete", handlers.BulkDeleteTransactions)
    bankAPI.POST("/create-acts", handlers.BulkCreateActsFromTransactions)
    
    // Баланс и платежи
    bankAPI.GET("/accounts/:id/balance", handlers.GetAccountBalance)
    bankAPI.POST("/execute-payment", handlers.ExecutePayment)
}

    // ========== WHATSAPP BUSINESS API ==========
    whatsappAPI := r.Group("/api/whatsapp")
    whatsappAPI.Use(middleware.AuthMiddleware(cfg))
    {
        // Подключение и статус
        whatsappAPI.POST("/connect", handlers.ConnectWhatsApp)
        whatsappAPI.GET("/status", handlers.GetWhatsAppStatus)
        whatsappAPI.POST("/disconnect", handlers.DisconnectWhatsApp)
        
        // Отправка сообщений
        whatsappAPI.POST("/send", handlers.SendWhatsAppMessage)
        whatsappAPI.GET("/messages", handlers.GetWhatsAppMessages)
        whatsappAPI.GET("/messages/stats", handlers.GetWhatsAppMessageStats)
        
        // Шаблоны
        whatsappAPI.GET("/templates", handlers.GetWhatsAppTemplates)
        whatsappAPI.POST("/templates", handlers.CreateWhatsAppTemplate)
        whatsappAPI.PUT("/templates/:id", handlers.UpdateWhatsAppTemplate)
        whatsappAPI.DELETE("/templates/:id", handlers.DeleteWhatsAppTemplate)
        
        // Рассылки
        whatsappAPI.POST("/broadcast", handlers.CreateWhatsAppBroadcast)
        whatsappAPI.GET("/broadcasts", handlers.GetWhatsAppBroadcasts)
        whatsappAPI.POST("/broadcast/:id/send", handlers.SendWhatsAppBroadcast)
        whatsappAPI.DELETE("/broadcasts/:id", handlers.DeleteWhatsAppBroadcast)
        
        // Контакты и статистика
        whatsappAPI.GET("/contacts", handlers.GetWhatsAppContacts)
        whatsappAPI.GET("/stats", handlers.GetWhatsAppStats)
    }

    // Webhook для WhatsApp (публичный, без авторизации)
    r.POST("/webhook/whatsapp", handlers.WhatsAppWebhook)
    r.GET("/webhook/whatsapp", handlers.WhatsAppWebhook)

    // Страница WhatsApp
    r.GET("/whatsapp", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("whatsapp"), func(c *gin.Context) {
        c.HTML(http.StatusOK, "whatsapp.html", gin.H{
            "title": "WhatsApp Business | Business Stack",
        })
    })

    // ========== РЕЗЕРВНОЕ КОПИРОВАНИЕ ==========
    backupAPI := r.Group("/api/backup")
    backupAPI.Use(middleware.AuthMiddleware(cfg))
    {
        backupAPI.GET("/settings", handlers.GetBackupSettings)
        backupAPI.PUT("/settings", handlers.UpdateBackupSettings)
        backupAPI.POST("/create", handlers.CreateFullBackup)
        backupAPI.GET("/history", handlers.GetBackupHistory)
        backupAPI.GET("/download/:id", handlers.DownloadBackup)
        backupAPI.DELETE("/delete/:id", handlers.DeleteBackup)
    }

    // ========== AI ЧАТ-БОТ ДЛЯ САЙТА ==========
    chatbotAPI := r.Group("/api/chatbot")
    chatbotAPI.Use(middleware.AuthMiddleware(cfg))
    {
        chatbotAPI.GET("/settings", handlers.GetChatbotSettings)
        chatbotAPI.PUT("/settings", handlers.UpdateChatbotSettings)
        chatbotAPI.GET("/conversations", handlers.GetChatbotConversations)
        chatbotAPI.GET("/messages/:id", handlers.GetChatbotMessages)
        chatbotAPI.GET("/leads", handlers.GetChatbotLeads)
        chatbotAPI.POST("/lead", handlers.CreateChatbotLead)
    }

    // Публичные эндпоинты для виджета
    r.POST("/api/chatbot/message", handlers.SendChatbotMessage)
    r.GET("/chatbot-widget", handlers.ChatbotWidget)

    // Страница управления чат-ботом (ТОЛЬКО ДЛЯ АДМИНОВ И РАЗРАБОТЧИКОВ)
    r.GET("/chatbot", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
        c.HTML(http.StatusOK, "chatbot.html", gin.H{
            "title": "AI Чат-бот (Dev Mode) | Business Stack",
        })
    })
    
    // ========== ПАРТНЁРСКАЯ ПРОГРАММА ==========
    partnerAPI := r.Group("/api/partner")
    partnerAPI.Use(middleware.AuthMiddleware(cfg))
    {
        partnerAPI.GET("/stats", handlers.GetReferralStatsHandler)
        partnerAPI.GET("/friends", handlers.GetReferralFriendsHandler)
        partnerAPI.GET("/link", handlers.GetReferralLinkHandler)
        partnerAPI.POST("/payout", handlers.RequestReferralPayoutHandler)
        partnerAPI.GET("/payouts", handlers.GetReferralPayoutsHandler)
        partnerAPI.POST("/save-payout-details", handlers.SavePayoutDetailsHandler)
    }
    
    // Страница бэкапов
    r.GET("/backup", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("backup"), func(c *gin.Context) {
        c.HTML(http.StatusOK, "backup.html", gin.H{
            "title": "Резервное копирование | Business Stack",
        })
    })

    // Страница банк-клиента
    r.GET("/bank", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("bank-client"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "bank_integration.html", gin.H{
        "title": "Банк-клиент | Business Stack",
    })
})

// Страница актов сверки
r.GET("/reconciliation-acts", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("reconciliation"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "reconciliation_acts", gin.H{
        "title": "Акты сверки | FinCore",
    })
})
  // ========== РАСШИРЕННЫЙ ЗУП ==========
payrollAPI := r.Group("/api/payroll")
payrollAPI.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("payroll"))
{
    payrollAPI.GET("/employees", handlers.GetEmployeesForPayroll)
    payrollAPI.POST("/employees", handlers.AddEmployeeToPayroll)      
    payrollAPI.PUT("/employees/:id", handlers.UpdateEmployeeInPayroll) 
    payrollAPI.DELETE("/employees/:id", handlers.DeleteEmployeeFromPayroll) 
    payrollAPI.POST("/calculate", handlers.CalculatePayroll)
    payrollAPI.GET("/history", handlers.GetPayrollHistory)
    
    // ✅ ДОБАВИТЬ ЭТИ ТРИ СТРОКИ:
    payrollAPI.POST("/history", handlers.CreatePayrollHistory)       // создание записи в истории
    payrollAPI.PUT("/history/:id", handlers.UpdatePayrollHistory)    // обновление (статус paid)
    payrollAPI.DELETE("/history/:id", handlers.DeletePayrollHistory) // удаление записи

   payrollAPI.DELETE("/history/clear-all", handlers.ClearAllPayrollHistory)

// В секции payrollAPI добавьте:
payrollAPI.POST("/archive", handlers.SavePayslipToArchive)           // сохранение в архив
payrollAPI.GET("/archive", handlers.GetPayslipArchive)               // получение списка
payrollAPI.GET("/archive/:id", handlers.GetPayslipContent)           // получение содержимого
payrollAPI.DELETE("/archive/:id", handlers.DeletePayslipFromArchive) // удаление документа
payrollAPI.DELETE("/archive/clear-all", handlers.ClearPayslipArchive) // очистка всего архива
payrollAPI.GET("/benchmark", handlers.GetBenchmarkData)
    
    payrollAPI.POST("/pay", handlers.ProcessPayrollPayment)
    payrollAPI.POST("/tax-report", handlers.GenerateTaxReport)
    
    payrollAPI.POST("/sick-leave", handlers.CalculateSickLeave)
    payrollAPI.POST("/vacation", handlers.CalculateVacation)
    payrollAPI.POST("/alimony", handlers.CalculateAlimony)
    payrollAPI.POST("/payment-order", handlers.GeneratePaymentOrder)
    payrollAPI.GET("/employee/:id", handlers.GetEmployeePayrollDetails)
    payrollAPI.POST("/create-tables", handlers.CreatePayrollTables)
}


// ========== МАССОВЫЙ ИМПОРТ EXCEL ==========
importAPI := r.Group("/api/import")
importAPI.Use(middleware.AuthMiddleware(cfg))
{
    // Импорт банковских выписок
    importAPI.POST("/bank-statement", handlers.ImportBankStatement)
    importAPI.POST("/invoices", handlers.ImportInvoices)
    importAPI.POST("/acts", handlers.ImportActs)
    
    // Шаблоны
    importAPI.GET("/templates", handlers.GetImportTemplates)
    importAPI.POST("/create-tables", handlers.CreateImportTables)
}


// ========== ПОМОЩНИК ЗАКРЫТИЯ МЕСЯЦА ==========
monthEndAPI := r.Group("/api/month-end")
monthEndAPI.Use(middleware.AuthMiddleware(cfg))
{
    monthEndAPI.POST("/start", handlers.StartMonthEndClosing)
    monthEndAPI.GET("/status", handlers.GetMonthEndStatus)
    monthEndAPI.GET("/history", handlers.GetMonthEndHistory)
    monthEndAPI.POST("/create-tables", handlers.CreateMonthEndTables)
}

// API для управления данными клиента (статистика, экспорт, удаление)
clientDataAPI := r.Group("/api/client")
clientDataAPI.Use(middleware.AuthMiddleware(cfg))
{
    // Статистика и управление
    clientDataAPI.GET("/data-info", handlers.GetClientDataInfo)
    clientDataAPI.GET("/export-all-data", handlers.ExportAllData)
    clientDataAPI.DELETE("/delete-all-data", handlers.DeleteAllClientData)
    
    // ========== ПРОСМОТР ДАННЫХ (ДОБАВИТЬ ЭТИ СТРОКИ) ==========
    clientDataAPI.GET("/transactions", handlers.GetClientTransactions)
    clientDataAPI.GET("/payments", handlers.GetClientPayments)
    clientDataAPI.GET("/categories", handlers.GetClientCategories)
    clientDataAPI.GET("/recurring", handlers.GetClientRecurring)
    clientDataAPI.GET("/stats", handlers.GetClientStats)
    clientDataAPI.GET("/export", handlers.ExportClientData)
    clientDataAPI.DELETE("/delete-all", handlers.DeleteClientAllData)
}

// SQL-доступ заявки
sqlAPI := r.Group("/api/client")
sqlAPI.Use(middleware.AuthMiddleware(cfg))
{
    sqlAPI.POST("/request-sql-access", handlers.RequestSQLAccess)
}
// ========== TEAMSPHERE - СКРАМ-ДОСКА ==========
scrumAPI := r.Group("/api/scrum")
scrumAPI.Use(middleware.AuthMiddleware(cfg))
{
    // Спринты
    scrumAPI.POST("/sprint", handlers.CreateSprint)
    scrumAPI.GET("/sprints", handlers.GetSprints)
    scrumAPI.POST("/sprint/:id/start", handlers.StartSprint)
    scrumAPI.POST("/sprint/:id/complete", handlers.CompleteSprint)
    
    // Задачи
    scrumAPI.POST("/task", handlers.CreateScrumTask)
    scrumAPI.GET("/board/:sprint_id", handlers.GetScrumBoard)
    scrumAPI.PUT("/task/:id/status", handlers.UpdateTaskStatus)
    scrumAPI.POST("/tasks/reorder", handlers.ReorderTasks)
    
    // Комментарии
    scrumAPI.POST("/task/:id/comment", handlers.AddTaskComment)
    scrumAPI.GET("/task/:id/comments", handlers.GetTaskComments)
    
    // Аналитика
    scrumAPI.GET("/burndown/:sprint_id", handlers.GetBurndownChart)
    scrumAPI.GET("/velocity", handlers.GetVelocityChart)
    
    // Таблицы
    scrumAPI.POST("/create-tables", handlers.CreateScrumTables)
}

    // ========== EMAIL-МАРКЕТИНГ ==========
    emailAPI := r.Group("/api/email")
    emailAPI.Use(middleware.AuthMiddleware(cfg))
    {
        emailAPI.POST("/campaign", handlers.CreateEmailCampaign)
        emailAPI.GET("/campaigns", handlers.GetEmailCampaigns)
        emailAPI.POST("/campaign/:id/send", handlers.SendEmailCampaign)
        emailAPI.GET("/templates", handlers.GetEmailTemplates)
        emailAPI.POST("/templates", handlers.CreateEmailTemplate)
    }
    // Страница email-маркетинга
    r.GET("/email-marketing", func(c *gin.Context) {
        c.HTML(http.StatusOK, "email_marketing.html", gin.H{
            "title": "Email-маркетинг | Business Stack",
        })
    })

    // ========== МАРКЕТПЛЕЙС ==========
    marketplace := r.Group("/marketplace")
    marketplace.Use(middleware.AuthMiddleware(cfg))
    {
        marketplace.GET("/", handlers.MarketplacePageHandler)
        marketplace.GET("/api/apps", handlers.GetMarketplaceApps)
        marketplace.GET("/api/apps/:slug", handlers.GetMarketplaceApp)
        marketplace.POST("/api/purchase", handlers.PurchaseApp)
        marketplace.POST("/api/review", handlers.AddReview)
        marketplace.GET("/api/my-purchases", handlers.GetMyPurchases)
    }

    // ========== API МАРКЕТПЛЕЙСОВ (Ozon, WB, Яндекс) ==========
    marketplaceAPI := r.Group("/api/marketplace")
    marketplaceAPI.Use(middleware.AuthMiddleware(cfg))
    {
        marketplaceAPI.POST("/connect", handlers.ConnectMarketplace)
        marketplaceAPI.GET("/integrations", handlers.GetMarketplaceIntegrationsList)
        marketplaceAPI.POST("/sync/:id", handlers.SyncMarketplaceOrders)
        marketplaceAPI.GET("/orders", handlers.GetMarketplaceOrders)
        marketplaceAPI.POST("/stock", handlers.UpdateMarketplaceStock)
        marketplaceAPI.GET("/products/:id", handlers.GetMarketplaceProducts)
        marketplaceAPI.POST("/prices", handlers.UpdateMarketplacePrices)
        marketplaceAPI.GET("/analytics/:id", handlers.GetMarketplaceAnalytics)
        marketplaceAPI.DELETE("/disconnect/:id", handlers.DisconnectMarketplace)
    }

    // Страница маркетплейсов
    r.GET("/marketplace-integrations", func(c *gin.Context) {
        c.HTML(http.StatusOK, "marketplace_integrations.html", gin.H{
            "title": "Интеграция с маркетплейсами | Business Stack",
        })
    })

    // API для архивации из CRM
    crmArchive := r.Group("/api/crm")
    crmArchive.Use(middleware.AuthMiddleware(cfg))
    {
        crmArchive.POST("/customers/:id/archive", handlers.ArchiveCustomer)
    }
    
    // ========== PWA И PUSH УВЕДОМЛЕНИЯ ==========
    r.GET("/service-worker.js", func(c *gin.Context) { c.File("./static/service-worker.js") })
    r.GET("/manifest.json", func(c *gin.Context) { c.File("./static/manifest.json") })
    r.GET("/api/pwa/info", handlers.GetPWAInfo)
    r.POST("/api/push/subscribe", handlers.SavePushSubscription)
    r.GET("/api/push/subscriptions", handlers.GetPushSubscriptions)

    // Админские маршруты для управления OAuth клиентами
    adminOAuth := r.Group("/admin/oauth")
    adminOAuth.Use(middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg))
    {
        adminOAuth.GET("/clients", handlers.OAuthClientsPageHandler)
        adminOAuth.POST("/clients", handlers.CreateOAuthClient)
    }
    

// ========== VPN МАРШРУТЫ ==========
vpnGroup := r.Group("/vpn")
vpnGroup.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("vpn"))
{
    vpnGroup.GET("/", handlers.VPNSalesPageHandler)
    vpnGroup.GET("/keys", handlers.GetVPNKeysPage)
    vpnGroup.GET("/api/keys", handlers.GetVPNKeysAPI)
    vpnGroup.GET("/api/stats", handlers.GetVPNStatsAPI)
    vpnGroup.POST("/api/create", handlers.CreateVPNKey)
    vpnGroup.POST("/api/keys", handlers.CreateVPNKeyAPI)
    vpnGroup.GET("/api/config/:client", handlers.GetVPNConfig)
    vpnGroup.GET("/api/status/:client", handlers.CheckVPNKey)
    vpnGroup.POST("/api/renew/:client", handlers.RenewVPNKey)
    vpnGroup.DELETE("/api/keys/:id", handlers.RevokeVPNKeyAPI)
    vpnGroup.GET("/api/keys/:id/config", handlers.DownloadVPNConfig)
    vpnGroup.GET("/api/keys/:id/mobile", handlers.DownloadMobileConfig)

    vpnGroup.GET("/api/countries", handlers.GetVPNCountriesList)
    vpnGroup.GET("/api/global-stats", handlers.GetVPNGlobalStats)
    // Максимальное шифрование
    vpnGroup.GET("/api/max-security/:id/config", handlers.GetMaxSecurityConfig)
    vpnGroup.GET("/api/security-status", handlers.GetSecurityStatus)
    // Переключение между странами
    vpnGroup.POST("/api/switch-server/:id", handlers.SwitchServerForKey)
    vpnGroup.POST("/api/create-for-country", handlers.CreateKeyForCountry)
}

// ========== VPN API АЛИАСЫ (для совместимости с фронтендом) ==========
// Добавляем алиасы чтобы работал /api/vpn/stats (без /vpn в начале)
vpnAlias := r.Group("/api/vpn")
vpnAlias.Use(middleware.AuthMiddleware(cfg))
{
    vpnAlias.GET("/stats", handlers.GetVPNStatsAPI)
    vpnAlias.GET("/keys", handlers.GetVPNKeysAPI)
    vpnAlias.POST("/keys", handlers.CreateVPNKeyAPI)
    vpnAlias.DELETE("/keys/:id", handlers.RevokeVPNKeyAPI)
    vpnAlias.GET("/countries", handlers.GetVPNCountriesList)
    vpnAlias.GET("/global-stats", handlers.GetVPNGlobalStats)
}
    // ========== МИГРАЦИЯ (3 ФАЗЫ) ==========
    migrationAPI := r.Group("/api/migration")
    migrationAPI.Use(middleware.AuthMiddleware(cfg))
    {
        migrationAPI.POST("/project", handlers.CreateMigrationProject)
        migrationAPI.GET("/projects", handlers.GetMigrationProjects)
        migrationAPI.GET("/project/:id/status", handlers.GetMigrationStatus)
        migrationAPI.POST("/project/:id/phase2", handlers.StartPhase2)
        migrationAPI.POST("/project/:id/phase3", handlers.StartPhase3)
        migrationAPI.POST("/project/:id/sync", handlers.SyncEntities)

 // НОВЫЕ МАРШРУТЫ ДЛЯ УПРАВЛЕНИЯ МИГРАЦИЕЙ
    migrationAPI.POST("/project/:id/stop", handlers.StopMigration)
    migrationAPI.DELETE("/project/:id", handlers.DeleteMigrationProject)
    migrationAPI.POST("/project/:id/force-phase", handlers.ForcePhaseTransition)
    }

    // Страница миграции
    r.GET("/migration", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("migration"), func(c *gin.Context) {
        c.HTML(http.StatusOK, "migration.html", gin.H{
            "title": "Миграция данных 3 фазы | Business Stack",
        })
    })
    
    // ========== STEALTH VPN (НЕВИДИМЫЙ VPN) ==========
    // Stealth VPN API - не конфликтует с существующими VPN роутами
    stealthVPN := r.Group("/api/vpn/stealth")
    stealthVPN.Use(middleware.AuthMiddleware(cfg))
    {
        // Получить VLESS конфигурацию
        stealthVPN.GET("/config/vless", handlers.GetVLessConfigHandler)
        
        // Умный роутинг
        stealthVPN.GET("/routing", handlers.GetSmartRulesHandler)
        stealthVPN.POST("/routing", handlers.AddSmartRuleHandler)
        stealthVPN.DELETE("/routing/:id", handlers.DeleteSmartRuleHandler)
        
        // Получить stealth тарифы
        stealthVPN.GET("/plans", handlers.GetStealthPlansHandler)
    }
    
    // Страница Stealth VPN
    r.GET("/vpn/stealth", handlers.StealthVPNPageHandler)

    // Админ маршруты для VPN
    adminVPN := r.Group("/admin/vpn")
    adminVPN.Use(middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg))
    {
        adminVPN.GET("/keys", handlers.GetAllVPNKeys)
        adminVPN.GET("/stats", handlers.AdminVPNHandler)
    }

    r.POST("/api/service-order", serviceOrderHandler)

    // Страницы авторизации
    authPages := r.Group("/")
    {
        authPages.GET("/login", handlers.LoginPageHandler)
        authPages.GET("/register", handlers.RegisterPageHandler)
        authPages.GET("/forgot-password", handlers.ForgotPasswordPageHandler)

authPages.GET("/reset-password", func(c *gin.Context) {
    token := c.Query("token")
    if token == "" {
        c.HTML(http.StatusBadRequest, "error.html", gin.H{
            "title": "Ошибка",
            "error": "Неверная ссылка для сброса пароля",
        })
        return
    }
    c.HTML(http.StatusOK, "reset_password.html", gin.H{
        "title": "Сброс пароля | Business Stack",
        "token": token,
    })
})

  authPages.GET("/qr/confirm-reset", func(c *gin.Context) {
        token := c.Query("token")
        if token == "" {
            c.HTML(http.StatusBadRequest, "error.html", gin.H{
                "title": "Ошибка",
                "error": "Неверная ссылка подтверждения",
            })
            return
        }
        c.HTML(http.StatusOK, "qr_confirm_reset.html", gin.H{
            "title": "Подтверждение | Business Stack",
            "token": token,
        })
    })
    }

// ========== ПУБЛИЧНЫЙ МАРШРУТ: ПОЛУЧИТЬ ПОЛЬЗОВАТЕЛЯ ПО ТОКЕНУ ==========
r.GET("/api/auth/user-by-token", func(c *gin.Context) {
    token := c.Query("token")
    if token == "" {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Токен не указан",
        })
        return
    }

    var userID string
    var expiresAt time.Time
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT user_id, expires_at 
        FROM reset_tokens 
        WHERE token = $1 AND used = false
    `, token).Scan(&userID, &expiresAt)
    
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": "Неверный или истекший токен",
        })
        return
    }
    
    if time.Now().After(expiresAt) {
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": "Токен истек",
        })
        return
    }

    var user struct {
        ID    string `json:"id"`
        Email string `json:"email"`
        Name  string `json:"name"`
    }
    
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, email, COALESCE(name, '') as name 
        FROM users WHERE id = $1
    `, userID).Scan(&user.ID, &user.Email, &user.Name)
    
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Пользователь не найден",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "user": user,
    })
})

    // API авторизации
    authAPI := r.Group("/api/auth")
    authAPI.Use(func(c *gin.Context) {
        ip := c.ClientIP()
        path := c.Request.URL.Path
        
        // Исключаем QR генерацию и WebSocket из rate limiter
        if strings.Contains(path, "generate-reset") || 
           strings.Contains(path, "qr/reset-status") {
            c.Next()
            return
        }
        
        if authLimiter.Limit(ip) {
            c.JSON(http.StatusTooManyRequests, gin.H{
                "error": "Слишком много попыток входа. Попробуйте через минуту.",
            })
            c.Abort()
            return
        }
        c.Next()
    })
    {
        authAPI.POST("/register", handlers.RegisterHandler)
        authAPI.POST("/login", handlers.LoginHandler)
        authAPI.POST("/refresh", handlers.RefreshHandler)
        authAPI.POST("/logout", handlers.LogoutHandler)
        authAPI.POST("/trusted-devices/add", handlers.AddTrustedDevice)
        authAPI.POST("/trusted-devices/revoke", handlers.RevokeTrustedDevice)
        authAPI.GET("/trusted-devices/list", handlers.GetTrustedDevices)
        authAPI.POST("/login-by-id", handlers.LoginByIDHandler)
        authAPI.POST("/register-by-id", handlers.RegisterByIDHandler)

        // ========== ВОССТАНОВЛЕНИЕ ПАРОЛЯ ==========
        authAPI.POST("/forgot-password", handlers.ForgotPasswordHandler)
        authAPI.POST("/send-reset-code", handlers.SendResetCodeHandler)
        authAPI.POST("/verify-reset-code", handlers.VerifyResetCodeHandler)
        authAPI.POST("/reset-password", handlers.ResetPasswordHandler)
        authAPI.POST("/qr/generate-reset", handlers.GenerateResetQRHandler)
        authAPI.GET("/qr/reset-status", handlers.CheckResetQRStatusHandler)
        authAPI.GET("/qr/reset-check", handlers.CheckResetQRStatusHandler)  // для восстановления
        authAPI.POST("/qr/confirm-reset", handlers.ConfirmResetQRHandler)
        authAPI.GET("/current-user-for-qr", handlers.GetCurrentUserForQR)
        authAPI.GET("/2fa/profile-status", handlers.Check2FAProfileStatus)
        authAPI.POST("/2fa/verify-profile", handlers.Verify2FAProfile)
    }

    // Реферальная программа
    referralAPI := r.Group("/api/referral")
    referralAPI.Use(middleware.AuthMiddleware(cfg))
    {
        referralAPI.POST("/program/create", handlers.CreateReferralProgram)
        referralAPI.GET("/program", handlers.GetReferralProgram)
        referralAPI.GET("/commissions", handlers.GetReferralCommissions)
        referralAPI.POST("/commissions/pay", handlers.PayCommission)
    }
    r.GET("/ref", handlers.ProcessReferral)

    // Верификация
    verificationAPI := r.Group("/api/verification")
    {
        verificationAPI.POST("/send-email", handlers.SendVerificationEmail)
        verificationAPI.POST("/send-telegram", handlers.SendVerificationTelegram)
        verificationAPI.POST("/verify", handlers.VerifyCode)
        verificationAPI.GET("/status", handlers.CheckVerificationStatus)
    }

    // Защищенные маршруты
   protected := r.Group("/")
protected.Use(middleware.AuthMiddleware(cfg))
{
    protected.GET("/settings", handlers.SettingsHandler)
    protected.GET("/my-subscriptions", handlers.MySubscriptionsPageHandler)
    protected.GET("/trusted-devices", handlers.TrustedDevicesHandler)
    protected.GET("/monetization", handlers.MonetizationHandler)
    protected.GET("/calendar", handlers.CalendarHandler)
    protected.GET("/api/client/module-requests", handlers.GetMyModuleRequests)
}

r.GET("/profile", middleware.AuthMiddleware(cfg), func(c *gin.Context) {
    role := c.GetString("role")
    platformRole := c.GetString("platform_role")
    
    // Владелец платформы, админ или разработчик видят profile.html
    if platformRole == "owner" || platformRole == "admin" || role == "admin" || role == "developer" {
        c.HTML(200, "profile.html", gin.H{
            "title": "Панель управления | BusinessStack",
        })
    } else {
        c.HTML(200, "client_profile.html", gin.H{
            "title": "Мой кабинет | Business Stack",
        })
    }
})
// Клиентский профиль (для обычных пользователей)
r.GET("/client-profile", middleware.AuthMiddleware(cfg), func(c *gin.Context) {
    role := c.GetString("role")
    if role == "developer" || role == "admin" || role == "owner" {
        c.Redirect(302, "/profile") // разработчиков отправляем на их профиль
        return
    }
    c.HTML(200, "client_profile.html", gin.H{
        "title": "Мой кабинет | Business Stack",
    })
})

   // Админские маршруты с проверкой 2FA
adminGroup := r.Group("/")
adminGroup.Use(middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), middleware.Require2FA())
{
    adminGroup.GET("/admin", handlers.AdminDashboardHandler)
    adminGroup.GET("/admin/users", handlers.AdminUsersHandler)
    adminGroup.GET("/admin/subscriptions", handlers.AdminSubscriptionsHandler)
    adminGroup.GET("/admin-fixed", handlers.AdminFixedHandler)
    adminGroup.GET("/gold-admin", handlers.GoldAdminHandler)
    adminGroup.GET("/database-admin", handlers.DatabaseAdminHandler)
    adminGroup.GET("/users", handlers.UsersHandler)
    adminGroup.GET("/subscriptions", handlers.SubscriptionsHandler)
    r.GET("/crm", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("crm"), handlers.CRMHandler)
    adminGroup.GET("/admin/api-keys", handlers.AdminAPIKeysHandler)

    admin2FA := r.Group("/api/admin/2fa")
    admin2FA.Use(middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg))
    {
        admin2FA.POST("/enable", handlers.EnableAdmin2FA)
        admin2FA.POST("/verify", handlers.VerifyAdmin2FA)
    }
}

// ========== МОДУЛИ И ПОДПИСКИ ==========
// ========== ПЛАТФОРМА (ТОЛЬКО ДЛЯ ВЛАДЕЛЬЦА И ЕГО ПОМОЩНИКОВ) ==========
// Это твоя личная админка, куда клиенты НЕ ИМЕЮТ ДОСТУПА!
platformGroup := r.Group("/platform")
platformGroup.Use(middleware.AuthMiddleware(cfg), middleware.RequirePlatformAccess())
{
    // Управление всеми тенантами (организациями)
    platformGroup.GET("/tenants", handlers.GetTenants)  
    platformGroup.POST("/tenants", handlers.CreateTenant)
    platformGroup.PUT("/tenants/:id", handlers.UpdateTenant)
    platformGroup.DELETE("/tenants/:id", handlers.DeleteTenant)
    
    // Назначение ролей пользователям в тенантах
    platformGroup.POST("/tenants/:id/set-admin", handlers.SetTenantAdmin)
    platformGroup.POST("/tenants/:id/set-developer", handlers.SetTenantDeveloper)
    
   // Управление твоими помощниками (платформа админы/разработчики)
platformGroup.GET("/staff", handlers.GetPlatformStaffList)
platformGroup.POST("/staff", handlers.AddPlatformStaff)
platformGroup.POST("/staff/admin", handlers.AddPlatformAdmin)
platformGroup.POST("/staff/developer", handlers.AddPlatformDeveloper)
platformGroup.DELETE("/staff/:email", handlers.RemovePlatformStaffByEmail)


    // Глобальные настройки платформы
    platformGroup.GET("/settings", handlers.GetPlatformSettings)
    platformGroup.PUT("/settings", handlers.UpdatePlatformSettings)
    
    // Выдача прямого доступа к модулям (для клиентов)
    platformGroup.POST("/grant-access", handlers.GrantModuleAccess)
}

// ========== АДМИНКА ОРГАНИЗАЦИИ (ДЛЯ КЛИЕНТОВ) ==========
// Это админка, которую видят клиенты в своей организации
// Обрати внимание: используется RequireTenantAdmin, а не RequirePlatformAccess!
tenantAdminGroup := r.Group("/admin/tenant")
tenantAdminGroup.Use(middleware.AuthMiddleware(cfg), middleware.RequireTenantAdmin())
{
    tenantAdminGroup.GET("/", handlers.TenantAdminDashboard)
    tenantAdminGroup.GET("/users", handlers.TenantGetUsers)
    tenantAdminGroup.POST("/users", handlers.TenantCreateUser)
    tenantAdminGroup.PUT("/users/:id/role", handlers.TenantSetRole)
    tenantAdminGroup.DELETE("/users/:id", handlers.TenantDeleteUser)
    
    // Управление модулями внутри организации
    tenantAdminGroup.GET("/modules", handlers.TenantGetModules)
    tenantAdminGroup.POST("/modules/grant", handlers.TenantGrantModuleAccess)
}
modulesGroup := r.Group("/api/modules")
modulesGroup.Use(middleware.AuthMiddleware(cfg))
{
    modulesGroup.GET("", handlers.GetModules)
    modulesGroup.GET("/my-subscriptions", handlers.GetMyModuleSubscriptions)
    modulesGroup.POST("/start-trial", func(c *gin.Context) {
        var moduleName string
        
        // Пробуем получить из URL параметра ?module=xxx
        moduleName = c.Query("module")
        
        // Если нет - пробуем из JSON { "module_code": "xxx" }
        if moduleName == "" {
            var req struct {
                ModuleCode string `json:"module_code"`
            }
            if err := c.ShouldBindJSON(&req); err == nil && req.ModuleCode != "" {
                moduleName = req.ModuleCode
            }
        }
        
        // Если нет - пробуем из JSON { "module": "xxx" }
        if moduleName == "" {
            var req struct {
                Module string `json:"module"`
            }
            if err := c.ShouldBindJSON(&req); err == nil && req.Module != "" {
                moduleName = req.Module
            }
        }
        
        if moduleName == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "Missing parameters", "message": "Parameter 'module' or 'module_code' is required"})
            return
        }
        
        userID := c.GetString("user_id")
        if userID == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
            return
        }
        
        err := middleware.StartModuleTrial(userID, moduleName)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
        
        trialDays := middleware.GetModuleTrialDays(moduleName)
        c.JSON(http.StatusOK, gin.H{
            "success":     true,
            "message":     "Пробный период активирован!",
            "trial_days":  trialDays,
            "module":      moduleName,
        })
    })
    modulesGroup.GET("/check/:module", handlers.CheckModuleAccess)
}

// API для статусов модулей и обратной связи
devModulesGroup := r.Group("/api/dev-modules")
devModulesGroup.Use(middleware.AuthMiddleware(cfg))
{
    // Статусы модулей
    devModulesGroup.GET("/status/:route", handlers.GetModuleStatus)
    devModulesGroup.PUT("/status/:route", middleware.AdminMiddleware(cfg), handlers.UpdateModuleStatus)
    
    // Обратная связь
    devModulesGroup.POST("/feedback", handlers.ReportModuleIssue)
     devModulesGroup.POST("/report-issue", handlers.ReportModuleIssue) 
    devModulesGroup.GET("/feedback", middleware.AdminMiddleware(cfg), handlers.GetModuleFeedback)
    devModulesGroup.GET("/feedback/:id", middleware.AdminMiddleware(cfg), handlers.GetModuleFeedbackByID)
    devModulesGroup.PUT("/feedback/:id/status", middleware.AdminMiddleware(cfg), handlers.UpdateModuleRequestStatus)
    devModulesGroup.DELETE("/feedback/:id", middleware.AdminMiddleware(cfg), handlers.DeleteModuleFeedback)
    
    // Заявки пользователя
    devModulesGroup.GET("/my-requests", handlers.GetMyModuleRequests)
    
    // Комментарии к заявкам
    devModulesGroup.GET("/requests/:id/comments", handlers.GetRequestComments)
    devModulesGroup.POST("/requests/:id/comments", handlers.AddRequestComment)
    
    // Статистика (админ)
    devModulesGroup.GET("/stats", middleware.AdminMiddleware(cfg), handlers.GetModuleStatistics)
}
    // Дашборды
    dashboards := r.Group("/")
    dashboards.Use(middleware.AuthMiddleware(cfg))
    {
        dashboards.GET("/dashboard-improved", handlers.DashboardImprovedHandler)
        dashboards.GET("/realtime-dashboard", handlers.RealtimeDashboardHandler)
        dashboards.GET("/revenue-dashboard", handlers.RevenueDashboardHandler)
        dashboards.GET("/partner-dashboard", handlers.PartnerDashboardHandler)
        dashboards.GET("/unified-dashboard", handlers.UnifiedDashboardHandler)
        dashboards.GET("/dashboard-stats", handlers.DashboardStatsHandler)
    }

    // Платежи (публичные страницы, без авторизации)
    r.GET("/payment", handlers.PaymentHandler)
    r.GET("/bank_card_payment", handlers.BankCardPaymentHandler)
    r.GET("/payment-success", handlers.PaymentSuccessHandler)
    r.GET("/usdt-payment", handlers.USDTPaymentHandler)
    r.GET("/rub-payment", handlers.RUBPaymentHandler)

    // ========== ЛОГИСТИКА ==========
    // Страницы логистики (публичные или с авторизацией)
    logisticsGroup := r.Group("/logistics")
logisticsGroup.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("logistics"))
{
        logisticsGroup.GET("/", handlers.LogisticsDashboardHandler)
        logisticsGroup.GET("/orders", handlers.LogisticsOrdersHandler)
        logisticsGroup.GET("/track", handlers.TrackHandler)
    }
    
    // API логистики
    logisticsAPI := r.Group("/api/logistics")
    logisticsAPI.Use(middleware.AuthMiddleware(cfg))
    {
        logisticsAPI.POST("/orders", handlers.APICreateOrder)
        logisticsAPI.GET("/orders", handlers.APIGetOrders)
        logisticsAPI.PUT("/orders/:id/status", handlers.APIUpdateOrderStatus)
        logisticsAPI.GET("/stats", handlers.APIGetStats)
        logisticsAPI.GET("/track/:trackingNumber", handlers.TrackAPIHandler)
    }


    
    // Доставка (оставляем для обратной совместимости)
    deliveryAPI := r.Group("/api/delivery")
    deliveryAPI.Use(middleware.AuthMiddleware(cfg))
    {
        deliveryAPI.GET("/track/:trackingNumber", handlers.TrackAPIHandler)
    }

//---------------------------------------------------------------------
      // Основное API
api := r.Group("/api")

// Rate limiter middleware
api.Use(func(c *gin.Context) {
    path := c.Request.URL.Path
    
    if strings.Contains(path, "generate-reset") || 
       strings.Contains(path, "qr/reset-status") ||
       strings.Contains(path, "qr/status") {
        c.Next()
        return
    }
    
    ip := c.ClientIP()
    if rateLimiter.Limit(ip) {
        c.JSON(http.StatusTooManyRequests, gin.H{
            "error": "Слишком много запросов. Попробуйте позже.",
        })
        c.Abort()
        return
    }
    c.Next()
})

// Auth middleware
api.Use(middleware.AuthMiddleware(cfg))

// Получить название компании (для уведомлений)
api.GET("/company/name", handlers.GetCompanyNameFromContextHandler)

// ========== ФИНАНСОВЫЙ УЧЕТ ==========
api.GET("/journal-entries", handlers.GetJournalEntriesSimple)
api.GET("/journal-entries/:id", handlers.GetJournalEntry)
api.POST("/journal-entries", handlers.CreateJournalEntrySimple)
api.PUT("/journal-entries/:id", handlers.UpdateJournalEntry)
api.DELETE("/journal-entries/:id", handlers.DeleteJournalEntry)

// ========== АВТОМАТИЧЕСКАЯ ПРИВЯЗКА ТЕГОВ ==========
api.POST("/journal-entries/auto-tag", handlers.AutoAssignTagToEntry)

// ========== МАССОВЫЕ ОПЕРАЦИИ С ПРОВОДКАМИ ==========
api.POST("/journal-entries/mass-archive", handlers.MassMoveToArchive)
api.POST("/journal-entries/mass-delete", handlers.MassDeleteEntries)
api.POST("/journal-entries/mass-post", handlers.MassPostEntries)

api.GET("/chart-of-accounts", handlers.GetChartOfAccounts)
api.POST("/chart-of-accounts", handlers.CreateChartOfAccount)
api.PUT("/chart-of-accounts/:id", handlers.UpdateChartOfAccount)
api.DELETE("/chart-of-accounts/:id", handlers.DeleteChartOfAccount)

// ========== АРХИВ СЧЕТОВ ==========
api.GET("/chart-of-accounts/archived", handlers.GetArchivedChartOfAccounts)
api.DELETE("/chart-of-accounts/:id/archive", handlers.ArchiveChartOfAccount)
api.PUT("/chart-of-accounts/:id/restore", handlers.RestoreChartOfAccount)
api.DELETE("/chart-of-accounts/:id/permanent", handlers.PermanentDeleteChartOfAccount)

api.GET("/payments", handlers.GetFinancePayments)
api.POST("/payments", handlers.CreateFinancePayment)
api.PUT("/payments/:id", handlers.UpdatePayment)       
api.PUT("/payments/:id/status", handlers.UpdateFinancePaymentStatus)
api.DELETE("/payments/:id", handlers.DeletePayment)    

api.GET("/cash-operations", handlers.GetCashOperations)
api.POST("/cash-operations", handlers.CreateCashOperation)

    // ==========  ОТЧЁТОВ ==========
    api.GET("/reports/account-ledger", handlers.GetAccountLedger)
    api.GET("/reports/accounts-receivable", handlers.GetAccountsReceivable)
    api.GET("/reports/accounts-payable", handlers.GetAccountsPayable)
    api.GET("/reports/chessboard", handlers.GetChessboardReport)
    api.GET("/reports/purchase-ledger", handlers.GetPurchaseLedger)
    api.GET("/reports/sales-ledger", handlers.GetSalesLedger)

// ========== АРХИВ ЖУРНАЛА ПРОВОДОК ==========
journalArchive := r.Group("/api/journal/archive")
journalArchive.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("journal"))
{
    journalArchive.POST("/move/:id", handlers.MoveJournalToArchive)
    journalArchive.GET("/list", handlers.GetJournalArchiveList)
    journalArchive.POST("/restore/:id", handlers.RestoreJournalFromArchive)
    journalArchive.DELETE("/permanent/:id", handlers.PermanentDeleteJournalArchive)
    journalArchive.GET("/stats", handlers.GetJournalArchiveStats)
}
// ========== WEBHOOKS ДЛЯ РАЗРАБОТЧИКОВ ==========
api.GET("/webhooks", func(c *gin.Context) {
    userID := c.GetString("user_id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, url, events, secret, created_at FROM webhooks WHERE user_id = $1
    `, userID)
    if err != nil {
        c.JSON(200, []gin.H{})
        return
    }
    defer rows.Close()
    
    var webhooks []gin.H
    for rows.Next() {
        var id, url, secret string
        var events interface{}
        var createdAt time.Time
        
        rows.Scan(&id, &url, &events, &secret, &createdAt)
        webhooks = append(webhooks, gin.H{
            "id":         id,
            "url":        url,
            "events":     events,
            "secret":     secret,
            "created_at": createdAt,
        })
    }
    c.JSON(200, webhooks)
})

api.POST("/webhooks", func(c *gin.Context) {
    userID := c.GetString("user_id")
    var req struct {
        URL    string   `json:"url"`
        Events []string `json:"events"`
        Secret string   `json:"secret"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO webhooks (user_id, url, events, secret, created_at)
        VALUES ($1, $2, $3, $4, NOW())
    `, userID, req.URL, req.Events, req.Secret)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

api.DELETE("/webhooks/:id", func(c *gin.Context) {
    userID := c.GetString("user_id")
    id := c.Param("id")
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM webhooks WHERE id = $1 AND user_id = $2
    `, id, userID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})
    api.Use(middleware.AuthMiddleware(cfg))



  

// ========== АВАТАРКИ ==========
    api.POST("/avatar/upload", handlers.UploadAvatar)
    api.DELETE("/avatar", handlers.DeleteAvatar)
    api.GET("/avatar/:id", handlers.GetUserAvatar)
        api.GET("/notifications/settings", handlers.GetNotificationSettings)
        api.PUT("/notifications/settings", handlers.UpdateNotificationSettings)
        api.GET("/health", handlers.HealthHandler)
        api.GET("/crm/health", handlers.CRMHealthHandler)
api.GET("/user/role", func(c *gin.Context) {
    role := c.GetString("role")
    c.JSON(200, gin.H{"role": role})
})
        api.GET("/user/profile", handlers.GetUserProfile) 
        api.GET("/system/stats", handlers.SystemStatsHandler)
        api.GET("/test", handlers.TestHandler)
        api.POST("/user/profile", handlers.UpdateProfileHandler)
        api.POST("/user/password", handlers.UpdatePasswordHandler)
        api.POST("/user/save-filters", handlers.SaveUserFilters)
        api.GET("/user/load-filters", handlers.LoadUserFilters) 
        api.GET("/plans", handlers.GetPlansHandler)
        api.POST("/subscriptions", handlers.CreateSubscriptionHandler)
        api.POST("/ai/ask", handlers.AIAskHandler)
        api.POST("/ai/ask-with-file", handlers.AskWithFileHandler)
        api.GET("/user/subscriptions", handlers.GetUserSubscriptionsHandler)
        api.GET("/user/ai-usage", handlers.GetUserAIUsageHandler)
        api.POST("/telegram/ensure-key", handlers.EnsureAPIKeyForTelegram)
        api.POST("/webapp/auth", handlers.WebAppAuthHandler)
        api.POST("/chat/save", handlers.SaveChatMessage)
        api.GET("/chat/history", handlers.GetChatHistory)
        api.POST("/knowledge/upload", handlers.UploadKnowledgeHandler)
        api.GET("/knowledge/list", handlers.ListKnowledgeHandler)
        api.DELETE("/knowledge/delete/:id", handlers.DeleteKnowledgeHandler)
        api.POST("/notify", handlers.NotifyHandler)
        api.POST("/keys/create", handlers.CreateAPIKeyHandler)
        api.GET("/user/keys", handlers.GetUserAPIKeysHandler)
        api.POST("/keys/revoke", handlers.RevokeAPIKeyHandler)
        api.POST("/keys/validate", handlers.ValidateAPIKeyHandler)
        api.GET("/referral/stats", handlers.GetReferralStatsHandler)
        api.GET("/referral/friends", handlers.GetReferralFriendsHandler)
        api.GET("/2fa/status", handlers.GetTwoFAStatus)
        api.GET("/2fa/generate", handlers.GenerateTwoFASecret)
        api.POST("/2fa/verify", handlers.VerifyTwoFACode)
        api.POST("/2fa/disable", handlers.DisableTwoFA)
        api.GET("/2fa/backup-codes", handlers.GetBackupCodes)
        api.POST("/2fa/backup-codes", handlers.GenerateBackupCodes)
        api.GET("/2fa/settings", handlers.Get2FASettings)
        api.GET("/2fa/check-trust", handlers.CheckTrustedDevice)
        api.POST("/2fa/trust-device", handlers.TrustDevice)
         api.POST("/2fa/backup-codes/generate", handlers.GenerateBackupCodes)
        
        api.POST("/2fa/verify-backup", handlers.VerifyWithBackupCode)
        api.GET("/2fa/discount-status", handlers.GetTwoFADiscountStatus)
        api.POST("/2fa/apply-discount", handlers.ApplyTwoFADiscountToSubscription)
        api.GET("/crm/customers", handlers.GetCustomers)
        api.POST("/crm/customers", handlers.CreateCustomer)
        api.PUT("/crm/customers/:id", handlers.UpdateCustomer)
        api.DELETE("/crm/customers/:id", handlers.DeleteCustomer)
       // Партнёры для актов сверки
        api.GET("/crm/partners", handlers.GetPartners)
        api.POST("/crm/partners", handlers.CreatePartner)
        api.PUT("/crm/partners/:id", handlers.UpdatePartner)    
        api.DELETE("/crm/partners/:id", handlers.DeletePartner)
        api.GET("/crm/deals", handlers.GetDeals)
        api.POST("/crm/deals", handlers.CreateDeal)
        api.PUT("/crm/deals/:id", handlers.UpdateDeal)
        api.DELETE("/crm/deals/:id", handlers.DeleteDeal)
        api.PUT("/crm/deals/:id/stage", handlers.UpdateDealStage)
        api.GET("/crm/stats", handlers.GetCRMStats)
        api.POST("/crm/deals/:id/attachments", handlers.UploadDealAttachment)
        api.GET("/crm/deals/:id/attachments", handlers.GetDealAttachments)
        api.GET("/crm/attachments/:attachment_id/download", handlers.DownloadDealAttachment)
        api.DELETE("/crm/attachments/:attachment_id", handlers.DeleteDealAttachment)
        api.GET("/crm/advanced-stats", handlers.GetCRMAdvancedStats)
        api.POST("/crm/customers/batch/delete", handlers.BatchDeleteCustomers)
        api.PUT("/crm/customers/batch/status", handlers.BatchUpdateCustomersStatus)
        api.POST("/crm/deals/batch/delete", handlers.BatchDeleteDeals)
        api.PUT("/crm/deals/batch/stage", handlers.BatchUpdateDealsStage)
        api.PUT("/crm/deals/batch/responsible", handlers.BatchUpdateDealsResponsible)
        api.GET("/crm/customers/export/csv", handlers.ExportCustomersCSV)
        api.GET("/crm/customers/export/excel", handlers.ExportCustomersExcel)
        api.GET("/crm/deals/export/csv", handlers.ExportDealsCSV)
        api.GET("/crm/deals/export/excel", handlers.ExportDealsExcel)
        api.GET("/crm/history/:type/:id", handlers.GetEntityHistory)
        api.GET("/crm/tags", handlers.GetTags)
        api.POST("/crm/tags", handlers.CreateTag)
        api.DELETE("/crm/tags/:id", handlers.DeleteTag)
        api.POST("/crm/activities", handlers.AddActivity)
        api.GET("/crm/activities/:type/:id", handlers.GetActivities)
        api.POST("/crm/ai/ask", handlers.AIAskHandler)
        api.POST("/transcription/upload", handlers.UploadAudio)
        api.GET("/transcriptions", handlers.GetTranscriptions)
        api.GET("/transcription/:id", handlers.GetTranscriptionByID)
        api.GET("/crm/forecast", handlers.GetSalesForecast)
        api.GET("/crm/conversion", handlers.GetStageConversion)
        api.DELETE("/crm/activities/:id", handlers.DeleteActivity)
        api.PUT("/crm/tags/:id", handlers.UpdateTag)
        api.POST("/ai/consultant", handlers.AIConsultantHandler)
        api.GET("/analytics/ltv", handlers.GetLTVPredictions)
        api.GET("/analytics/ltv/:id", handlers.GetCustomerLTV)
        api.GET("/analytics/insights", handlers.GetInsights)
        api.GET("/analytics/segments", handlers.GetSegmentSummary)
        api.GET("/analytics/cohorts/run", handlers.RunCohortAnalysis)

// ========== БЕЗОПАСНОСТЬ - СЕССИИ ==========
api.GET("/sessions", handlers.GetUserSessions)
api.DELETE("/sessions/:id", handlers.TerminateSession)
api.DELETE("/sessions/all", handlers.TerminateAllSessions)

// ========== БЕЗОПАСНОСТЬ - ИСТОРИЯ ВХОДОВ ==========
api.GET("/login-history", handlers.GetLoginHistory)

// ========== БЕЗОПАСНОСТЬ - НАСТРОЙКИ ПОЛЬЗОВАТЕЛЯ ==========
api.GET("/user/settings", handlers.GetUserSettings)
api.POST("/sessions/limit", handlers.SetMaxSessions)
    // Экспорт аналитики
    api.GET("/analytics/export/csv", handlers.ExportAnalyticsCSV)
    api.GET("/analytics/export/excel", handlers.ExportAnalyticsExcel)
    api.GET("/analytics/export/pdf", handlers.ExportAnalyticsPDF)
    api.GET("/analytics/data", handlers.GetAnalyticsData)

    // Отмена подписки
    api.POST("/subscriptions/:id/cancel", func(c *gin.Context) {
        id := c.Param("id")
        userID := c.GetString("user_id")
        
        log.Printf("📝 Отмена подписки: id=%s, userID=%s", id, userID)
        
        _, err := database.Pool.Exec(c.Request.Context(), 
            "UPDATE subscriptions SET status = 'canceled' WHERE id = $1 AND user_id = $2",
            id, userID)
        
        if err != nil {
            log.Printf("❌ Ошибка: %v", err)
            c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка отмены подписки"})
            return
        }
        
        c.JSON(http.StatusOK, gin.H{"success": true, "message": "Подписка отменена"})
    })

 // ========== ТРИАЛЬНЫЙ ПЕРИОД ==========
    api.POST("/trial/start", func(c *gin.Context) {
        moduleName := c.Query("module")
        userID := c.GetString("user_id")
        
        if moduleName == "" {
            c.JSON(400, gin.H{"error": "module parameter required"})
            return
        }
        
        err := middleware.StartModuleTrial(userID, moduleName)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }
        
        c.JSON(200, gin.H{
            "success":     true,
            "message":     "14-дневный пробный период активирован!",
            "trial_days":  14,
        })
    })


    // ========== ТРИАЛЬНЫЙ ПЕРИОД ДЛЯ РАЗРАБОТЧИКОВ ==========
    api.POST("/developer/trial/start", func(c *gin.Context) {
        userID := c.GetString("user_id")
        
        if userID == "" {
            c.JSON(401, gin.H{"error": "unauthorized"})
            return
        }
        
        err := middleware.StartDeveloperTrial(userID)
        if err != nil {
            c.JSON(500, gin.H{"error": err.Error()})
            return
        }

   // ПОВЫШАЕМ РОЛЬ ДО DEVELOPER
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE users SET role = 'developer' WHERE id = $1 AND role = 'user'
    `, userID)
    
    if err != nil {
        fmt.Printf("⚠️ Ошибка повышения роли: %v\n", err)
    }
        
        c.JSON(200, gin.H{
            "success":     true,
            "message":     "14-дневный пробный период разработчика активирован!",
            "trial_days":  14,
        })
    })

api.GET("/developer/stats", func(c *gin.Context) {
    userID := c.GetString("user_id")
    
    var totalRequests, totalUsers, totalApps int64
    
    // Количество запросов к API
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM api_usage WHERE user_id = $1
    `, userID).Scan(&totalRequests)
    
    // ✅ ИСПРАВЛЕНО: Считаем ВСЕХ пользователей из таблицы users
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM users WHERE deleted_at IS NULL
    `).Scan(&totalUsers)
    if err != nil {
        log.Printf("❌ Ошибка подсчета пользователей: %v", err)
        totalUsers = 0
    }
    log.Printf("📊 totalUsers = %d", totalUsers)
    
    // Количество приложений разработчика
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM oauth_clients 
        WHERE client_id IN (SELECT client_id FROM oauth_authorizations WHERE user_id = $1)
    `, userID).Scan(&totalApps)
    
    c.JSON(200, gin.H{
        "total_requests": totalRequests,
        "total_users":     totalUsers,
        "total_apps":      totalApps,
        "labels":         []string{"Янв", "Фев", "Мар", "Апр", "Май", "Июн"},
        "values":         []int64{65, 59, 80, 81, 56, 55},
    })
})
// ========== WEBHOOKS ==========
api.GET("/api/webhooks", func(c *gin.Context) {
    userID := c.GetString("user_id")
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, url, events, secret, created_at FROM webhooks WHERE user_id = $1
    `, userID)
    if err != nil {
        c.JSON(200, []gin.H{})
        return
    }
    defer rows.Close()
    
    var webhooks []gin.H
    for rows.Next() {
        var id, url, secret string
        var events interface{}
        var createdAt time.Time
        
        rows.Scan(&id, &url, &events, &secret, &createdAt)
        webhooks = append(webhooks, gin.H{
            "id":         id,
            "url":        url,
            "events":     events,
            "secret":     secret,
            "created_at": createdAt,
        })
    }
    c.JSON(200, webhooks)
})

api.POST("/api/webhooks", func(c *gin.Context) {
    userID := c.GetString("user_id")
    var req struct {
        URL    string   `json:"url"`
        Events []string `json:"events"`
        Secret string   `json:"secret"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO webhooks (user_id, url, events, secret, created_at)
        VALUES ($1, $2, $3, $4, NOW())
    `, userID, req.URL, req.Events, req.Secret)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

api.DELETE("/api/webhooks/:id", func(c *gin.Context) {
    userID := c.GetString("user_id")
    id := c.Param("id")
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM webhooks WHERE id = $1 AND user_id = $2
    `, id, userID)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

    // ========== API KEYS MANAGEMENT ==========
    apiKeysGroup := r.Group("/api/keys")
    apiKeysGroup.Use(middleware.AuthMiddleware(cfg))
    {
        apiKeysGroup.POST("/generate", handlers.GenerateAPIKey)
        apiKeysGroup.GET("", handlers.GetAPIKeys)
        apiKeysGroup.DELETE("/:id", handlers.RevokeAPIKey)
        apiKeysGroup.GET("/:id/stats", handlers.GetAPIKeyStats)
        apiKeysGroup.GET("/:id/daily-stats", handlers.GetAPIKeyDailyStats)
    }
    
       secureAPI := r.Group("/secure-api")
      secureAPI.Use(middleware.AuthMiddleware(cfg))
    {
        secureAPI.GET("/user/profile", handlers.GetUserProfile)
        secureAPI.GET("/user/ai-history", handlers.GetUserAIHistoryHandler)

        // ========== NEBULA CLOUD - ОБЛАЧНОЕ ХРАНИЛИЩЕ ==========
        cloudAPI := r.Group("/api/cloud")
        cloudAPI.Use(middleware.AuthMiddleware(cfg))
        {
            cloudAPI.GET("/files", handlers.GetCloudFiles)
            cloudAPI.POST("/upload", handlers.UploadCloudFile)
            cloudAPI.DELETE("/files/:id", handlers.DeleteCloudFile)
            cloudAPI.GET("/files/:id/download", handlers.DownloadCloudFile)  
            cloudAPI.POST("/files/:id/star", handlers.ToggleStarFile)        
            cloudAPI.POST("/folder", handlers.CreateCloudFolder)
            cloudAPI.GET("/stats", handlers.GetCloudStats)
            cloudAPI.GET("/plans", handlers.GetCloudPlans)
            cloudAPI.POST("/create", handlers.CreateCloudBucket) 
            cloudAPI.POST("/upgrade", handlers.UpgradeCloudPlan)  


        }

        // Страница облачного хранилища
       r.GET("/cloud", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("cloud"), handlers.NebulaCloudPage)
    }

    // ========== FUSIONAPI - Брендовый API продукт с AI ==========
    fusionAPI := r.Group("/api/fusion")
    fusionAPI.Use(middleware.AuthMiddleware(cfg))
    {
        // API ключи
        fusionAPI.GET("/my-key", handlers.GetMyAPIKey)
        fusionAPI.GET("/usage-stats", handlers.GetAPIUsageStats)
        fusionAPI.POST("/regenerate-key", handlers.RegenerateAPIKey)
        fusionAPI.GET("/plans", handlers.GetAPIPlans)
        fusionAPI.POST("/upgrade-plan", handlers.APIPlanUpgradeRequest)
        fusionAPI.GET("/docs", handlers.GetAPIDocumentation)
        
        // AI Агенты (новые функции для FusionAPI)
        fusionAPI.GET("/agents", handlers.GetMyAgents)
        fusionAPI.POST("/agents", handlers.CreateFusionAgent)
        fusionAPI.PUT("/agents/:id", handlers.UpdateFusionAgent)
        fusionAPI.DELETE("/agents/:id", handlers.DeleteFusionAgent)
        fusionAPI.POST("/agents/:id/chat", handlers.ChatWithFusionAgent)
        
        // AI Аналитика
        fusionAPI.GET("/analytics/ai", handlers.GetFusionAIAnalytics)
    }
    
    // Страница портала FusionAPI
    r.GET("/fusion-portal", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fusion-api"), handlers.FusionAPIPortalHandler)

    // ========== AI AGENTS MANAGEMENT ==========
    aiAgents := r.Group("/api/ai/agents")
    aiAgents.Use(middleware.AuthMiddleware(cfg))
    {
        aiAgents.GET("", handlers.GetAgents)
        aiAgents.POST("", handlers.CreateAgent)
        aiAgents.GET("/:id", handlers.GetAgentDetails)
        aiAgents.PUT("/:id", handlers.UpdateAgent)
        aiAgents.DELETE("/:id", handlers.DeleteAgent)
        aiAgents.POST("/:id/clone", handlers.CloneAgent)
        aiAgents.POST("/:id/toggle", handlers.ToggleAgentStatus)
        aiAgents.POST("/:id/actions", handlers.AddAgentAction)
        aiAgents.GET("/logs", handlers.GetAgentLogs)
        aiAgents.GET("/stats", handlers.GetAgentStats)
        aiAgents.GET("/export", handlers.ExportAgents)
    }
    
    r.GET("/notify", handlers.NotifyPageHandler)

    userKeys := r.Group("/api/user/keys")
    userKeys.Use(middleware.AuthMiddleware(cfg))
    {
        userKeys.DELETE("/:id", handlers.RevokeAPIKeyHandler)
    }

    // Публичное API с защитой через API ключи
    v1 := r.Group("/api/v1")
   v1.Use(middleware.AuthMiddleware(cfg))

   // ========== API ИНТЕГРАЦИИ ДЛЯ ВНЕШНИХ СЕРВИСОВ ==========
    v1.POST("/auth/verify", handlers.VerifyTokenHandler)
    v1.GET("/user/info", handlers.GetUserInfoByToken)
    {
        v1.GET("/health", handlers.HealthHandler)
        v1.POST("/ai/ask", handlers.AIAskHandler)
        v1.POST("/ai/consultant", handlers.AIConsultantHandler)
        v1.GET("/crm/customers", handlers.GetCustomers)
        v1.GET("/crm/deals", handlers.GetDeals)
        v1.GET("/vpn/status", handlers.GetVPNStats)
        v1.GET("/vpn/plans", handlers.GetStealthPlansHandler)
    }

    adminAPI := r.Group("/api/admin")
    adminAPI.Use(middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg))
    {
        adminAPI.PUT("/subscriptions/:id/cancel", handlers.AdminCancelSubscriptionHandler)
        adminAPI.PUT("/subscriptions/:id/reactivate", handlers.AdminReactivateSubscriptionHandler)
        adminAPI.GET("/plans", handlers.AdminGetPlansHandler)
        adminAPI.POST("/plans", handlers.AdminCreatePlanHandler)
        adminAPI.PUT("/plans/:id", handlers.AdminUpdatePlanHandler)
        adminAPI.DELETE("/plans/:id", handlers.AdminDeletePlanHandler)
        adminAPI.PUT("/api-keys/:id", handlers.AdminUpdateAPIKeyHandler)
        adminAPI.DELETE("/api-keys/:id", handlers.AdminDeleteAPIKeyHandler)
        adminAPI.GET("/stats", handlers.AdminStatsHandler)
        adminAPI.GET("/users", handlers.AdminUsersHandler)
        adminAPI.PUT("/users/:id/block", handlers.AdminToggleUserBlockHandler)
        adminAPI.GET("/payments", handlers.AdminPaymentsHandler)
        adminAPI.GET("/payment-stats", handlers.AdminPaymentStats)
        adminAPI.GET("/payments/recent", handlers.GetRecentPayments)
        adminAPI.GET("/security-logs", handlers.AdminSecurityLogs)
        adminAPI.GET("/blocked-ips", handlers.AdminBlockedIPs)
        adminAPI.POST("/users/toggle-block", handlers.AdminToggleUserBlock)
        adminAPI.POST("/users/change-role", handlers.AdminChangeUserRole)
        adminAPI.POST("/users/delete", handlers.AdminDeleteUser)
        adminAPI.GET("/tenants", handlers.GetTenants)
        adminAPI.POST("/tenants", handlers.CreateTenant)
        adminAPI.PUT("/tenants/:id", handlers.UpdateTenant)
        adminAPI.DELETE("/tenants/:id", handlers.DeleteTenant)
        adminAPI.POST("/tenants/:id/switch", handlers.SwitchTenant)
        

 // Админские API для выплат
    adminAPI.GET("/payouts", handlers.AdminGetPayouts)
    adminAPI.POST("/payouts/update", handlers.AdminUpdatePayoutStatus)
    adminAPI.DELETE("/payouts/:id", handlers.AdminDeletePayout)

    // ========== УПРАВЛЕНИЕ ПОЛЬЗОВАТЕЛЯМИ (CRUD) ==========
    adminAPI.GET("/users/all", handlers.GetAllUsers)
    adminAPI.POST("/users/create", handlers.CreateUser)
    adminAPI.PUT("/users/:id", handlers.UpdateUser)
    adminAPI.DELETE("/users/:id", handlers.DeleteUser)
    adminAPI.POST("/users/:id/block", handlers.BlockUser)
    adminAPI.POST("/users/:id/unblock", handlers.UnblockUser)
    adminAPI.PUT("/users/:id/role", handlers.ChangeUserRole)
    // ========== УПРАВЛЕНИЕ РОЛЯМИ ==========
    adminAPI.GET("/roles", handlers.GetRoles)
    adminAPI.POST("/roles", handlers.CreateRole)
    adminAPI.POST("/users/:id/assign-role", handlers.AssignRole)
    adminAPI.GET("/users/:id/roles", handlers.GetUserRoles)
    }

    // Админская страница для управления компаниями (отдельно)
    adminTenants := r.Group("/admin/tenants")
    adminTenants.Use(middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg))
    {
        adminTenants.GET("/", handlers.TenantAdminPage)
    }

// ========== ЗАГРУЗКА ДОКУМЕНТАЦИИ ДЛЯ AI (RAG) ==========
r.POST("/api/admin/load-knowledge", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    kb := services.NewKnowledgeBase(database.Pool)
    count, err := kb.LoadDirectory("./knowledge_base")
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{
        "success": true,
        "message": fmt.Sprintf("Загружено %d документов", count),
    })
})
    
    // API Documentation with back button
    r.GET("/api-docs", func(c *gin.Context) {
        c.HTML(http.StatusOK, "api_with_back.html", gin.H{
            "title": "API Documentation - TeamSphere",
        })
    })

    // Original Swagger (без кнопки)
    r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

    // Обработка запросов Chrome DevTools
    r.GET("/.well-known/appspecific/com.chrome.devtools.json", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{
            "app-specific": true,
        })
    })

// Страница архива журнала проводок
r.GET("/journal-archive", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("journal"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "journal_archive.html", gin.H{
        "title": "Архив журнала проводок | FinCore",
    })
})

// ========== API ЗАЯВОК С МУЛЬТИТЕНАНТНОСТЬЮ ==========
r.GET("/api/orders/list", func(c *gin.Context) {
    userEmail := c.GetString("user_email")
    role := c.GetString("role")
    platformRole := c.GetString("platform_role")
    
    // ✅ ПРОВЕРКА НА ВЛАДЕЛЬЦА/АДМИНА
    isOwner := userEmail == "dev@businessstack.ru" || 
               platformRole == "owner" || 
               role == "admin" || 
               role == "platform_owner"
    
    var rows pgx.Rows
    var err error
    
    if isOwner {
        // ✅ Владелец видит ВСЕ заявки (из обеих таблиц)
        log.Printf("👑 Владелец просматривает ВСЕ заявки")
        rows, err = database.Pool.Query(c.Request.Context(), `
            SELECT 
                id, 
                'service' as source,
                COALESCE(client_name, '') as client_name,
                COALESCE(client_contact, '') as client_contact,
                COALESCE(service_type, '') as service_type,
                COALESCE(deadline, '') as deadline,
                COALESCE(NULLIF(budget, ''), '0') as budget,
                COALESCE(status, 'new') as status,
                created_at,
                COALESCE(deposit_status, 'not_paid') as deposit_status,
                COALESCE(deposit_amount, 0) as deposit_amount,
                COALESCE(remaining_amount, 0) as remaining_amount,
                COALESCE(remaining_status, 'not_paid') as remaining_status,
                COALESCE(work_status, 'waiting_deposit') as work_status,
                COALESCE(tenant_id::text, '') as tenant_id,
                '' as description
            FROM service_orders 
            
            UNION ALL
            
            SELECT 
                id, 
                'individual' as source,
                COALESCE(name, '') as client_name,
                COALESCE(phone, '') as client_contact,
                'Индивидуальный заказ' as service_type,
                '' as deadline,
                COALESCE(budget, 0)::text as budget,
                COALESCE(status, 'new') as status,
                created_at,
                'not_paid' as deposit_status,
                0 as deposit_amount,
                0 as remaining_amount,
                'not_paid' as remaining_status,
                'waiting_deposit' as work_status,
                '' as tenant_id,
                COALESCE(description, '') as description
            FROM individual_orders
            
            ORDER BY created_at DESC LIMIT 100
        `)
    } else {
        // ✅ Обычный пользователь - только свои заявки
        tenantID := c.GetString("tenant_id")
        if tenantID == "" {
            tenantID = c.GetString("token_tenant_id")
        }
        if tenantID == "" {
            userID := c.GetString("user_id")
            if userID != "" {
                database.Pool.QueryRow(c.Request.Context(), 
                    "SELECT tenant_id FROM users WHERE id = $1", userID).Scan(&tenantID)
            }
        }
        log.Printf("👤 Пользователь просматривает заявки tenant: %s", tenantID)
        rows, err = database.Pool.Query(c.Request.Context(), `
            SELECT 
                id, 
                'service' as source,
                COALESCE(client_name, '') as client_name,
                COALESCE(client_contact, '') as client_contact,
                COALESCE(service_type, '') as service_type,
                COALESCE(deadline, '') as deadline,
                COALESCE(NULLIF(budget, ''), '0') as budget,
                COALESCE(status, 'new') as status,
                created_at,
                COALESCE(deposit_status, 'not_paid') as deposit_status,
                COALESCE(deposit_amount, 0) as deposit_amount,
                COALESCE(remaining_amount, 0) as remaining_amount,
                COALESCE(remaining_status, 'not_paid') as remaining_status,
                COALESCE(work_status, 'waiting_deposit') as work_status,
                COALESCE(tenant_id::text, '') as tenant_id,
                '' as description
            FROM service_orders 
            WHERE tenant_id = $1 
            
            UNION ALL
            
            SELECT 
                id, 
                'individual' as source,
                COALESCE(name, '') as client_name,
                COALESCE(phone, '') as client_contact,
                'Индивидуальный заказ' as service_type,
                '' as deadline,
                COALESCE(budget, 0)::text as budget,
                COALESCE(status, 'new') as status,
                created_at,
                'not_paid' as deposit_status,
                0 as deposit_amount,
                0 as remaining_amount,
                'not_paid' as remaining_status,
                'waiting_deposit' as work_status,
                '' as tenant_id,
                COALESCE(description, '') as description
            FROM individual_orders
            WHERE phone LIKE $2 OR name LIKE $2
            
            ORDER BY created_at DESC LIMIT 50
        `, tenantID, "%"+userEmail+"%")
    }
    
    if err != nil {
        log.Printf("❌ Ошибка получения заявок: %v", err)
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var orders []gin.H
    for rows.Next() {
        var id int
        var source, name, contact, service, deadline, budgetStr, status, depositStatus, remainingStatus, workStatus, tenantIDFromDB, description string
        var depositAmount, remainingAmount float64
        var createdAt time.Time
        
        err := rows.Scan(&id, &source, &name, &contact, &service, &deadline, &budgetStr, &status, &createdAt,
            &depositStatus, &depositAmount, &remainingAmount, &remainingStatus, &workStatus, &tenantIDFromDB, &description)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования заявки: %v", err)
            continue
        }
        
        var budget float64
        fmt.Sscanf(budgetStr, "%f", &budget)
        
        // Определяем иконку в зависимости от источника
        sourceIcon := "📋"
        if source == "individual" {
            sourceIcon = "👤"
        }
        
        orders = append(orders, gin.H{
            "id": id, 
            "source": source,
            "source_icon": sourceIcon,
            "name": name, 
            "contact": contact, 
            "service": service,
            "deadline": deadline, 
            "budget": budget,
            "status": status, 
            "date": createdAt.Format("02.01.2006 15:04"),
            "description": description,
            "tenant_id": tenantIDFromDB,
            "deposit_status": depositStatus,
            "deposit_status_text": map[string]string{
                "not_paid": "⏳ Ожидает предоплату 50%",
                "paid": "✅ Предоплата внесена",
            }[depositStatus],
            "deposit_amount": depositAmount,
            "remaining_amount": remainingAmount,
            "remaining_status": remainingStatus,
            "remaining_status_text": map[string]string{
                "not_paid": "⏳ Ожидает остаток",
                "paid": "✅ Остаток оплачен",
            }[remainingStatus],
            "work_status": workStatus,
            "work_status_text": map[string]string{
                "waiting_deposit": "⏳ Ожидает предоплату",
                "in_progress": "🔧 В работе",
                "waiting_remaining": "⏳ Ожидает остаток",
                "completed": "🎉 Завершён",
                "cancelled": "❌ Отменён",
            }[workStatus],
        })
    }
    
    log.Printf("📋 Возвращено %d заявок (включая индивидуальные)", len(orders))
    c.JSON(200, orders)
})
// ========== API ДЛЯ ДОРАБОТОК/ФИЧРЕКВЕСТОВ С МУЛЬТИТЕНАНТНОСТЬЮ ==========
// Создать заявку на доработку (для всех авторизованных пользователей)
r.POST("/api/feature-request", middleware.AuthMiddleware(cfg), func(c *gin.Context) {
    // ✅ Берем ИЗ КОНТЕКСТА
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    userEmail := c.GetString("user_email")
    
    // ✅ Пробуем получить tenant_id из контекста
    tenantID := c.GetString("tenant_id")
    
    // ✅ ЕСЛИ tenant_id пустой - получаем из БД
    if tenantID == "" || tenantID == "null" {
        var dbTenantID string
        err := database.Pool.QueryRow(c.Request.Context(), 
            "SELECT tenant_id FROM users WHERE id = $1", userID).Scan(&dbTenantID)
        if err == nil && dbTenantID != "" && dbTenantID != "null" {
            tenantID = dbTenantID
            // Сохраняем в контекст для дальнейшего использования
            c.Set("tenant_id", tenantID)
            log.Printf("[AUTH] ✅ Tenant ID получен из БД для %s: %s", userEmail, tenantID)
        } else {
            log.Printf("❌ Ошибка: tenant_id не найден для пользователя %s (userID=%s)", userEmail, userID)
            c.JSON(http.StatusUnauthorized, gin.H{
                "error": "Ошибка авторизации: tenant_id не найден",
                "code":  "TENANT_NOT_FOUND",
            })
            return
        }
    }
    
    if userID == "" {
        log.Printf("❌ Ошибка: user_id отсутствует в контексте для пользователя %s", userEmail)
        c.JSON(http.StatusUnauthorized, gin.H{
            "error": "Ошибка авторизации: user_id не найден",
        })
        return
    }
    
    log.Printf("📝 Создание заявки: userID=%s, tenantID=%s", userID, tenantID)
    
    var req struct {
        Title       string `json:"title"`
        Description string `json:"description"`
        Priority    string `json:"priority"`
    }
    
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "Неверный формат данных: " + err.Error()})
        return
    }
    
    // Валидация
    if req.Title == "" {
        c.JSON(400, gin.H{"error": "Заголовок обязателен"})
        return
    }
    if req.Description == "" {
        c.JSON(400, gin.H{"error": "Описание обязательно"})
        return
    }
    
    if req.Priority == "" {
        req.Priority = "medium"
    }
    
    // ✅ Используем tenant_id ИЗ КОНТЕКСТА ИЛИ БД
    _, err := database.Pool.Exec(c.Request.Context(), 
        `INSERT INTO feature_requests 
         (user_id, user_name, user_email, title, description, priority, status, tenant_id, created_at) 
         VALUES ($1, $2, $3, $4, $5, $6, 'new', $7, NOW())`,
        userID, userName, userEmail, req.Title, req.Description, req.Priority, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка сохранения: %v", err)
        c.JSON(500, gin.H{"error": "Ошибка сохранения заявки: " + err.Error()})
        return
    }
    
    log.Printf("✅ Заявка создана: %s от %s (user_id=%s, tenant_id=%s)", 
        req.Title, userEmail, userID, tenantID)
    
    c.JSON(200, gin.H{
        "success": true,
        "message": "Заявка на доработку отправлена",
        "tenant_id": tenantID,
    })
})
// GET /api/feature-requests - получить идеи
r.GET("/api/feature-requests", middleware.AuthMiddleware(cfg), func(c *gin.Context) {
    userID := c.GetString("user_id")
    userEmail := c.GetString("user_email")
    role := c.GetString("role")
    platformRole := c.GetString("platform_role")
    tenantID := c.GetString("tenant_id")
    
    // Если tenant_id пустой - получаем из БД
    if tenantID == "" {
        var dbTenantID string
        err := database.Pool.QueryRow(c.Request.Context(), 
            "SELECT tenant_id FROM users WHERE id = $1", userID).Scan(&dbTenantID)
        if err == nil && dbTenantID != "" {
            tenantID = dbTenantID
            c.Set("tenant_id", tenantID)
        }
    }
    
    log.Printf("🔍 Запрос идей: userID=%s, userEmail=%s, tenantID=%s, role=%s", 
        userID, userEmail, tenantID, role)
    
    // Проверяем, является ли пользователь владельцем/админом
    isOwner := userEmail == "dev@businessstack.ru" || 
               platformRole == "owner" || 
               role == "admin" || 
               role == "platform_owner"
    
    var rows pgx.Rows
    var err error
    
    if isOwner {
        // Владелец видит ВСЕ идеи
        log.Printf("👑 Владелец просматривает ВСЕ идеи")
        rows, err = database.Pool.Query(c.Request.Context(), 
            `SELECT id, COALESCE(user_name, '') as user_name, 
                    COALESCE(user_email, '') as user_email, 
                    COALESCE(title, '') as title, 
                    COALESCE(description, '') as description, 
                    COALESCE(priority, 'medium') as priority, 
                    COALESCE(status, 'new') as status, 
                    created_at 
             FROM feature_requests 
             ORDER BY created_at DESC`)
    } else {
        // Обычный пользователь - только свои идеи (по tenant_id ИЛИ по email)
        log.Printf("👤 Пользователь %s просматривает идеи", userEmail)
        rows, err = database.Pool.Query(c.Request.Context(), 
            `SELECT id, COALESCE(user_name, '') as user_name, 
                    COALESCE(user_email, '') as user_email, 
                    COALESCE(title, '') as title, 
                    COALESCE(description, '') as description, 
                    COALESCE(priority, 'medium') as priority, 
                    COALESCE(status, 'new') as status, 
                    created_at 
             FROM feature_requests 
             WHERE tenant_id = $1 OR user_email = $2 OR user_id = $3
             ORDER BY created_at DESC`, tenantID, userEmail, userID)
    }
    
    if err != nil {
        log.Printf("❌ Ошибка получения идей: %v", err)
        c.JSON(200, []gin.H{})
        return
    }
    defer rows.Close()
    
    var requests []gin.H
    for rows.Next() {
        var id int
        var userName, userEmail, title, description, priority, status string
        var createdAt time.Time
        
        err := rows.Scan(&id, &userName, &userEmail, &title, &description, &priority, &status, &createdAt)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования идеи: %v", err)
            continue
        }
        
        requests = append(requests, gin.H{
            "id":          id,
            "user_name":   userName,
            "user_email":  userEmail,
            "title":       title,
            "description": description,
            "priority":    priority,
            "status":      status,
            "date":        createdAt.Format("02.01.2006 15:04"),
            "created_at":  createdAt,
        })
    }
    
    if requests == nil {
        requests = []gin.H{}
    }
    
    log.Printf("📋 Найдено идей: %d", len(requests))
    c.JSON(200, requests)
})
// Обновить статус доработки (только для админов)
r.PUT("/api/feature-requests/:id/status", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    var req struct{ Status string `json:"status"` }
    c.BindJSON(&req)
    _, err := database.Pool.Exec(c.Request.Context(), 
        "UPDATE feature_requests SET status = $1, updated_at = NOW() WHERE id = $2", req.Status, id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Остальные маршруты без изменений...
// (здесь продолжается ваш код с r.PUT, r.DELETE и т.д.)

// Отметить заявку как просмотренную
r.PUT("/api/orders/:id/view", func(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(), 
        "UPDATE service_orders SET status = 'viewed', viewed_at = NOW() WHERE id = $1", id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Обновить заявку
r.PUT("/api/orders/:id/update", func(c *gin.Context) {
    id := c.Param("id")
    var data struct {
        Name     string `json:"name"`
        Contact  string `json:"contact"`
        Service  string `json:"service"`
        Deadline string `json:"deadline"`
    }
    if err := c.BindJSON(&data); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    _, err := database.Pool.Exec(c.Request.Context(), 
        `UPDATE service_orders 
         SET client_name = $1, client_contact = $2, service_type = $3, deadline = $4 
         WHERE id = $5`,
        data.Name, data.Contact, data.Service, data.Deadline, id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Удалить заявку
r.DELETE("/api/orders/:id/delete", func(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(), 
        "DELETE FROM service_orders WHERE id = $1", id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Страница просмотра заявок (админка)
r.GET("/admin/orders-view", func(c *gin.Context) {
    c.HTML(200, "orders_view.html", gin.H{
        "title": "Заявки | Business Stack Admin",
    })
})

r.GET("/developer/admin", middleware.AuthMiddleware(cfg), func(c *gin.Context) {
    role := c.GetString("role")
    platformRole := c.GetString("platform_role")
    userEmail := c.GetString("user_email")
    
    // Владелец платформы, разработчик, админ или owner
    if userEmail == "dev@businesstack.ru" || platformRole == "owner" || role == "developer" || role == "admin" || role == "owner" {
        c.HTML(200, "admin_dashboard_universal.html", gin.H{
            "title": "Админ-панель",
        })
        return
    }
    c.String(403, "⛔ Доступ только для разработчиков, администраторов и владельца")
})

// ========== API ДЛЯ ПОЛУЧЕНИЯ ДАННЫХ КЛИЕНТА ==========
// ========== API ДЛЯ ПОЛУЧЕНИЯ ДАННЫХ КЛИЕНТА ==========
// Получить данные текущего клиента
r.GET("/api/client/data", middleware.AuthMiddleware(cfg), func(c *gin.Context) {
    userID := c.GetString("user_id")
    userEmail := c.GetString("user_email")
    tenantID := c.GetString("tenant_id")
    
    // Получаем статистику клиента из БД
    var projectsCount, activeServices, ticketsCount, daysWithUs, totalRequests, ordersCount, ideasCount int
    var storageUsed float64
    var regDate time.Time
    
    // ✅ ПОЛУЧАЕМ ВСЕ ДАННЫЕ ПОЛЬЗОВАТЕЛЯ ИЗ БАЗЫ (включая телефон и организацию)
    var userName, userPhone, userOrg, userInn string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT 
            COALESCE(name, '') as name,
            COALESCE(phone, '') as phone,
            COALESCE(organization_name, '') as organization_name,
            COALESCE(organization_inn, '') as organization_inn,
            created_at
        FROM users 
        WHERE id = $1::uuid
    `, userID).Scan(&userName, &userPhone, &userOrg, &userInn, &regDate)
    
    if err != nil {
        log.Printf("❌ Ошибка получения пользователя: %v", err)
        userName = ""
        userPhone = ""
        userOrg = ""
        userInn = ""
    }
    
    // Количество проектов клиента
    err = database.Pool.QueryRow(c.Request.Context(), 
        "SELECT COUNT(*) FROM projects WHERE user_id = $1::uuid AND tenant_id = $2::uuid", userID, tenantID).Scan(&projectsCount)
    if err != nil {
        projectsCount = 0
    }
    
    // Количество активных услуг (подписок)
    err = database.Pool.QueryRow(c.Request.Context(), 
        "SELECT COUNT(*) FROM subscriptions WHERE user_id = $1::uuid AND status = 'active' AND tenant_id = $2::uuid", userID, tenantID).Scan(&activeServices)
    if err != nil {
        activeServices = 0
    }
    
    // Количество обращений в поддержку
    err = database.Pool.QueryRow(c.Request.Context(), 
        "SELECT COUNT(*) FROM support_tickets WHERE user_id = $1::uuid AND tenant_id = $2::uuid", userID, tenantID).Scan(&ticketsCount)
    if err != nil {
        ticketsCount = 0
    }
    
    // Количество дней с регистрации
    if !regDate.IsZero() {
        diff := time.Since(regDate)
        daysWithUs = int(diff.Hours() / 24)
        if daysWithUs < 1 {
            daysWithUs = 1
        }
    } else {
        daysWithUs = 1
    }
    
    // Количество запросов к API
    err = database.Pool.QueryRow(c.Request.Context(), 
        "SELECT COUNT(*) FROM api_usage WHERE user_id = $1::uuid AND tenant_id = $2::uuid", userID, tenantID).Scan(&totalRequests)
    if err != nil {
        totalRequests = 0
    }
    
    // Использовано хранилища
    err = database.Pool.QueryRow(c.Request.Context(), 
        "SELECT COALESCE(SUM(size), 0) FROM cloud_files WHERE user_id = $1::uuid AND tenant_id = $2::uuid", userID, tenantID).Scan(&storageUsed)
    if err != nil {
        storageUsed = 0
    }
    
    // Количество заявок клиента
    err = database.Pool.QueryRow(c.Request.Context(), 
        "SELECT COUNT(*) FROM service_orders WHERE client_contact LIKE $1 AND tenant_id = $2::uuid", "%"+userEmail+"%", tenantID).Scan(&ordersCount)
    if err != nil {
        ordersCount = 0
    }
    
    // Количество идей клиента
    err = database.Pool.QueryRow(c.Request.Context(), 
        "SELECT COUNT(*) FROM feature_requests WHERE user_id = $1::uuid AND tenant_id = $2::uuid", userID, tenantID).Scan(&ideasCount)
    if err != nil {
        ideasCount = 0
    }
    
    // ✅ ВОЗВРАЩАЕМ ВСЕ ДАННЫЕ
    c.JSON(200, gin.H{
        "name":              userName,
        "email":             userEmail,
        "user_id":           userID,
        "phone":             userPhone,
        "organization_name": userOrg,
        "organization_inn":  userInn,
        "projects_count":    projectsCount,
        "active_services":   activeServices,
        "tickets_count":     ticketsCount,
        "days_with_us":      daysWithUs,
        "total_requests":    totalRequests,
        "storage_used":      storageUsed / 1024 / 1024 / 1024,
        "orders_count":      ordersCount,
        "ideas_count":       ideasCount,
        "created_at":        regDate,
        "last_login":        time.Now(),
    })
})
// ========== HR МОДУЛЬ ==========

// Статистика HR
r.GET("/api/hr/stats", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    var totalEmployees, totalCandidates, openVacancies, newApplications int
    var totalPayroll float64
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM employees WHERE status='active' AND tenant_id=$1", tenantID).Scan(&totalEmployees)
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM candidates WHERE status='new' AND tenant_id=$1", tenantID).Scan(&totalCandidates)
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM vacancies WHERE status='open' AND tenant_id=$1", tenantID).Scan(&openVacancies)
    database.Pool.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM candidates WHERE status='new' AND tenant_id=$1", tenantID).Scan(&newApplications)
    database.Pool.QueryRow(c.Request.Context(), "SELECT COALESCE(SUM(salary), 0) FROM employees WHERE status='active' AND tenant_id=$1", tenantID).Scan(&totalPayroll)
    c.JSON(200, gin.H{
        "total_employees":  totalEmployees,
        "total_candidates": totalCandidates,
        "open_vacancies":   openVacancies,
        "new_applications": newApplications,
        "total_payroll":    totalPayroll,
    })
})

// Список сотрудников
r.GET("/api/hr/employees", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    rows, err := database.Pool.Query(c.Request.Context(), 
        `SELECT e.id, e.full_name, COALESCE(p.title, e.position) as position, e.department, 
                e.phone, e.email, e.hire_date, e.salary, e.bonus, e.status 
         FROM employees e 
         LEFT JOIN positions p ON e.position_id = p.id 
         WHERE e.status='active' AND e.tenant_id = $1
         ORDER BY e.hire_date DESC`, tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var employees []gin.H
    for rows.Next() {
        var id int
        var fullName, position, department, phone, email, status string
        var hireDate time.Time
        var salary, bonus float64
        rows.Scan(&id, &fullName, &position, &department, &phone, &email, &hireDate, &salary, &bonus, &status)
        employees = append(employees, gin.H{
            "id": id, "full_name": fullName, "position": position, "department": department,
            "phone": phone, "email": email, "hire_date": hireDate, "salary": salary, "bonus": bonus, "status": status,
        })
    }
    c.JSON(200, employees)
})

// Добавить сотрудника
r.POST("/api/hr/employees", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    var req struct {
        FullName   string  `json:"full_name"`
        Position   string  `json:"position"`
        Department string  `json:"department"`
        Phone      string  `json:"phone"`
        Email      string  `json:"email"`
        Salary     float64 `json:"salary"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    _, err := database.Pool.Exec(c.Request.Context(), 
        "INSERT INTO employees (full_name, position, department, phone, email, salary, hire_date, status, tenant_id) VALUES ($1, $2, $3, $4, $5, $6, NOW(), 'active', $7)",
        req.FullName, req.Position, req.Department, req.Phone, req.Email, req.Salary, tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"message": "Сотрудник добавлен"})
})

// Удалить сотрудника
r.DELETE("/api/hr/employees/:id", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(), "UPDATE employees SET status='inactive' WHERE id=$1", id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Список вакансий
r.GET("/api/hr/vacancies", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    rows, err := database.Pool.Query(c.Request.Context(), 
        `SELECT id, title, COALESCE(salary_from, 0), COALESCE(salary_to, 0), 
                COALESCE(description, ''), status, created_at 
         FROM vacancies WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var vacancies []gin.H
    for rows.Next() {
        var id int
        var title, description, status string
        var salaryFrom, salaryTo float64
        var createdAt time.Time
        
        err := rows.Scan(&id, &title, &salaryFrom, &salaryTo, &description, &status, &createdAt)
        if err != nil {
            continue
        }
        
        vacancies = append(vacancies, gin.H{
            "id":          id,
            "title":       title,
            "salary_from": salaryFrom,
            "salary_to":   salaryTo,
            "description": description,
            "status":      status,
            "created_at":  createdAt,
        })
    }
    c.JSON(200, vacancies)
})

// Создать вакансию
r.POST("/api/hr/vacancies", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    var req struct {
        Title       string  `json:"title"`
        SalaryFrom  float64 `json:"salary_from"`
        SalaryTo    float64 `json:"salary_to"`
        Description string  `json:"description"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    _, err := database.Pool.Exec(c.Request.Context(), 
        "INSERT INTO vacancies (title, salary_from, salary_to, description, status, tenant_id) VALUES ($1, $2, $3, $4, 'open', $5)",
        req.Title, req.SalaryFrom, req.SalaryTo, req.Description, tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"message": "Вакансия создана"})
})

// Удалить вакансию
r.DELETE("/api/hr/vacancies/:id", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(), "DELETE FROM vacancies WHERE id=$1", id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Список кандидатов
r.GET("/api/hr/candidates", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    rows, err := database.Pool.Query(c.Request.Context(), 
        `SELECT id, full_name, COALESCE(vacancy, ''), COALESCE(experience, ''), 
                COALESCE(expected_salary, 0), COALESCE(phone, ''), COALESCE(email, ''), 
                status, created_at 
         FROM candidates WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var candidates []gin.H
    for rows.Next() {
        var id int
        var fullName, vacancy, experience, phone, email, status string
        var expectedSalary float64
        var createdAt time.Time
        
        err := rows.Scan(&id, &fullName, &vacancy, &experience, &expectedSalary, &phone, &email, &status, &createdAt)
        if err != nil {
            continue
        }
        
        candidates = append(candidates, gin.H{
            "id":              id,
            "full_name":       fullName,
            "vacancy":         vacancy,
            "experience":      experience,
            "expected_salary": expectedSalary,
            "phone":           phone,
            "email":           email,
            "status":          status,
            "date":            createdAt.Format("02.01.2006"),
        })
    }
    c.JSON(200, candidates)
})

// Добавить кандидата
r.POST("/api/hr/candidates", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    var req struct {
        FullName       string  `json:"full_name"`
        Vacancy        string  `json:"vacancy"`
        Experience     string  `json:"experience"`
        ExpectedSalary float64 `json:"expected_salary"`
        Phone          string  `json:"phone"`
        Email          string  `json:"email"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    _, err := database.Pool.Exec(c.Request.Context(), 
        "INSERT INTO candidates (full_name, vacancy, experience, expected_salary, phone, email, status, tenant_id) VALUES ($1, $2, $3, $4, $5, $6, 'new', $7)",
        req.FullName, req.Vacancy, req.Experience, req.ExpectedSalary, req.Phone, req.Email, tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"message": "Кандидат добавлен"})
})

// Принять кандидата на работу
r.POST("/api/hr/candidates/:id/hire", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    tenantID := c.GetString("tenant_id")
    var fullName, vacancy, phone, email string
    var expectedSalary float64
    err := database.Pool.QueryRow(c.Request.Context(), 
        "SELECT full_name, vacancy, phone, email, expected_salary FROM candidates WHERE id=$1 AND tenant_id=$2", id, tenantID).Scan(&fullName, &vacancy, &phone, &email, &expectedSalary)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    _, err = database.Pool.Exec(c.Request.Context(), 
        "INSERT INTO employees (full_name, position, phone, email, salary, status, tenant_id) VALUES ($1, $2, $3, $4, $5, 'active', $6)", 
        fullName, vacancy, phone, email, expectedSalary, tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    database.Pool.Exec(c.Request.Context(), "DELETE FROM candidates WHERE id=$1", id)
    c.JSON(200, gin.H{"message": "Кандидат принят на работу"})
})

// Удалить кандидата
r.DELETE("/api/hr/candidates/:id", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(), "DELETE FROM candidates WHERE id=$1", id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Рассчитать зарплату
r.POST("/api/hr/payroll/calculate", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    rows, err := database.Pool.Query(c.Request.Context(), "SELECT id, salary, bonus, tax_percent FROM employees WHERE status='active' AND tenant_id=$1", tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var totalNet, totalTaxes float64
    for rows.Next() {
        var id int
        var salary, bonus, taxPercent float64
        rows.Scan(&id, &salary, &bonus, &taxPercent)
        gross := salary + bonus
        tax := gross * taxPercent / 100
        net := gross - tax
        totalNet += net
        totalTaxes += tax
        database.Pool.Exec(c.Request.Context(), 
            "INSERT INTO payroll (employee_id, month, base_salary, bonus, tax, net_salary, paid) VALUES ($1, date_trunc('month', NOW()), $2, $3, $4, $5, false)",
            id, salary, bonus, tax, net)
    }
    c.JSON(200, gin.H{"total_net": totalNet, "total_taxes": totalTaxes, "message": "Зарплата рассчитана"})
})

// Обновить статус оплаты заявки
r.PUT("/api/orders/:id/payment", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        PaymentStatus string  `json:"payment_status"`
        Budget        float64 `json:"budget"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(),
        "UPDATE service_orders SET payment_status = $1, budget = $2 WHERE id = $3",
        req.PaymentStatus, req.Budget, id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Внести предоплату 50%
r.PUT("/api/orders/:id/deposit", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        DepositAmount float64 `json:"deposit_amount"`
        WorkStatus    string  `json:"work_status"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    var budget float64
    database.Pool.QueryRow(c.Request.Context(),
        "SELECT COALESCE(NULLIF(budget, ''), '0')::float FROM service_orders WHERE id=$1", id).Scan(&budget)

    remaining := budget - req.DepositAmount

    _, err := database.Pool.Exec(c.Request.Context(),
        `UPDATE service_orders
         SET deposit_status = 'paid',
             deposit_amount = $1,
             deposit_date = NOW(),
             remaining_amount = $2,
             work_status = $3
         WHERE id = $4`,
        req.DepositAmount, remaining, req.WorkStatus, id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true, "remaining": remaining})
})

// Внести остаток (после завершения работы)
r.PUT("/api/orders/:id/remaining", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    var req struct {
        RemainingAmount float64 `json:"remaining_amount"`
        WorkStatus      string  `json:"work_status"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    _, err := database.Pool.Exec(c.Request.Context(),
        `UPDATE service_orders
         SET remaining_status = 'paid',
             remaining_amount = $1,
             remaining_date = NOW(),
             payment_status = 'paid',
             work_status = $2
         WHERE id = $3`,
        req.RemainingAmount, req.WorkStatus, id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

        // ========== УПРАВЛЕНЧЕСКИЙ УЧЁТ (ТЕГИ/ПРОЕКТЫ) ==========
    fincoreTags := r.Group("/api/fincore/tags")
    fincoreTags.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"))
    {
        fincoreTags.GET("", handlers.GetFincoreTags)
        fincoreTags.POST("", handlers.CreateFincoreTag)
        fincoreTags.PUT("/:id", handlers.UpdateFincoreTag)
        fincoreTags.DELETE("/:id", handlers.DeleteFincoreTag)
    }

    // Отчёты по тегам
    fincoreReports := r.Group("/api/fincore/reports")
    fincoreReports.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"))
    {
        fincoreReports.GET("/by-tag", handlers.GetFincoreReportByTag)
    }

    // Привязка тегов к проводкам
    fincoreAssign := r.Group("/api/fincore/assign")
    fincoreAssign.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"))
    {
        fincoreAssign.POST("/entry", handlers.AssignTagToEntry)
        fincoreAssign.GET("/entry/:entryId/tags", handlers.GetEntryTags) 
        fincoreAssign.DELETE("/entry/:entry_id/tag/:tag_id", handlers.RemoveTagFromEntry)
        fincoreAssign.POST("/entry/:entryId/tags", handlers.AssignTagsToEntry)//tag
    }

  
    // Экспорт и топ тегов
    fincoreExtra := r.Group("/api/fincore/extra")
    fincoreExtra.Use(middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"))
    {
        fincoreExtra.GET("/export", handlers.ExportFincoreReport)
        fincoreExtra.GET("/top-tags", handlers.GetTopTags)
        fincoreExtra.DELETE("/template/:id", handlers.DeleteTemplatePosting)

    fincoreExtra.GET("/compare", handlers.ComparePeriods)          // Сравнение периодов
    fincoreExtra.GET("/budget-alerts", handlers.CheckBudgetAlerts) // Уведомления о бюджете
    fincoreExtra.GET("/forecast", handlers.GetForecast)            // Прогнозирование
    fincoreExtra.GET("/dashboard", handlers.GetManagementDashboard) // Управленческий дашборд
    fincoreExtra.GET("/plan-fact", handlers.GetPlanFactAnalysis)    // План-факт анализ
    }

    // Страница управленческого учёта
    r.GET("/management", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("fincore"), func(c *gin.Context) {
        c.HTML(http.StatusOK, "fincore_management", gin.H{
            "title": "Управленческий учёт | FinCore",
        })
    })

// ========== ПАНЕЛЬ РАЗРАБОТЧИКА ==========
r.GET("/dev-modules", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    c.HTML(http.StatusOK, "dev_modules_panel", gin.H{
        "title": "DevModules | Управление модулями в разработке",
    })
})

// ========== API ДЛЯ УПРАВЛЕНИЯ МОДУЛЯМИ В РАЗРАБОТКЕ ==========
// Получить все модули
r.GET("/api/dev-modules", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, name, route, COALESCE(icon, '🔧'), COALESCE(description, ''), created_at 
        FROM dev_modules ORDER BY created_at DESC
    `)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var modules []gin.H
    for rows.Next() {
        var id int
        var name, route, icon, desc string
        var createdAt time.Time
        rows.Scan(&id, &name, &route, &icon, &desc, &createdAt)
        modules = append(modules, gin.H{
            "id": id, "name": name, "route": route,
            "icon": icon, "desc": desc, "created_at": createdAt,
        })
    }
    c.JSON(200, modules)
})

// Добавить модуль
r.POST("/api/dev-modules", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    var req struct {
        Name  string `json:"name"`
        Route string `json:"route"`
        Icon  string `json:"icon"`
        Desc  string `json:"desc"`
    }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO dev_modules (name, route, icon, description, created_at, updated_at)
        VALUES ($1, $2, $3, $4, NOW(), NOW())
        ON CONFLICT (route) DO UPDATE SET
            name = EXCLUDED.name,
            icon = EXCLUDED.icon,
            description = EXCLUDED.description,
            updated_at = NOW()
    `, req.Name, req.Route, req.Icon, req.Desc)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Удалить модуль
r.DELETE("/api/dev-modules/:route", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    route := c.Param("route")
    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM dev_modules WHERE route = $1
    `, "/"+route)
    
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})


// ========== API ДЛЯ ЗАЯВОК НА МОДУЛИ ==========
// Получить заявки на модули (только для админов)
r.GET("/api/admin/module-requests", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, 
               COALESCE(module, '') as module_name,
               COALESCE(user_id::text, '') as user_id,
               COALESCE(user_email, '') as user_email,
               COALESCE(user_name, '') as user_name,
               COALESCE(issue, '') as issue,
               COALESCE(status, 'new') as status,
               created_at
        FROM module_feedback
        ORDER BY created_at DESC
    `)
    if err != nil {
        log.Printf("❌ Ошибка запроса module_feedback: %v", err)
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var requests []gin.H
    for rows.Next() {
        var id int
        var moduleName, userID, userEmail, userName, issue, status string
        var createdAt time.Time
        err := rows.Scan(&id, &moduleName, &userID, &userEmail, &userName, &issue, &status, &createdAt)
        if err != nil {
            log.Printf("❌ Ошибка сканирования: %v", err)
            continue
        }
        requests = append(requests, gin.H{
            "id": id,
            "module_name": moduleName,
            "user_id": userID,
            "user_email": userEmail,
            "user_name": userName,
            "message": issue,
            "status": status,
            "created_at": createdAt.Format("02.01.2006 15:04"),
        })
    }
    c.JSON(200, requests)
})

// Получить детали заявки
r.GET("/api/admin/module-requests/:id", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    var moduleName, userID, userEmail, userName, issue, status, url, userAgent string
    var createdAt time.Time

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT module, user_id::text, user_email, user_name, issue, status, url, user_agent, created_at
        FROM module_feedback
        WHERE id = $1
    `, id).Scan(&moduleName, &userID, &userEmail, &userName, &issue, &status, &url, &userAgent, &createdAt)

    if err != nil {
        c.JSON(404, gin.H{"error": "Заявка не найдена"})
        return
    }

    c.JSON(200, gin.H{
        "id": id,
        "module_name": moduleName,
        "user_id": userID,
        "user_email": userEmail,
        "user_name": userName,
        "message": issue,
        "status": status,
        "url": url,
        "user_agent": userAgent,
        "created_at": createdAt.Format("02.01.2006 15:04"),
    })
})

// Удалить заявку
r.DELETE("/api/admin/module-requests/:id", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    _, err := database.Pool.Exec(c.Request.Context(),
        "DELETE FROM module_feedback WHERE id = $1", id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// Обновить статус заявки
r.PUT("/api/admin/module-requests/:id/status", middleware.AuthMiddleware(cfg), middleware.AdminMiddleware(cfg), func(c *gin.Context) {
    id := c.Param("id")
    var req struct{ Status string `json:"status"` }
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    _, err := database.Pool.Exec(c.Request.Context(), 
        "UPDATE module_feedback SET status = $1, resolved = $2 WHERE id = $3", 
        req.Status, req.Status == "done" || req.Status == "resolved", id)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"success": true})
})

// ========== WEBSOCKET ДЛЯ РЕАЛЬНОГО ВРЕМЕНИ ==========
// WebSocket для инвентаризации
r.GET("/ws/inventory", func(c *gin.Context) {
    handlers.InventoryWebSocket(c)
})

// WebSocket для поставщиков
//r.GET("/ws/suppliers", func(c *gin.Context) {
    //handlers.SupplierWebSocket(c)
//})

// WebSocket для приемки
//r.GET("/ws/receipts", func(c *gin.Context) {
    //handlers.ReceiptWebSocket(c)
//})

log.Println("✅ WebSocket маршруты зарегистрированы")

// ========== API ДЛЯ ПОЛУЧЕНИЯ ПРОЕКТОВ ==========
// Получить проекты текущего пользователя
r.GET("/api/my-projects", middleware.AuthMiddleware(cfg), func(c *gin.Context) {
    userID := c.GetString("user_id")
    userEmail := c.GetString("user_email")
    tenantID := c.GetString("tenant_id")
    
    log.Printf("📊 Запрос проектов: userID=%s, email=%s, tenantID=%s", userID, userEmail, tenantID)
    
    // Если tenant_id пустой - получаем из БД
    if tenantID == "" {
        err := database.Pool.QueryRow(c.Request.Context(), 
            "SELECT tenant_id::text FROM users WHERE id = $1::uuid", userID).Scan(&tenantID)
        if err != nil {
            log.Printf("❌ Ошибка получения tenant_id: %v", err)
        }
    }
    
    // Получаем проекты для этого пользователя
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            id::text,
            name,
            COALESCE(description, '') as description,
            COALESCE(status, 'active') as status,
            created_at,
            updated_at,
            user_id::text,
            tenant_id::text
        FROM projects 
        WHERE user_id = $1::uuid
        ORDER BY created_at DESC
    `, userID)
    
    if err != nil {
        log.Printf("❌ Ошибка получения проектов: %v", err)
        c.JSON(200, gin.H{
            "projects": []gin.H{},
            "count": 0,
            "error": err.Error(),
        })
        return
    }
    defer rows.Close()
    
    var projects []gin.H
    for rows.Next() {
        var id, name, description, status, userIDFromDB, tenantIDFromDB string
        var createdAt, updatedAt time.Time
        
        err := rows.Scan(&id, &name, &description, &status, &createdAt, &updatedAt, &userIDFromDB, &tenantIDFromDB)
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
            continue
        }
        
        projects = append(projects, gin.H{
            "id": id,
            "name": name,
            "description": description,
            "status": status,
            "user_id": userIDFromDB,
            "tenant_id": tenantIDFromDB,
            "created_at": createdAt,
            "updated_at": updatedAt,
            "created_at_formatted": createdAt.Format("02.01.2006 15:04"),
        })
    }
    
    log.Printf("📋 Найдено проектов: %d для пользователя %s", len(projects), userEmail)
    
    c.JSON(200, gin.H{
        "success": true,
        "projects": projects,
        "count": len(projects),
        "user_id": userID,
        "tenant_id": tenantID,
    })
})

      r.NoRoute(func(c *gin.Context) {
        c.HTML(http.StatusNotFound, "404.html", gin.H{
            "Title":   "Страница не найдена - Business Stack",
            "Version": "3.0",
        })
    })

    port := ":" + cfg.Port
    baseURL := "http://localhost:" + cfg.Port
    fmt.Printf("   🔒 Безопасность     %s/security-center\n", baseURL)
    fmt.Printf("📍 ВСЕ ИНТЕРФЕЙСЫ ДОСТУПНЫ ПО ССЫЛКАМ:\n\n")
    fmt.Printf("   🔹 Главная           %s/\n", baseURL)
    fmt.Printf("   🔹 Дашборд          %s/dashboard-improved\n", baseURL)
    fmt.Printf("   🔹 Админка          %s/admin\n", baseURL)
    fmt.Printf("   🔹 CRM              %s/crm\n", baseURL)
    fmt.Printf("   🔹 Аналитика        %s/analytics\n", baseURL)
    fmt.Printf("   🔹 Платежи          %s/payment\n", baseURL)
    fmt.Printf("   🔹 Тарифы           %s/pricing\n", baseURL)
    fmt.Printf("   🔹 Партнёры         %s/partner\n", baseURL)
    fmt.Printf("   🔹 Контакты         %s/contact\n", baseURL)
    fmt.Printf("   🔹 Логистика        %s/logistics\n", baseURL)
    fmt.Printf("   🔹 Отслеживание     %s/track\n\n", baseURL)
    fmt.Printf("   🔐 Вход             %s/login\n", baseURL)
    fmt.Printf("   🔐 Регистрация      %s/register\n", baseURL)
    fmt.Printf("   🔐 Восстановление   %s/forgot-password\n\n", baseURL)
    fmt.Printf("   ⚙️  Настройки       %s/settings\n", baseURL)
    fmt.Printf("   ⚙️  Пользователи    %s/users\n", baseURL)
    fmt.Printf("   ⚙️  Подписки        %s/subscriptions\n", baseURL)
    fmt.Printf("   ⚙️  Мои подписки    %s/my-subscriptions\n", baseURL)
    fmt.Printf("   👤 Профиль          %s/profile\n\n", baseURL)
    fmt.Printf("   💳 Оплата картой    %s/bank_card_payment\n", baseURL)
    fmt.Printf("   💳 USDT             %s/usdt-payment\n", baseURL)
    fmt.Printf("   💳 RUB              %s/rub-payment\n", baseURL)
    fmt.Printf("   💳 Успешно          %s/payment-success\n\n", baseURL)
    fmt.Printf("   📊 Админ (Fixed)    %s/admin-fixed\n", baseURL)
    fmt.Printf("   📊 Gold Admin       %s/gold-admin\n", baseURL)
    fmt.Printf("   📊 Админ БД         %s/database-admin\n\n", baseURL)
    fmt.Printf("   📈 Дашборд улучш.   %s/dashboard-improved\n", baseURL)
    fmt.Printf("   📈 Real-time        %s/realtime-dashboard\n", baseURL)
    fmt.Printf("   📈 Выручка          %s/revenue-dashboard\n", baseURL)
    fmt.Printf("   📈 Партнёрский      %s/partner-dashboard\n", baseURL)
    fmt.Printf("   📈 Унифицированный  %s/unified-dashboard\n\n", baseURL)
    fmt.Printf("   📡 API Health       %s/api/health\n", baseURL)
    fmt.Printf("   🔹 FusionAPI        %s/fusion-portal\n", baseURL)
    fmt.Printf("   🔹 API Документация %s/api/fusion/docs\n", baseURL)
    fmt.Printf("   📡 CRM Health       %s/api/crm/health\n", baseURL)
    fmt.Printf("   📡 Система          %s/api/system/stats\n", baseURL)
    fmt.Printf("   📡 Тест             %s/api/test\n", baseURL)
    fmt.Printf("   📡 Отслеживание API %s/api/delivery/track/:id\n\n", baseURL)
    fmt.Printf("============================================================\n")
    fmt.Printf("   ⚙️  Конфигурация: порт=%s, режим=%s, БД=%s\n", cfg.Port, cfg.Env, cfg.DBName)
    fmt.Printf("   🔒 SKIP_AUTH=%v – все защищённые страницы открыты без токена\n", cfg.SkipAuth)
    fmt.Printf("============================================================\n")

    log.Printf("🚀 Сервер запущен на порту %s", port)
    
    // Запуск планировщиков
    handlers.StartSyncScheduler()
    handlers.StartBitrixSyncScheduler()
    handlers.StartTeamSphereScheduler()

   
  // Favicon обработка
r.GET("/favicon.ico", func(c *gin.Context) {
    c.File("./static/favicon.ico")
})
r.GET("/favicon.svg", func(c *gin.Context) {
    c.File("./static/favicon.svg")
})




// AI Assistant widget (добавить после инициализации шаблонов)
r.GET("/api/ai/widget", func(c *gin.Context) {
    c.HTML(http.StatusOK, "ai_widget.html", gin.H{
        "title": "AI Assistant",
    })
})
    
    r.GET("/team/team", func(c *gin.Context) {
        c.HTML(http.StatusOK, "team_page.html", gin.H{
            "title": "Команда | TeamSphere",
        })
    })

    // Tasks page
    r.GET("/tasks", func(c *gin.Context) {
        c.HTML(http.StatusOK, "tasks.html", gin.H{
            "title": "Задачи - TeamSphere",
        })
    })
    
    // Chat page
    r.GET("/chat", func(c *gin.Context) {
        c.HTML(http.StatusOK, "chat.html", gin.H{
            "title": "Чат - TeamSphere",
        })
    })
    
    // TeamSphere Calendar page
    r.GET("/team-calendar", func(c *gin.Context) {
        c.HTML(http.StatusOK, "calendar.html", gin.H{
            "title": "Календарь - TeamSphere",
        })
    })

    r.GET("/security-center", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("security"), func(c *gin.Context) {
        c.HTML(http.StatusOK, "security_universal.html", gin.H{
            "title": "Security Center | Business Stack",
        })
    })

    // Универсальная аналитика - новый путь
    r.GET("/analytics-center", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("analytics"), func(c *gin.Context) {
        c.HTML(http.StatusOK, "analytics_universal.html", gin.H{
            "title": "Analytics Center | Business Stack",
        })
    })

// Страница импорта Excel
r.GET("/import-excel", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("import-excel"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "import-excel.html", gin.H{
        "title": "Импорт Excel | FinCore",
    })
})

// Страница закрытия месяца
r.GET("/month-end", middleware.AuthMiddleware(cfg), middleware.RequireModuleAccess("month-closing"), func(c *gin.Context) {
    c.HTML(http.StatusOK, "month_end", gin.H{
        "title": "Закрытие месяца | FinCore",
    })
})    

// Страница управления данными клиента
r.GET("/client-data", middleware.AuthMiddleware(cfg), func(c *gin.Context) {
    c.HTML(http.StatusOK, "client_data_management", gin.H{
        "title": "Управление данными | FinCore",
    })
})
    // Страница "Мои приложения"
    r.GET("/my-apps", handlers.GetMyApps)
    r.GET("/my-apps/settings", handlers.AppSettingsPage)

    // API для маркетплейса
    r.GET("/api/marketplace/my-apps", handlers.GetMyAppsAPI)
    r.PUT("/api/marketplace/apps/:id/settings", handlers.UpdateAppSettings)

// ========== МОБИЛЬНОЕ ПРИЛОЖЕНИЕ (PWA) - МАРКЕТПЛЕЙС МОДУЛЕЙ ==========
// Главная страница мобильного приложения
r.GET("/marketplace-app", func(c *gin.Context) {
    c.HTML(http.StatusOK, "mobile_app.html", gin.H{
        "title": "BizStore — Маркетплейс модулей",
        "version": "1.0.0",
    })
})


// Мобильное приложение для подписок
r.GET("/mobile", func(c *gin.Context) {
    c.HTML(http.StatusOK, "mobile_subscriptions.html", gin.H{
        "title": "Subscription Mobile",
    })
})

// API для скачивания APK (если есть файл)
r.GET("/api/mobile/download", func(c *gin.Context) {
    apkPath := "./static/app/bizstore.apk"
    if _, err := os.Stat(apkPath); os.IsNotExist(err) {
        c.JSON(200, gin.H{
            "download_url": "https://play.google.com/store/apps/details?id=com.bizstore.app",
            "message":      "Скачайте приложение из Google Play",
        })
        return
    }
    c.Header("Content-Type", "application/vnd.android.package-archive")
    c.Header("Content-Disposition", "attachment; filename=bizstore.apk")
    c.File(apkPath)
})


    
// Запускаем на всех интерфейсах с улучшенными таймаутами
srv := &http.Server{
    Addr:         "0.0.0.0:" + cfg.Port,
    Handler:      r,
    ReadTimeout:  120 * time.Second,   // 2 минуты на чтение
    WriteTimeout: 120 * time.Second,   // 2 минуты на запись
   IdleTimeout:  21600 * time.Second,   // 6 ЧАСОВ бездействия (вот это главное!)
    MaxHeaderBytes: 1 << 20,           // 1 MB
}

log.Printf("🚀 Сервер запущен на порту %s с таймаутом бездействия 3 часа", cfg.Port)
// Проверка доступности хендлеров (чтобы не было ошибок компиляции)
// Если некоторых хендлеров нет - закомментируй соответствующие строки
log.Println("✅ Все маршруты зарегистрированы")
if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
    log.Fatalf("❌ Ошибка запуска сервера: %v", err)
}
}


// ========== HR АССИСТЕНТ ==========
func handleHRRequest(c *gin.Context, tenantID, userID, message string) string {
    msg := strings.ToLower(message)
    
    // ❌ НЕТ ДЕФОЛТНЫХ ЗНАЧЕНИЙ!
    // ✅ ТОЛЬКО ИЗ КОНТЕКСТА!
    if tenantID == "" {
        return "⚠️ Ошибка: tenant_id не найден в контексте"
    }
    
    // Создание вакансии
    if strings.Contains(msg, "создай") && strings.Contains(msg, "ваканс") {
        title := "Новая вакансия"
        if strings.Contains(msg, "разработчик") {
            title = "Разработчик Go"
        } else if strings.Contains(msg, "менеджер") {
            title = "Менеджер по продажам"
        } else if strings.Contains(msg, "дизайнер") {
            title = "UI/UX Дизайнер"
        } else if strings.Contains(msg, "тестировщик") {
            title = "QA Инженер"
        }
        
        _, err := database.Pool.Exec(context.Background(),
            `INSERT INTO vacancies (title, status, tenant_id, created_at) 
             VALUES ($1, 'open', $2, NOW())`,
            title, tenantID)
        
        if err != nil {
            return fmt.Sprintf("Ошибка создания вакансии: %v", err)
        }
        
        return fmt.Sprintf("✅ Вакансия '%s' успешно создана!", title)
    }
    
    // Список вакансий
    if strings.Contains(msg, "список") && strings.Contains(msg, "ваканс") {
        rows, err := database.Pool.Query(context.Background(),
            `SELECT title, status, created_at FROM vacancies 
             WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 10`,
            tenantID)
        if err != nil {
            return "Ошибка получения списка вакансий"
        }
        defer rows.Close()
        
        var vacancies []string
        for rows.Next() {
            var title, status string
            var createdAt time.Time
            rows.Scan(&title, &status, &createdAt)
            statusIcon := "🟢"
            if status != "open" {
                statusIcon = "🔴"
            }
            vacancies = append(vacancies, fmt.Sprintf("%s %s (%s) - %s", statusIcon, title, status, createdAt.Format("02.01.2006")))
        }
        
        if len(vacancies) == 0 {
            return "📋 Вакансий пока нет. Создайте первую: 'создай вакансию разработчик'"
        }
        
        return "📋 **Список вакансий:**\n" + strings.Join(vacancies, "\n")
    }
    
    // Статистика
    if strings.Contains(msg, "статистик") {
        var total int
        database.Pool.QueryRow(context.Background(),
            "SELECT COUNT(*) FROM vacancies WHERE tenant_id = $1", tenantID).Scan(&total)
        
        var open int
        database.Pool.QueryRow(context.Background(),
            "SELECT COUNT(*) FROM vacancies WHERE tenant_id = $1 AND status = 'open'", tenantID).Scan(&open)
        
        closed := total - open
        
        return fmt.Sprintf(`📊 HR Статистика:
• Всего вакансий: %d
• Открытых: %d
• Закрытых: %d

💡 Команды:
• "создай вакансию разработчик"
• "список вакансий"`, total, open, closed)
    }
    
    return `🤖 Я HR ассистент

Что я могу:
• создать вакансию разработчик
• список вакансий
• статистика

Просто напишите, что нужно сделать!`
}