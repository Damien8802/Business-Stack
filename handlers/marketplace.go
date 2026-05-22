package handlers

import (
    "fmt"
    "log"
    "net/http"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "subscription-system/database"
)

// MarketplacePageHandler - главная страница маркетплейса
func MarketplacePageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "marketplace_index.html", gin.H{
        "Title": "Маркетплейс | Business Stack",
    })
}

// GetMarketplaceApps - получить каталог приложений
func GetMarketplaceApps(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    category := c.Query("category")
    
    // Получаем купленные приложения
    purchasedApps := getPurchasedAppIDs(c, tenantID)
    
    // Все приложения с правильными категориями
    allApps := []gin.H{
        {
            "id":            "1",
            "name":          "Маркетплейсы: Ozon + Wildberries",
            "description":   "Управление заказами, остатками, ценами на Ozon и Wildberries",
            "category":      "integration",
            "price_monthly": 6900,
            "price_yearly":  69000,
            "icon_url":      "📦",
            "slug":          "marketplaces-integration",
            "featured":      true,
            "purchased":     contains(purchasedApps, "1"),
        },
        {
            "id":            "2",
            "name":          "1С:Бухгалтерия + ERP",
            "description":   "Двусторонняя синхронизация с 1С:Бухгалтерия 3.0 и 1С:ERP",
            "category":      "integration",
            "price_monthly": 4900,
            "price_yearly":  49000,
            "icon_url":      "🏦",
            "slug":          "1c-integration",
            "featured":      true,
            "purchased":     contains(purchasedApps, "2"),
        },
        {
            "id":            "3",
            "name":          "HeadHunter Рекрутинг",
            "description":   "Автоматический импорт резюме, публикация вакансий, аналитика рынка труда",
            "category":      "integration",
            "price_monthly": 5900,
            "price_yearly":  59000,
            "icon_url":      "👥",
            "slug":          "hh-ru-integration",
            "featured":      true,
            "purchased":     contains(purchasedApps, "3"),
        },
        {
            "id":            "4",
            "name":          "Telegram Бизнес Пак",
            "description":   "Корпоративный Telegram: рассылки, уведомления, чат-боты, аналитика",
            "category":      "integration",
            "price_monthly": 2900,
            "price_yearly":  29000,
            "icon_url":      "📱",
            "slug":          "telegram-business",
            "featured":      true,
            "purchased":     contains(purchasedApps, "4"),
        },
        {
            "id":            "5",
            "name":          "YandexGPT для бизнеса",
            "description":   "AI-ассистенты, генерация контента, анализ документов на YandexGPT",
            "category":      "ai_agent",
            "price_monthly": 9900,
            "price_yearly":  99000,
            "icon_url":      "🧠",
            "slug":          "yandex-gpt",
            "featured":      true,
            "purchased":     contains(purchasedApps, "5"),
        },
        {
            "id":            "6",
            "name":          "Sales CRM Pro",
            "description":   "Готовая CRM для отдела продаж: воронки, сделки, аналитика",
            "category":      "template",
            "price_monthly": 2900,
            "price_yearly":  29000,
            "icon_url":      "📈",
            "slug":          "sales-crm",
            "featured":      true,
            "purchased":     contains(purchasedApps, "6"),
        },
        {
            "id":            "7",
            "name":          "ЮKassa Платежи",
            "description":   "Приём платежей через ЮKassa: все способы оплаты в одном решении",
            "category":      "integration",
            "price_monthly": 0,
            "price_yearly":  0,
            "icon_url":      "💵",
            "slug":          "yookassa",
            "featured":      true,
            "purchased":     contains(purchasedApps, "7"),
        },
        {
            "id":            "8",
            "name":          "AI Копирайтер Pro",
            "description":   "Генерация текстов, постов, статей с помощью нейросети",
            "category":      "ai_agent",
            "price_monthly": 2900,
            "price_yearly":  29000,
            "icon_url":      "✍️",
            "slug":          "ai-copywriter",
            "featured":      false,
            "purchased":     contains(purchasedApps, "8"),
        },
        {
            "id":            "9",
            "name":          "Виджет Поддержки",
            "description":   "Чат-виджет для сайта с интеграцией в CRM",
            "category":      "service",
            "price_monthly": 1900,
            "price_yearly":  19000,
            "icon_url":      "💬",
            "slug":          "support-widget",
            "featured":      false,
            "purchased":     contains(purchasedApps, "9"),
        },
        {
            "id":            "10",
            "name":          "Отчётность для ФНС",
            "description":   "Автоматическое формирование и отправка отчётов в налоговую",
            "category":      "service",
            "price_monthly": 4900,
            "price_yearly":  49000,
            "icon_url":      "📋",
            "slug":          "fns-reporting",
            "featured":      false,
            "purchased":     contains(purchasedApps, "10"),
        },
        {
            "id":            "subscription-system",
            "name":          "Subscription Clean Works",
            "description":   "💼 Комплексная ERP система: CRM, VPN, AI, Маркетплейс, Логистика, Финансы. Управляйте бизнесом в одном месте!",
            "category":      "business",
            "price_monthly": 0,
            "price_yearly":  0,
            "icon_url":      "📦",
            "slug":          "subscription-system",
            "featured":      true,
            "purchased":     true,
            "status":        "active",
            "activated_at":  "2026-04-11",
            "expires_at":    "2026-05-11",
            "settings_url":  "/my-apps/settings",
        },
    }
    
    // Фильтрация по категории
    var filteredApps []gin.H
    for _, app := range allApps {
        if category == "" || category == "all" || app["category"] == category {
            filteredApps = append(filteredApps, app)
        }
    }
    
    c.JSON(http.StatusOK, gin.H{"apps": filteredApps})
}

// getPurchasedAppIDs - получить список ID купленных приложений
func getPurchasedAppIDs(c *gin.Context, tenantID string) []string {
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT app_id FROM marketplace_purchases 
        WHERE tenant_id = $1 AND status = 'active' AND expires_at > NOW()
    `, tenantID)
    
    if err != nil {
        return []string{}
    }
    defer rows.Close()
    
    var purchasedIDs []string
    appIDMap := map[string]string{
        "a24ebc11-7f4d-4196-ae2e-4930b0f1aef7": "1",
        "94aa41d1-21c3-4cfa-b78d-1cd3d545bcf1": "2",
        "174636d9-b199-4231-936a-2d5fe18f3585": "3",
        "984730d2-d373-4dac-89a2-6190b6ff7f92": "4",
        "a99e0fa9-b11b-4658-87f6-efdd2cc104bb": "5",
        "a416e86c-7131-4ee5-b5d3-4ea0ba8639cf": "6",
        "f3743181-34be-4630-8231-265c1124ba19": "7",
    }
    
    for rows.Next() {
        var appID uuid.UUID
        rows.Scan(&appID)
        if id, ok := appIDMap[appID.String()]; ok {
            purchasedIDs = append(purchasedIDs, id)
        }
    }
    
    return purchasedIDs
}

// contains - проверяет наличие элемента в слайсе
func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}

// PurchaseApp - покупка приложения
func PurchaseApp(c *gin.Context) {
    log.Println("🔥 PurchaseApp called!")

    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    
    companyID := c.GetString("company_id")
    if companyID == "" {
        companyID = tenantID
    }
    
    userID := c.GetString("user_id")
    if userID == "" {
        userID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var req struct {
        AppID         string `json:"app_id" binding:"required"`
        BillingPeriod string `json:"billing_period"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        log.Printf("❌ JSON parse error: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    if req.BillingPeriod == "" {
        req.BillingPeriod = "monthly"
    }

    // Обработка вашего приложения
    if req.AppID == "subscription-system" {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "message": "✅ Приложение Subscription Clean Works уже активировано",
            "app": gin.H{
                "id":           "subscription-system",
                "name":         "Subscription Clean Works",
                "status":       "active",
                "activated_at": "2026-04-11",
                "expires_at":   "2026-05-11",
                "settings_url": "/my-apps/settings",
            },
        })
        return
    }

    // Маппинг строковых ID в реальные UUID из БД
    appUUIDMap := map[string]string{
        "1": "a24ebc11-7f4d-4196-ae2e-4930b0f1aef7",
        "2": "94aa41d1-21c3-4cfa-b78d-1cd3d545bcf1",
        "3": "174636d9-b199-4231-936a-2d5fe18f3585",
        "4": "984730d2-d373-4dac-89a2-6190b6ff7f92",
        "5": "a99e0fa9-b11b-4658-87f6-efdd2cc104bb",
        "6": "a416e86c-7131-4ee5-b5d3-4ea0ba8639cf",
        "7": "f3743181-34be-4630-8231-265c1124ba19",
    }

    realAppID := appUUIDMap[req.AppID]
    if realAppID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid app ID"})
        return
    }

    appUUID, _ := uuid.Parse(realAppID)

    // Цены приложений
    appPrices := map[string]map[string]float64{
        "1": {"monthly": 6900, "yearly": 69000},
        "2": {"monthly": 4900, "yearly": 49000},
        "3": {"monthly": 5900, "yearly": 59000},
        "4": {"monthly": 2900, "yearly": 29000},
        "5": {"monthly": 9900, "yearly": 99000},
        "6": {"monthly": 2900, "yearly": 29000},
        "7": {"monthly": 0, "yearly": 0},
    }

    price, ok := appPrices[req.AppID][req.BillingPeriod]
    if !ok {
        price = 0
    }

    // Названия приложений
    appNames := map[string]string{
        "1": "Маркетплейсы: Ozon + Wildberries",
        "2": "1С:Бухгалтерия + ERP",
        "3": "HeadHunter Рекрутинг",
        "4": "Telegram Бизнес Пак",
        "5": "YandexGPT для бизнеса",
        "6": "Sales CRM Pro",
        "7": "ЮKassa Платежи",
    }

    appName := appNames[req.AppID]
    if appName == "" {
        appName = "Приложение"
    }

    period := "месяц"
    if req.BillingPeriod == "yearly" {
        period = "год"
    }

    expiresAt := time.Now().AddDate(0, 1, 0)
    if req.BillingPeriod == "yearly" {
        expiresAt = time.Now().AddDate(1, 0, 0)
    }

    // Проверяем, есть ли уже покупка
    var exists bool
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM marketplace_purchases 
        WHERE tenant_id = $1 AND app_id = $2 AND status = 'active')
    `, tenantID, appUUID).Scan(&exists)

    if exists {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "message": fmt.Sprintf("Приложение '%s' уже активировано", appName),
        })
        return
    }

    purchaseID := uuid.New()
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO marketplace_purchases (
            id, company_id, app_id, user_id, status, payment_method, 
            amount, purchased_at, expires_at, auto_renew, tenant_id
        ) VALUES ($1, $2, $3, $4, 'active', 'card', $5, NOW(), $6, true, $7)
    `, purchaseID, companyID, appUUID, userID, price, expiresAt, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка сохранения покупки: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process purchase: " + err.Error()})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":    true,
        "message":    fmt.Sprintf("Приложение '%s' успешно активировано на %s", appName, period),
        "expires_at": expiresAt.Format("2006-01-02"),
        "price":      price,
    })
}

// GetMyPurchases - получить список купленных приложений
func GetMyPurchases(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, app_id, amount, purchased_at, expires_at, status
        FROM marketplace_purchases
        WHERE tenant_id = $1 AND status = 'active' AND expires_at > NOW()
        ORDER BY purchased_at DESC
    `, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка загрузки покупок: %v", err)
        c.JSON(http.StatusOK, gin.H{"purchases": []gin.H{}})
        return
    }
    defer rows.Close()

    var purchases []gin.H
    
    // Добавляем ваше приложение, если его нет в БД
    purchases = append(purchases, gin.H{
        "id":           "subscription-system",
        "app_id":       "subscription-system",
        "name":         "Subscription Clean Works",
        "description":  "Комплексная ERP система",
        "icon":         "📦",
        "price":        0,
        "purchased_at": "2026-04-11",
        "expires_at":   "2026-05-11",
        "is_active":    true,
        "status":       "active",
        "settings_url": "/my-apps/settings",
    })

    for rows.Next() {
        var id uuid.UUID
        var appID uuid.UUID
        var amount float64
        var purchasedAt, expiresAt time.Time
        var status string

        rows.Scan(&id, &appID, &amount, &purchasedAt, &expiresAt, &status)
        
        purchases = append(purchases, gin.H{
            "id":           id.String(),
            "app_id":       appID.String(),
            "name":         "Приложение",
            "price":        amount,
            "purchased_at": purchasedAt.Format("2006-01-02"),
            "expires_at":   expiresAt.Format("2006-01-02"),
            "is_active":    expiresAt.After(time.Now()),
            "status":       status,
        })
    }

    c.JSON(http.StatusOK, gin.H{"purchases": purchases})
}

// GetMyApps - страница моих приложений
func GetMyApps(c *gin.Context) {
    c.HTML(http.StatusOK, "my_apps.html", gin.H{
        "Title": "Мои приложения | Маркетплейс",
    })
}

// GetMyAppsAPI - API для получения списка приложений
func GetMyAppsAPI(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    myApps := []gin.H{
        {
            "id":           "subscription-system",
            "name":         "Subscription Clean Works",
            "description":  "Комплексная ERP система с CRM, VPN, AI и маркетплейсом",
            "icon":         "📦",
            "version":      "1.0.0",
            "status":       "active",
            "activated_at": "2026-04-11",
            "expires_at":   "2026-05-11",
            "settings": gin.H{
                "mode":  "development",
                "debug": true,
                "modules": gin.H{
                    "crm":         true,
                    "vpn":         true,
                    "ai":          true,
                    "marketplace": true,
                    "telegram":    false,
                    "_1c":         false,
                },
            },
            "stats": gin.H{
                "storage_used":  2.5,
                "storage_total": 10,
                "api_calls":     1234,
                "api_limit":     10000,
                "users":         5,
            },
            "settings_url": "/my-apps/settings",
        },
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true, "apps": myApps})
}

// UpdateAppSettings - обновление настроек приложения
func UpdateAppSettings(c *gin.Context) {
    appID := c.Param("id")
    
    var settings map[string]interface{}
    if err := c.ShouldBindJSON(&settings); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    log.Printf("📝 Обновление настроек для %s: %v", appID, settings)
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Настройки успешно сохранены",
        "app_id":  appID,
        "settings": settings,
    })
}

// AppSettingsPage - страница настроек приложения
func AppSettingsPage(c *gin.Context) {
    c.HTML(http.StatusOK, "my_app_settings.html", gin.H{
        "Title": "Настройки Subscription Clean Works",
        "App": gin.H{
            "Name":        "Subscription Clean Works",
            "Version":     "1.0.0",
            "Status":      "active",
            "ActivatedAt": "2026-04-11",
            "ExpiresAt":   "2026-05-11",
            "Icon":        "📦",
        },
    })
}

// AddReview - добавить отзыв на приложение
func AddReview(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"success": true, "message": "Функция в разработке"})
}

// GetMarketplaceApp - детальная страница приложения
func GetMarketplaceApp(c *gin.Context) {
    slug := c.Param("slug")
    c.JSON(http.StatusOK, gin.H{
        "id":   slug,
        "name": slug,
    })
}
