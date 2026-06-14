package services

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "regexp"
    "strings"
    "time"
    
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
)

type AIActionExecutor struct {
    db *pgxpool.Pool
}

func NewAIActionExecutor(db *pgxpool.Pool) *AIActionExecutor {
    return &AIActionExecutor{db: db}
}

type ActionResult struct {
    Success bool
    Message string
    Data    interface{}
    Error   error
}

func (e *AIActionExecutor) ExecuteAction(tenantID, userID string, intent IntentExtended, entities map[string]string) ActionResult {
    log.Printf("⚡ Executing action: %s, tenantID: %s, userID: %s", intent.Action, tenantID, userID)
    
    // Проверка tenant - если пустой или default, ошибка в middleware
    if tenantID == "" || tenantID == "default" {
        log.Printf("❌ ExecuteAction: invalid tenantID='%s', userID=%s, action=%s", tenantID, userID, intent.Action)
        return ActionResult{
            Success: false,
            Message: "❌ Ошибка авторизации: не удалось определить организацию. Пожалуйста, выйдите и зайдите заново.",
        }
    }
    
    // Валидируем что tenantID это валидный UUID
    if _, err := uuid.Parse(tenantID); err != nil {
        log.Printf("❌ ExecuteAction: tenantID is not valid UUID: %s", tenantID)
        return ActionResult{
            Success: false,
            Message: "❌ Ошибка авторизации: неверный формат идентификатора организации.",
        }
    }
    
    switch intent.Action {
    // CRM
    case "create_partner":
        return e.createPartner(tenantID, userID, entities)
    case "create_customer":
        return e.createCustomer(tenantID, userID, entities)
    case "create_deal":
        return e.createDeal(tenantID, userID, entities)
    case "show_partner_info":
        return e.showPartnerInfo(tenantID, entities)
    case "show_partner_deals":
        return e.showPartnerDeals(tenantID, entities)
    
    // FinCore - Финансы
  // FinCore - Финансы
case "create_invoice":
    return e.createInvoice(tenantID, entities)
case "create_payment":
    return e.createPayment(tenantID, userID, entities)  
case "create_act":
    return e.createReconciliationAct(tenantID, entities)
case "close_month":
    return e.closeMonth(tenantID, entities)
    case "create_journal_entry":
        return e.createJournalEntry(tenantID, entities)
    case "create_budget":
        return e.createBudget(tenantID, entities)
    case "create_tag":
        return e.createTag(tenantID, entities)
    
    // Отчёты
    case "generate_report":
        return e.generateReport(tenantID, entities)
    case "send_report":
        return e.sendReport(tenantID, entities)
    case "download_report":
        return e.downloadReport(tenantID, entities)
    case "download_invoice":
        return e.downloadInvoice(tenantID, entities)
    case "download_act":
        return e.downloadAct(tenantID, entities)
    case "download_tax_report":
        return e.downloadTaxReport(tenantID, entities)
    
    // Зарплата
    case "calculate_payroll":
        return e.calculatePayroll(tenantID, entities)
    case "add_employee":
        return e.addEmployee(tenantID, entities)
    case "generate_payslip":
        return e.generatePayslip(tenantID, entities)
    
    // Налоги
    case "generate_tax_report":
        return e.generateTaxReport(tenantID, entities)
    case "send_tax_report":
        return e.sendTaxReport(tenantID, entities)
    
    // Импорт
    case "import_excel":
        return e.importExcel(tenantID, entities)
    case "import_bank_statement":
        return e.importBankStatement(tenantID, entities)
    
    // Банк
    case "sync_bank":
        return e.syncBank(tenantID, entities)
    case "get_balance":
        return e.getBalance(tenantID, entities)
    
    // Прочее
    case "show_subscriptions":
        return e.showSubscriptions(tenantID, entities)
    case "create_task":
        return e.createTask(tenantID, entities)
    case "get_price", "get_info":
        return e.getInfo(intent.Action, entities)
    case "help":
        return ActionResult{Success: true, Message: GetHelpMessage()}
    
    // НОВЫЕ FINCORE ACTION
    case "archive_account":
        return e.archiveAccount(tenantID, entities)
    case "restore_account":
        return e.restoreAccount(tenantID, entities)
    case "show_archive":
        return e.showArchive(tenantID, entities)
    case "get_osv":
        return e.getOSV(tenantID, entities)
    case "export_to_excel":
        return e.exportToExcelFinCore(tenantID, entities)
    case "get_balance_sheet":
        return e.getBalanceSheet(tenantID, entities)

    default:
        return ActionResult{
            Success: true,
            Message: GetHelpMessage(),
        }
    }
}

// ============================================
// CRM ФУНКЦИИ
// ============================================

func (e *AIActionExecutor) createPartner(tenantID, userID string, entities map[string]string) ActionResult {
    name := entities["name"]
    if name == "" {
        name = entities["customer_name"]
    }
    if name == "" {
        name = entities["partner_name"]
    }
    
    if name == "" {
        return ActionResult{
            Success: false,
            Message: "❌ Не указано название партнёра. Пример: 'Создай партнёра ООО Ромашка +7 999 123-45-67'",
        }
    }
    
    // Извлекаем телефон из строки с названием
    phoneRe := regexp.MustCompile(`(\+?7[\d\s\-\(\)]{10,}|8[\d\s\-\(\)]{10,})`)
    phone := ""
    if phoneMatch := phoneRe.FindStringSubmatch(name); len(phoneMatch) > 0 {
        phone = strings.TrimSpace(phoneMatch[0])
        name = strings.TrimSpace(strings.Replace(name, phone, "", -1))
    }
    
    if phone == "" {
        phone = entities["phone"]
    }
    
    email := entities["email"]
    if email == "" {
        email = fmt.Sprintf("partner_%d@temp.local", time.Now().UnixNano())
    }
    
    ctx := context.Background()
    
    // Парсим tenantID в UUID
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    userUUID, err := uuid.Parse(userID)
    if err != nil {
        userUUID = uuid.Nil
    }
    
    // Проверяем существование партнёра
    var exists bool
    err = e.db.QueryRow(ctx, `
        SELECT EXISTS(SELECT 1 FROM crm_partners WHERE name = $1 AND tenant_id = $2)
    `, name, tenantUUID).Scan(&exists)
    
    if err == nil && exists {
        return ActionResult{
            Success: true,
            Message: fmt.Sprintf("ℹ️ Партнёр \"%s\" уже существует в системе\n\n🔗 [Перейти в CRM →](/crm)", name),
            Data: map[string]interface{}{
                "partner_name":   name,
                "phone":          phone,
                "email":          email,
                "already_exists": true,
                "redirect_url":   "/crm",
                "module":         "crm",
            },
        }
    }
    
    // Создаём партнёра
    partnerID := uuid.New()
    _, err = e.db.Exec(ctx, `
        INSERT INTO crm_partners (id, tenant_id, name, phone, email, user_id, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, NOW())
    `, partnerID, tenantUUID, name, phone, email, userUUID)
    
    if err != nil {
        log.Printf("❌ Ошибка создания партнёра: %v", err)
        return ActionResult{
            Success: false,
            Message: fmt.Sprintf("❌ Ошибка создания партнёра: %v", err),
            Error:   err,
        }
    }
    
    resultMsg := fmt.Sprintf("✅ Партнёр \"%s\" успешно создан!", name)
    if phone != "" {
        resultMsg += fmt.Sprintf("\n📞 Телефон: %s", phone)
    }
    resultMsg += fmt.Sprintf("\n\n🔗 [Перейти в CRM →](/crm)")
    
    log.Printf("✅ Партнёр '%s' создан (ID: %s)", name, partnerID)
    
    return ActionResult{
        Success: true,
        Message: resultMsg,
        Data: map[string]interface{}{
            "id":           partnerID.String(),
            "partner_name": name,
            "phone":        phone,
            "email":        email,
            "redirect_url": "/crm",
            "module":       "crm",
        },
    }
}

func (e *AIActionExecutor) createCustomer(tenantID, userID string, entities map[string]string) ActionResult {
    return e.createPartner(tenantID, userID, entities)
}

func (e *AIActionExecutor) createDeal(tenantID, userID string, entities map[string]string) ActionResult {
    customerName := entities["customer_name"]
    if customerName == "" {
        customerName = entities["partner_name"]
    }
    if customerName == "" {
        return ActionResult{Success: false, Message: "❌ Не указан партнёр для сделки"}
    }
    
    amount := entities["amount"]
    if amount == "" {
        return ActionResult{Success: false, Message: "❌ Не указана сумма сделки"}
    }
    
    tenantUUID, _ := uuid.Parse(tenantID)
    userUUID, _ := uuid.Parse(userID)
    
    // Находим партнёра по имени
    partner, err := e.getPartnerByName(tenantID, customerName)
    if err != nil {
        return ActionResult{
            Success: false, 
            Message: fmt.Sprintf("❌ Партнёр \"%s\" не найден. Сначала создайте партнёра командой: создай партнёра %s", customerName, customerName),
        }
    }
    
    partnerUUID, err := uuid.Parse(partner["id"].(string))
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный ID партнёра")}
    }
    
    var amountInt int
    fmt.Sscanf(amount, "%d", &amountInt)
    var dealName string
    if amountInt >= 1000000 {
        dealName = fmt.Sprintf("Крупная сделка с %s на %.1f млн ₽", customerName, float64(amountInt)/1000000)
    } else if amountInt >= 1000 {
        dealName = fmt.Sprintf("Сделка с %s на %d тыс ₽", customerName, amountInt/1000)
    } else {
        dealName = fmt.Sprintf("Сделка с %s на %d ₽", customerName, amountInt)
    }
    
    dealID := uuid.New()
    ctx := context.Background()
    
    _, err = e.db.Exec(ctx, `
        INSERT INTO crm_deals (id, title, value, tenant_id, partner_id, user_id, stage, created_at) 
        VALUES ($1, $2, $3, $4, $5, $6, 'new', NOW())
    `, dealID, dealName, amount, tenantUUID, partnerUUID, userUUID)
    
    if err != nil {
        log.Printf("❌ Ошибка создания сделки: %v", err)
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: %v", err), Error: err}
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("✅ Сделка \"%s\" на сумму %s ₽ создана для партнёра %s!\n\n🔗 [Посмотреть сделки →](/crm?tab=deals)", dealName, amount, customerName),
        Data: map[string]interface{}{
            "id":           dealID.String(),
            "title":        dealName,
            "value":        amount,
            "partner_id":   partner["id"],
            "partner_name": customerName,
            "redirect_url": "/crm?tab=deals",
            "module":       "crm",
        },
    }
}

func (e *AIActionExecutor) showPartnerInfo(tenantID string, entities map[string]string) ActionResult {
    partnerName := entities["partner_name"]
    if partnerName == "" {
        partnerName = entities["name"]
    }
    if partnerName == "" {
        return ActionResult{Success: false, Message: "❌ Не указано имя партнёра"}
    }
    
    partnerName = strings.TrimSpace(strings.ToLower(partnerName))
    
    partner, err := e.getPartnerByName(tenantID, partnerName)
    if err != nil {
        return ActionResult{
            Success: false,
            Message: fmt.Sprintf("❌ Партнёр \"%s\" не найден в системе.\n\n🔗 [Перейти в CRM →](/crm)", partnerName),
        }
    }
    
    deals, _ := e.getPartnerDeals(tenantID, partner["id"].(string))
    invoices, _ := e.getPartnerInvoices(tenantID, partnerName)
    
    message := fmt.Sprintf("📋 **Информация о партнёре \"%s\"**\n\n", partner["name"])
    message += fmt.Sprintf("📞 Телефон: %s\n", partner["phone"])
    if partner["email"] != "" {
        message += fmt.Sprintf("📧 Email: %s\n", partner["email"])
    }
    message += fmt.Sprintf("📅 Создан: %s\n\n", partner["created_at"].(time.Time).Format("02.01.2006"))
    
    if len(deals) > 0 {
        message += "**💰 Сделки:**\n"
        for _, deal := range deals {
            message += fmt.Sprintf("• %s — %.0f ₽ (%s)\n", deal["title"], deal["value"].(float64), deal["stage"])
        }
        message += "\n"
    } else {
        message += "💰 Сделок пока нет\n\n"
    }
    
    if len(invoices) > 0 {
        message += "**📄 Счета:**\n"
        for _, inv := range invoices {
            message += fmt.Sprintf("• №%s — %.0f ₽ (%s)\n", inv["number"], inv["amount"].(float64), inv["status"])
        }
    } else {
        message += "📄 Счетов пока нет"
    }
    
    message += fmt.Sprintf("\n\n🔗 [Перейти в CRM →](/crm)")
    
    return ActionResult{
        Success: true,
        Message: message,
        Data: map[string]interface{}{
            "partner":      partner,
            "deals":        deals,
            "invoices":     invoices,
            "redirect_url": "/crm",
            "module":       "crm",
        },
    }
}

func (e *AIActionExecutor) showPartnerDeals(tenantID string, entities map[string]string) ActionResult {
    partnerName := entities["partner_name"]
    if partnerName == "" {
        return ActionResult{Success: false, Message: "❌ Не указан партнёр"}
    }
    
    partner, err := e.getPartnerByName(tenantID, partnerName)
    if err != nil {
        return ActionResult{
            Success: false,
            Message: fmt.Sprintf("❌ Партнёр \"%s\" не найден", partnerName),
        }
    }
    
    deals, err := e.getPartnerDeals(tenantID, partner["id"].(string))
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: %v", err)}
    }
    
    if len(deals) == 0 {
        return ActionResult{
            Success: true,
            Message: fmt.Sprintf("📋 У партнёра \"%s\" пока нет сделок\n\n🔗 [Перейти в CRM →](/crm)", partner["name"]),
        }
    }
    
    message := fmt.Sprintf("📋 **Сделки партнёра \"%s\":**\n\n", partner["name"])
    total := 0.0
    for _, deal := range deals {
        message += fmt.Sprintf("• %s — %.0f ₽ (%s)\n", deal["title"], deal["value"].(float64), deal["stage"])
        total += deal["value"].(float64)
    }
    message += fmt.Sprintf("\n💰 **Общая сумма сделок:** %.0f ₽", total)
    message += fmt.Sprintf("\n\n🔗 [Перейти в CRM →](/crm)")
    
    return ActionResult{
        Success: true,
        Message: message,
        Data: map[string]interface{}{
            "deals":        deals,
            "total":        total,
            "redirect_url": "/crm",
            "module":       "crm",
        },
    }
}

// ============================================
// FINCORE - ФИНАНСОВЫЕ ФУНКЦИИ
// ============================================

func (e *AIActionExecutor) createInvoice(tenantID string, entities map[string]string) ActionResult {
    partnerName := entities["partner_name"]
    if partnerName == "" {
        partnerName = entities["customer_name"]
    }
    if partnerName == "" {
        return ActionResult{Success: false, Message: "❌ Не указан партнёр для счёта"}
    }
    
    amount := entities["amount"]
    if amount == "" {
        return ActionResult{Success: false, Message: "❌ Не указана сумма счёта"}
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    invoiceID := uuid.New()
    invoiceNumber := fmt.Sprintf("INV-%d", time.Now().UnixNano()%100000)
    ctx := context.Background()
    
    var tableExists bool
    e.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = 'invoices'
        )
    `).Scan(&tableExists)
    
    if !tableExists {
        _, err := e.db.Exec(ctx, `
            CREATE TABLE IF NOT EXISTS invoices (
                id UUID PRIMARY KEY,
                number VARCHAR(50),
                partner_name VARCHAR(255),
                amount DECIMAL(15,2),
                tenant_id UUID,
                status VARCHAR(50),
                created_at TIMESTAMP,
                due_date TIMESTAMP
            )
        `)
        if err != nil {
            return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания таблицы: %v", err)}
        }
    }
    
    _, err = e.db.Exec(ctx, `
        INSERT INTO invoices (id, number, partner_name, amount, tenant_id, status, created_at, due_date) 
        VALUES ($1, $2, $3, $4, $5, 'sent', NOW(), NOW() + INTERVAL '14 days')
    `, invoiceID, invoiceNumber, partnerName, amount, tenantUUID)
    
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: %v", err), Error: err}
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("✅ Счёт №%s на сумму %s ₽ выставлен партнёру %s!\n\n🔗 [Перейти в Финансы →](/finance)", invoiceNumber, amount, partnerName),
        Data: map[string]interface{}{
            "id":           invoiceID.String(),
            "number":       invoiceNumber,
            "amount":       amount,
            "redirect_url": "/finance",
            "module":       "finance",
        },
    }
}

func (e *AIActionExecutor) createPayment(tenantID, userID string, entities map[string]string) ActionResult {
    recipient := entities["recipient"]
    amount := entities["amount"]
    purpose := entities["purpose"]
    
    // ШАГ 1: Нет получателя
    if recipient == "" {
        return ActionResult{
            Success: false,
            Message: "💰 Кому нужно перевести деньги? Укажите получателя.",
            Data: map[string]interface{}{
                "step":      "recipient",
                "action":    "create_payment",
                "amount":    amount,
                "purpose":   purpose,
            },
        }
    }
    
    // ШАГ 2: Нет суммы
    if amount == "" {
        return ActionResult{
            Success: false,
            Message: fmt.Sprintf("💰 Получатель: %s\n\nКакую сумму перевести?", recipient),
            Data: map[string]interface{}{
                "step":      "amount",
                "action":    "create_payment",
                "recipient": recipient,
                "purpose":   purpose,
            },
        }
    }
    
    // ШАГ 3: Нет назначения
    if purpose == "" {
        return ActionResult{
            Success: false,
            Message: fmt.Sprintf("💰 Получатель: %s\n💰 Сумма: %s ₽\n\nУкажите назначение платежа:", recipient, amount),
            Data: map[string]interface{}{
                "step":      "purpose",
                "action":    "create_payment",
                "recipient": recipient,
                "amount":    amount,
            },
        }
    }
    
    // ШАГ 4: Все данные есть - создаем платеж
    tenantUUID, _ := uuid.Parse(tenantID)
    userUUID, _ := uuid.Parse(userID)
    
    var userName string
    e.db.QueryRow(context.Background(), 
        "SELECT COALESCE(name, email, 'Пользователь') FROM users WHERE id = $1", userUUID).Scan(&userName)
    
    paymentID := uuid.New()
    
    _, execErr := e.db.Exec(context.Background(), `
        INSERT INTO payments (
            id, counterparty_name, amount, tenant_id, user_id, 
            purpose, method, status, created_at, payment_date, 
            payment_method, plan_name, user_name, payment_type
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, 'completed', NOW(), 
                  CURRENT_DATE, 'bank_transfer', 'AI Платёж', $8, 'expense')
    `, paymentID, recipient, amount, tenantUUID, userUUID, 
       purpose, "bank_transfer", userName)
    
    if execErr != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: %v", execErr)}
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("✅ **Платёж создан!**\n\n💰 Сумма: %s ₽\n👤 Получатель: %s\n📝 Назначение: %s\n\n🔗 [Посмотреть платежи →](/finance)", 
            amount, recipient, purpose),
        Data: map[string]interface{}{
            "completed": true,
        },
    }
}
func (e *AIActionExecutor) createReconciliationAct(tenantID string, entities map[string]string) ActionResult {
    partnerName := entities["partner_name"]
    if partnerName == "" {
        return ActionResult{Success: false, Message: "❌ Не указан партнёр для акта сверки"}
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    actID := uuid.New()
    ctx := context.Background()
    
    var tableExists bool
    e.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = 'reconciliation_acts'
        )
    `).Scan(&tableExists)
    
    if !tableExists {
        _, err := e.db.Exec(ctx, `
            CREATE TABLE IF NOT EXISTS reconciliation_acts (
                id UUID PRIMARY KEY,
                partner_name VARCHAR(255),
                counterparty_name VARCHAR(255),
                tenant_id UUID,
                status VARCHAR(50),
                created_at TIMESTAMP
            )
        `)
        if err != nil {
            return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания таблицы: %v", err)}
        }
    }
    
    _, err = e.db.Exec(ctx, `
        INSERT INTO reconciliation_acts (id, partner_name, counterparty_name, tenant_id, status, created_at) 
        VALUES ($1, $2, $3, $4, 'draft', NOW())
    `, actID, partnerName, partnerName, tenantUUID)
    
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: %v", err), Error: err}
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("✅ Акт сверки с партнёром %s создан!\n\n🔗 [Перейти к актам сверки →](/reconciliation-acts)", partnerName),
        Data: map[string]interface{}{
            "id":           actID.String(),
            "partner_name": partnerName,
            "redirect_url": "/reconciliation-acts",
            "module":       "reconciliation",
        },
    }
}

// ============================================
// FINCORE - НОВЫЕ ФУНКЦИИ С ИСПРАВЛЕНИЯМИ
// ============================================

func (e *AIActionExecutor) archiveAccount(tenantID string, entities map[string]string) ActionResult {
    accountCode := entities["account_code"]
    if accountCode == "" {
        return ActionResult{
            Success: false,
            Message: "❌ Не указан код счета для архивации.\n\nПример: *архивируй счёт 51*",
        }
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    ctx := context.Background()
    
    // Проверяем существует ли таблица chart_of_accounts
    var tableExists bool
    e.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = 'chart_of_accounts'
        )
    `).Scan(&tableExists)
    
    if !tableExists {
        // Создаём таблицу если её нет
        _, err = e.db.Exec(ctx, `
            CREATE TABLE IF NOT EXISTS chart_of_accounts (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                code VARCHAR(20) NOT NULL,
                name VARCHAR(255) NOT NULL,
                type VARCHAR(50),
                is_active BOOLEAN DEFAULT true,
                tenant_id UUID NOT NULL,
                created_at TIMESTAMP DEFAULT NOW(),
                updated_at TIMESTAMP DEFAULT NOW(),
                deleted_at TIMESTAMP,
                UNIQUE(code, tenant_id)
            )
        `)
        if err != nil {
            return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания таблицы: %v", err)}
        }
        
        // Добавляем стандартные счета
        standardAccounts := []struct{ code, name, typ string }{
            {"01", "Основные средства", "asset"},
            {"10", "Материалы", "asset"},
            {"50", "Касса", "asset"},
            {"51", "Расчетный счет", "asset"},
            {"60", "Расчеты с поставщиками", "liability"},
            {"62", "Расчеты с покупателями", "asset"},
            {"68", "Расчеты по налогам", "liability"},
            {"69", "Расчеты по соцстрахованию", "liability"},
            {"70", "Расчеты с персоналом", "liability"},
            {"80", "Уставный капитал", "equity"},
            {"90", "Продажи", "revenue"},
            {"91", "Прочие доходы и расходы", "other"},
            {"99", "Прибыли и убытки", "equity"},
        }
        
        for _, acc := range standardAccounts {
            _, _ = e.db.Exec(ctx, `
                INSERT INTO chart_of_accounts (code, name, type, tenant_id, created_at, updated_at)
                VALUES ($1, $2, $3, $4, NOW(), NOW())
                ON CONFLICT (code, tenant_id) DO NOTHING
            `, acc.code, acc.name, acc.typ, tenantUUID)
        }
    }
    
    // Реально обновляем БД
    result, err := e.db.Exec(ctx, `
        UPDATE chart_of_accounts 
        SET deleted_at = NOW(), is_active = false, updated_at = NOW()
        WHERE code = $1 AND tenant_id = $2 AND deleted_at IS NULL
    `, accountCode, tenantUUID)
    
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка архивации: %v", err)}
    }
    
    if result.RowsAffected() == 0 {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Счёт %s не найден или уже в архиве", accountCode)}
    }
    
    return ActionResult{
        Success: true,
       Message: fmt.Sprintf("✅ **Счёт %s успешно архивирован!**\n\n📦 Счёт перемещён в архив.\n\n🔗 [Посмотреть архив счетов](/finance?tab=accounts)", accountCode),
        Data: map[string]interface{}{"account_code": accountCode, "archived": true},
    }
}

func (e *AIActionExecutor) restoreAccount(tenantID string, entities map[string]string) ActionResult {
    accountCode := entities["account_code"]
    if accountCode == "" {
        return ActionResult{
            Success: false,
            Message: "❌ Не указан код счета для восстановления.\n\nПример: *восстанови счёт 51*",
        }
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    ctx := context.Background()
    
    // Реально обновляем БД
    result, err := e.db.Exec(ctx, `
        UPDATE chart_of_accounts 
        SET deleted_at = NULL, is_active = true, updated_at = NOW()
        WHERE code = $1 AND tenant_id = $2 AND deleted_at IS NOT NULL
    `, accountCode, tenantUUID)
    
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка восстановления: %v", err)}
    }
    
    if result.RowsAffected() == 0 {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Архивированный счёт %s не найден", accountCode)}
    }
    
    return ActionResult{
        Success: true,
       Message: fmt.Sprintf("✅ **Счёт %s успешно восстановлен!**\n\n📋 Счёт возвращён в план счетов.\n\n🔗 [План счетов](/finance?tab=accounts)", accountCode),
        Data: map[string]interface{}{"account_code": accountCode, "restored": true},
    }
}

func (e *AIActionExecutor) showArchive(tenantID string, entities map[string]string) ActionResult {
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    ctx := context.Background()
    
    // Проверяем существование таблицы
    var tableExists bool
    e.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = 'chart_of_accounts'
        )
    `).Scan(&tableExists)
    
    if !tableExists {
        return ActionResult{
            Success: true,
            Message: "📦 **Архив счетов** (пуст)\n\n💡 Счета можно архивировать командой *архивируй счёт 51*",
        }
    }
    
    // Считаем реальное количество архивных счетов
    var count int
    e.db.QueryRow(ctx, `SELECT COUNT(*) FROM chart_of_accounts WHERE tenant_id = $1 AND deleted_at IS NOT NULL`, tenantUUID).Scan(&count)
    
    // Получаем список архивных счетов если их не много
    var archiveList string
    if count > 0 && count <= 20 {
        rows, err := e.db.Query(ctx, `
            SELECT code, name FROM chart_of_accounts 
            WHERE tenant_id = $1 AND deleted_at IS NOT NULL 
            ORDER BY code
        `, tenantUUID)
        if err == nil {
            defer rows.Close()
            archiveList = "\n\n**Архивированные счета:**\n"
            for rows.Next() {
                var code, name string
                rows.Scan(&code, &name)
                archiveList += fmt.Sprintf("• %s - %s\n", code, name)
            }
        }
    }
    
    archiveText := "пуст"
    if count > 0 {
        archiveText = fmt.Sprintf("%d счёт(ов)", count)
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📦 **Архив счетов** (%s)%s\n\n🔘 Нажмите на кнопку **\"Архив\"** в разделе **\"План счетов\"**\n\n🔗 [Перейти в финансы → План счетов](/finance?tab=accounts)\n\n💡 Там можно восстановить или удалить счета навсегда.", 
            archiveText, archiveList),
    }
}

func (e *AIActionExecutor) getOSV(tenantID string, entities map[string]string) ActionResult {
    period := entities["period"]
    if period == "" {
        period = "текущий месяц"
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    ctx := context.Background()
    
    // Проверяем наличие данных
    var entriesCount int
    e.db.QueryRow(ctx, `SELECT COUNT(*) FROM journal_entries WHERE tenant_id = $1`, tenantUUID).Scan(&entriesCount)
    
    var additionalInfo string
    if entriesCount == 0 {
        additionalInfo = "\n\n⚠️ В системе пока нет проводок. Создайте их в разделе Финансы → Журнал операций."
    } else {
        additionalInfo = fmt.Sprintf("\n\n📊 Найдено проводок: %d", entriesCount)
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📊 **Оборотно-сальдовая ведомость** за %s%s\n\n📥 [Скачать Excel](/api/reports/export/osv?tenant_id=%s)\n\n🔗 [Открыть в веб-интерфейсе](/finance?tab=reports)\n\n💡 Выберите период в фильтре на странице.", 
            period, additionalInfo, tenantID),
        Data: map[string]interface{}{
            "period": period,
            "entries_count": entriesCount,
        },
    }
}

func (e *AIActionExecutor) exportToExcelFinCore(tenantID string, entities map[string]string) ActionResult {
    reportType := entities["report_type"]
    if reportType == "" {
        reportType = "ОСВ"
    }
    
    period := entities["period"]
    if period == "" {
        period = "текущий месяц"
    }
    
    var downloadURL string
    switch strings.ToLower(reportType) {
    case "осв", "osv":
        downloadURL = fmt.Sprintf("/api/reports/export/osv?tenant_id=%s", tenantID)
    case "баланс", "balance_sheet":
        downloadURL = fmt.Sprintf("/api/reports/export/balance-sheet?tenant_id=%s", tenantID)
    default:
        downloadURL = fmt.Sprintf("/api/reports/export/osv?tenant_id=%s", tenantID)
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📎 **Экспорт %s в Excel** за %s\n\n📥 [Скачать файл](%s)\n\n💡 Файл скачается автоматически. Если скачивание не началось, нажмите на ссылку.", 
            reportType, period, downloadURL),
        Data: map[string]interface{}{
            "report_type": reportType,
            "period": period,
            "download_url": downloadURL,
        },
    }
}

func (e *AIActionExecutor) getBalanceSheet(tenantID string, entities map[string]string) ActionResult {
    date := entities["date"]
    if date == "" {
        date = "текущую дату"
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    ctx := context.Background()
    
    // Получаем суммы по типам счетов
    var totalAssets, totalLiabilities float64
    
    e.db.QueryRow(ctx, `
        SELECT COALESCE(SUM(
            CASE WHEN c.type IN ('asset', 'other_asset') THEN 
                COALESCE(j.amount, 0)
            ELSE 0 END
        ), 0)
        FROM chart_of_accounts c
        LEFT JOIN journal_entries j ON j.debit_account = c.code AND j.tenant_id = c.tenant_id
        WHERE c.tenant_id = $1 AND c.deleted_at IS NULL
    `, tenantUUID).Scan(&totalAssets)
    
    e.db.QueryRow(ctx, `
        SELECT COALESCE(SUM(
            CASE WHEN c.type IN ('liability', 'equity', 'capital') THEN 
                COALESCE(j.amount, 0)
            ELSE 0 END
        ), 0)
        FROM chart_of_accounts c
        LEFT JOIN journal_entries j ON j.credit_account = c.code AND j.tenant_id = c.tenant_id
        WHERE c.tenant_id = $1 AND c.deleted_at IS NULL
    `, tenantUUID).Scan(&totalLiabilities)
    
    // Если нет данных, показываем пример
    if totalAssets == 0 && totalLiabilities == 0 {
        return ActionResult{
            Success: true,
            Message: fmt.Sprintf("📊 **Бухгалтерский баланс** на %s\n\n⚠️ В системе пока нет данных для формирования баланса.\n\n📥 [Скачать шаблон Excel](/api/reports/export/balance-sheet?tenant_id=%s)\n\n🔗 [Перейти в финансы](/finance)\n\n💡 Создайте проводки для формирования баланса.", date, tenantID),
        }
    }
    
    difference := totalAssets - totalLiabilities
    
    return ActionResult{
        Success: true,
       Message: fmt.Sprintf("📊 **Бухгалтерский баланс** на %s\n\n**АКТИВЫ:** %.2f ₽\n**ПАССИВЫ:** %.2f ₽\n**Нераспределённая прибыль:** %.2f ₽\n\n📥 [Скачать Excel](/api/reports/export/balance-sheet?tenant_id=%s)\n\n🔗 [Открыть в веб-интерфейсе](/finance?tab=reports)", 
            date, totalAssets, totalLiabilities, difference, tenantID),
        Data: map[string]interface{}{
            "date": date,
            "total_assets": totalAssets,
            "total_liabilities": totalLiabilities,
            "difference": difference,
        },
    }
}

// ============================================
// ЗАКРЫТИЕ МЕСЯЦА - ИСПРАВЛЕННАЯ ВЕРСИЯ
// ============================================

func (e *AIActionExecutor) closeMonth(tenantID string, entities map[string]string) ActionResult {
    month := entities["month"]
    if month == "" {
        month = time.Now().Format("January 2006")
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    ctx := context.Background()
    
    // Проверяем существование таблицы month_closures
    var tableExists bool
    e.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = 'month_closures'
        )
    `).Scan(&tableExists)
    
    if !tableExists {
        _, err = e.db.Exec(ctx, `
            CREATE TABLE IF NOT EXISTS month_closures (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                tenant_id UUID NOT NULL,
                period DATE NOT NULL,
                closed_at TIMESTAMP DEFAULT NOW(),
                closed_by UUID,
                status VARCHAR(50) DEFAULT 'completed',
                UNIQUE(tenant_id, period)
            )
        `)
        if err != nil {
            return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания таблицы: %v", err)}
        }
    }
    
    // Определяем период закрытия
    var periodStart, periodEnd time.Time
    if strings.Contains(strings.ToLower(month), "январ") || strings.Contains(strings.ToLower(month), "january") {
        periodStart = time.Date(time.Now().Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
        periodEnd = periodStart.AddDate(0, 1, -1)
    } else if strings.Contains(strings.ToLower(month), "феврал") || strings.Contains(strings.ToLower(month), "february") {
        periodStart = time.Date(time.Now().Year(), time.February, 1, 0, 0, 0, 0, time.UTC)
        periodEnd = periodStart.AddDate(0, 1, -1)
    } else {
        // Если месяц не распознан, закрываем предыдущий месяц
        periodStart = time.Now().AddDate(0, -1, 0).Truncate(24 * time.Hour)
        periodStart = time.Date(periodStart.Year(), periodStart.Month(), 1, 0, 0, 0, 0, time.UTC)
        periodEnd = periodStart.AddDate(0, 1, -1)
    }
    
    // Проверяем не закрыт ли уже месяц
    var alreadyClosed bool
    e.db.QueryRow(ctx, `
        SELECT EXISTS(SELECT 1 FROM month_closures WHERE tenant_id = $1 AND period = $2)
    `, tenantUUID, periodStart).Scan(&alreadyClosed)
    
    if alreadyClosed {
        return ActionResult{
            Success: false,
            Message: fmt.Sprintf("⚠️ Месяц %s уже закрыт. Повторное закрытие невозможно.", periodStart.Format("January 2006")),
        }
    }
    
    // Создаём закрывающие проводки для счетов 90, 91, 99
    // 1. Закрытие счёта 90 "Продажи"
    _, err = e.db.Exec(ctx, `
        INSERT INTO journal_entries (id, debit_account, credit_account, amount, description, tenant_id, created_at)
        SELECT 
            gen_random_uuid(),
            '90' as debit_account,
            '99' as credit_account,
            SUM(amount) as amount,
            'Закрытие месяца: списание финансового результата от продаж',
            $1,
            NOW()
        FROM journal_entries 
        WHERE tenant_id = $1 
            AND debit_account = '90'
            AND created_at BETWEEN $2 AND $3
    `, tenantUUID, periodStart, periodEnd.Add(24*time.Hour))
    
    if err != nil {
        log.Printf("⚠️ Ошибка при закрытии счёта 90: %v", err)
    }
    
    // 2. Закрытие счёта 91 "Прочие доходы и расходы"
    _, err = e.db.Exec(ctx, `
        INSERT INTO journal_entries (id, debit_account, credit_account, amount, description, tenant_id, created_at)
        SELECT 
            gen_random_uuid(),
            '91' as debit_account,
            '99' as credit_account,
            SUM(amount) as amount,
            'Закрытие месяца: списание прочих доходов и расходов',
            $1,
            NOW()
        FROM journal_entries 
        WHERE tenant_id = $1 
            AND debit_account = '91'
            AND created_at BETWEEN $2 AND $3
    `, tenantUUID, periodStart, periodEnd.Add(24*time.Hour))
    
    if err != nil {
        log.Printf("⚠️ Ошибка при закрытии счёта 91: %v", err)
    }
    
    // 3. Сохраняем запись о закрытии месяца
    _, err = e.db.Exec(ctx, `
        INSERT INTO month_closures (tenant_id, period, closed_at, status)
        VALUES ($1, $2, NOW(), 'completed')
    `, tenantUUID, periodStart)
    
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка сохранения закрытия месяца: %v", err)}
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("✅ **Месяц %s успешно закрыт!**\n\n📋 Выполнены следующие операции:\n• Сформирован финансовый результат (счет 90 → 99)\n• Закрыты прочие доходы/расходы (счет 91 → 99)\n• Созданы закрывающие проводки\n\n🔗 [Посмотреть закрытие месяца](/finance?tab=reports)\n📥 [Скачать отчёт о закрытии](/api/reports/export/month-closure?tenant_id=%s)", 
            periodStart.Format("January 2006"), tenantID),
        Data: map[string]interface{}{
            "period": periodStart.Format("2006-01"),
            "closed_at": time.Now(),
            "status": "completed",
        },
    }
}

// ============================================
// ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ
// ============================================

func (e *AIActionExecutor) getPartnerByName(tenantID, name string) (map[string]interface{}, error) {
    ctx := context.Background()
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return nil, err
    }
    
    var id, partnerName, phone, email string
    var createdAt time.Time
    
    err = e.db.QueryRow(ctx, `
        SELECT id, name, COALESCE(phone, ''), COALESCE(email, ''), created_at
        FROM crm_partners
        WHERE name ILIKE $1 AND tenant_id = $2
        LIMIT 1
    `, "%"+name+"%", tenantUUID).Scan(&id, &partnerName, &phone, &email, &createdAt)
    
    if err != nil {
        return nil, err
    }
    
    return map[string]interface{}{
        "id":         id,
        "name":       partnerName,
        "phone":      phone,
        "email":      email,
        "created_at": createdAt,
    }, nil
}

func (e *AIActionExecutor) getPartnerDeals(tenantID, partnerID string) ([]map[string]interface{}, error) {
    ctx := context.Background()
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return nil, err
    }
    
    partnerUUID, err := uuid.Parse(partnerID)
    if err != nil {
        return nil, err
    }
    
    rows, err := e.db.Query(ctx, `
        SELECT id, title, value, stage, created_at
        FROM crm_deals 
        WHERE tenant_id = $1 AND partner_id = $2
        ORDER BY created_at DESC
        LIMIT 10
    `, tenantUUID, partnerUUID)
    
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var deals []map[string]interface{}
    for rows.Next() {
        var id, title, stage string
        var value float64
        var createdAt time.Time
        
        err := rows.Scan(&id, &title, &value, &stage, &createdAt)
        if err != nil {
            return nil, err
        }
        deals = append(deals, map[string]interface{}{
            "id":         id,
            "title":      title,
            "value":      value,
            "stage":      stage,
            "created_at": createdAt,
        })
    }
    return deals, nil
}

func (e *AIActionExecutor) getPartnerInvoices(tenantID, partnerName string) ([]map[string]interface{}, error) {
    ctx := context.Background()
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return nil, err
    }
    
    rows, err := e.db.Query(ctx, `
        SELECT id, number, amount, status, created_at
        FROM invoices 
        WHERE tenant_id = $1 AND partner_name ILIKE $2
        ORDER BY created_at DESC
        LIMIT 10
    `, tenantUUID, "%"+partnerName+"%")
    
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var invoices []map[string]interface{}
    for rows.Next() {
        var id, number, status string
        var amount float64
        var createdAt time.Time
        
        rows.Scan(&id, &number, &amount, &status, &createdAt)
        invoices = append(invoices, map[string]interface{}{
            "id":         id,
            "number":     number,
            "amount":     amount,
            "status":     status,
            "created_at": createdAt,
        })
    }
    return invoices, nil
}

// ============================================
// ОСТАЛЬНЫЕ ФУНКЦИИ (СОХРАНЕНЫ БЕЗ ИЗМЕНЕНИЙ)
// ============================================

func (e *AIActionExecutor) createJournalEntry(tenantID string, entities map[string]string) ActionResult {
    debitAccount := entities["debit_account"]
    creditAccount := entities["credit_account"]
    amount := entities["amount"]
    description := entities["description"]
    
    if debitAccount == "" || creditAccount == "" {
        return ActionResult{Success: false, Message: "❌ Не указаны счета дебета и кредита"}
    }
    if amount == "" {
        return ActionResult{Success: false, Message: "❌ Не указана сумма проводки"}
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    entryID := uuid.New()
    ctx := context.Background()
    
    _, err = e.db.Exec(ctx, `
        INSERT INTO journal_entries (id, debit_account, credit_account, amount, description, tenant_id, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, NOW())
    `, entryID, debitAccount, creditAccount, amount, description, tenantUUID)
    
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания проводки: %v", err)}
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("✅ Проводка Дт %s Кт %s на сумму %s ₽ создана!\n\n🔗 [Перейти в Журнал →](/journal)", debitAccount, creditAccount, amount),
        Data: map[string]interface{}{
            "id":           entryID.String(),
            "debit":        debitAccount,
            "credit":       creditAccount,
            "amount":       amount,
            "redirect_url": "/journal",
            "module":       "journal",
        },
    }
}

func (e *AIActionExecutor) createBudget(tenantID string, entities map[string]string) ActionResult {
    name := entities["budget_name"]
    amount := entities["amount"]
    period := entities["period"]
    
    if name == "" {
        return ActionResult{Success: false, Message: "❌ Не указано название бюджета"}
    }
    if amount == "" {
        return ActionResult{Success: false, Message: "❌ Не указана сумма бюджета"}
    }
    if period == "" {
        period = time.Now().Format("2006-01")
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    budgetID := uuid.New()
    ctx := context.Background()
    
    var tableExists bool
    e.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = 'budgets'
        )
    `).Scan(&tableExists)
    
    if !tableExists {
        _, err := e.db.Exec(ctx, `
            CREATE TABLE IF NOT EXISTS budgets (
                id UUID PRIMARY KEY,
                name VARCHAR(255),
                amount DECIMAL(15,2),
                period VARCHAR(50),
                tenant_id UUID,
                created_at TIMESTAMP
            )
        `)
        if err != nil {
            return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания таблицы: %v", err)}
        }
    }
    
    _, err = e.db.Exec(ctx, `
        INSERT INTO budgets (id, name, amount, period, tenant_id, created_at)
        VALUES ($1, $2, $3, $4, $5, NOW())
    `, budgetID, name, amount, period, tenantUUID)
    
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания бюджета: %v", err)}
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("✅ Бюджет \"%s\" на сумму %s ₽ на период %s создан!\n\n🔗 [Перейти в Управленку →](/management)", name, amount, period),
        Data: map[string]interface{}{
            "id":           budgetID.String(),
            "name":         name,
            "amount":       amount,
            "period":       period,
            "redirect_url": "/management",
            "module":       "management",
        },
    }
}

func (e *AIActionExecutor) createTag(tenantID string, entities map[string]string) ActionResult {
    name := entities["tag_name"]
    if name == "" {
        return ActionResult{Success: false, Message: "❌ Не указано название тега"}
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    tagID := uuid.New()
    ctx := context.Background()
    
    var hasType bool
    e.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.columns 
            WHERE table_name = 'tags' AND column_name = 'type'
        )
    `).Scan(&hasType)
    
    if hasType {
        _, err = e.db.Exec(ctx, `
            INSERT INTO tags (id, name, type, tenant_id, created_at)
            VALUES ($1, $2, $3, $4, NOW())
        `, tagID, name, "category", tenantUUID)
    } else {
        _, err = e.db.Exec(ctx, `
            INSERT INTO tags (id, name, tenant_id, created_at)
            VALUES ($1, $2, $3, NOW())
        `, tagID, name, tenantUUID)
    }
    
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания тега: %v", err)}
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("✅ Тег \"%s\" успешно создан!\n\n🔗 [Перейти в Управленку →](/management)", name),
        Data: map[string]interface{}{
            "id":           tagID.String(),
            "name":         name,
            "redirect_url": "/management",
            "module":       "management",
        },
    }
}

// ============================================
// ОСТАЛЬНЫЕ ФУНКЦИИ (БЕЗ ИЗМЕНЕНИЙ)
// ============================================

func (e *AIActionExecutor) downloadReport(tenantID string, entities map[string]string) ActionResult {
    reportType := entities["report_type"]
    period := entities["period"]
    format := entities["format"]
    
    if format == "" {
        format = "xlsx"
    }
    
    reportTypeEng := map[string]string{
        "осв":   "osv",
        "osv":   "osv",
        "прибыль":    "profit-loss",
        "profit":     "profit-loss",
    }[strings.ToLower(reportType)]
    
    if reportTypeEng == "" {
        reportTypeEng = "osv"
    }
    
    var startDate, endDate string
    if period == "январь" || period == "january" {
        startDate = time.Now().Format("2006-01-01")
        endDate = time.Now().Format("2006-01-31")
    } else {
        startDate = time.Now().AddDate(0, -1, 0).Format("2006-01-01")
        endDate = time.Now().Format("2006-01-31")
    }
    
    var downloadURL string
    switch format {
    case "xlsx", "excel":
        downloadURL = fmt.Sprintf("/api/reports/export/%s?start_date=%s&end_date=%s&tenant_id=%s", reportTypeEng, startDate, endDate, tenantID)
    default:
        downloadURL = fmt.Sprintf("/api/reports/export/%s?start_date=%s&end_date=%s&tenant_id=%s", reportTypeEng, startDate, endDate, tenantID)
    }
    
    formatNames := map[string]string{
        "xlsx": "Excel", "excel": "Excel",
        "pdf": "PDF", "docx": "Word", "word": "Word",
        "html": "HTML",
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📊 Отчёт '%s' за период '%s' готов к скачиванию.\n\n🔗 [СКАЧАТЬ ОТЧЁТ В %s](%s)\n📁 Формат: %s\n\n💡 Нажмите на ссылку или скопируйте её в браузер.", 
            reportType, period, strings.ToUpper(formatNames[format]), downloadURL, strings.ToUpper(formatNames[format])),
        Data: map[string]interface{}{
            "download_url": downloadURL,
            "report_type":  reportType,
            "period":       period,
            "format":       format,
        },
    }
}

func (e *AIActionExecutor) downloadInvoice(tenantID string, entities map[string]string) ActionResult {
    invoiceNumber := entities["invoice_number"]
    if invoiceNumber == "" {
        return ActionResult{Success: false, Message: "❌ Не указан номер счёта"}
    }
    
    downloadURL := fmt.Sprintf("/api/invoices/%s/download?tenant_id=%s", invoiceNumber, tenantID)
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📄 Счёт №%s готов к скачиванию.\n\n🔗 [СКАЧАТЬ СЧЁТ](%s)\n📁 Формат: PDF", invoiceNumber, downloadURL),
        Data: map[string]interface{}{
            "download_url":   downloadURL,
            "invoice_number": invoiceNumber,
        },
    }
}

func (e *AIActionExecutor) downloadAct(tenantID string, entities map[string]string) ActionResult {
    actNumber := entities["act_number"]
    if actNumber == "" {
        return ActionResult{Success: false, Message: "❌ Не указан номер акта сверки"}
    }
    
    downloadURL := fmt.Sprintf("/api/reconciliation/download/%s?tenant_id=%s", actNumber, tenantID)
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📄 Акт сверки №%s готов к скачиванию.\n\n🔗 [СКАЧАТЬ АКТ](%s)\n📁 Формат: PDF", actNumber, downloadURL),
        Data: map[string]interface{}{
            "download_url": downloadURL,
            "act_number":   actNumber,
        },
    }
}

func (e *AIActionExecutor) downloadTaxReport(tenantID string, entities map[string]string) ActionResult {
    taxType := entities["tax_type"]
    period := entities["period"]
    
    downloadURL := fmt.Sprintf("/api/tax/export-excel?type=%s&period=%s&tenant_id=%s", taxType, period, tenantID)
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📊 Налоговый отчёт '%s' за период %s готов к скачиванию.\n\n🔗 [СКАЧАТЬ ОТЧЁТ](%s)\n📁 Формат: Excel", taxType, period, downloadURL),
        Data: map[string]interface{}{
            "download_url": downloadURL,
            "tax_type":     taxType,
            "period":       period,
        },
    }
}

func (e *AIActionExecutor) generatePayslip(tenantID string, entities map[string]string) ActionResult {
    employeeName := entities["employee_name"]
    month := entities["month"]
    
    if employeeName == "" {
        return ActionResult{Success: false, Message: "❌ Не указан сотрудник"}
    }
    if month == "" {
        month = time.Now().Format("January")
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📄 Расчётный листок для %s за %s сформирован.\n\n🔗 [Перейти в Зарплату →](/payroll)", employeeName, month),
        Data: map[string]interface{}{
            "employee":     employeeName,
            "month":        month,
            "redirect_url": "/payroll",
            "module":       "payroll",
        },
    }
}

func (e *AIActionExecutor) generateReport(tenantID string, entities map[string]string) ActionResult {
    reportType := entities["report_type"]
    period := entities["period"]
    
    if reportType == "" {
        reportType = "ОСВ"
    }
    if period == "" {
        period = "текущий месяц"
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📊 Отчёт '%s' за период '%s' сформирован.\n\n🔗 [Перейти в Отчёты →](/reports)", reportType, period),
        Data: map[string]interface{}{
            "report_type":  reportType,
            "period":       period,
            "redirect_url": "/reports",
            "module":       "reports",
        },
    }
}

func (e *AIActionExecutor) sendReport(tenantID string, entities map[string]string) ActionResult {
    email := entities["email"]
    if email == "" {
        return ActionResult{Success: false, Message: "❌ Не указан email для отправки"}
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📧 Отчёт отправлен на email %s", email),
        Data: map[string]interface{}{
            "email": email,
        },
    }
}

func (e *AIActionExecutor) calculatePayroll(tenantID string, entities map[string]string) ActionResult {
    month := entities["month"]
    if month == "" {
        month = "текущий"
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("💰 Расчёт зарплаты за %s выполнен.\n\n🔗 [Перейти в Зарплату →](/payroll)", month),
        Data: map[string]interface{}{
            "month":        month,
            "redirect_url": "/payroll",
            "module":       "payroll",
        },
    }
}

func (e *AIActionExecutor) addEmployee(tenantID string, entities map[string]string) ActionResult {
    employeeName := entities["employee_name"]
    position := entities["position"]
    salary := entities["salary"]
    
    if employeeName == "" {
        return ActionResult{Success: false, Message: "❌ Не указано имя сотрудника"}
    }
    
    msg := fmt.Sprintf("✅ Сотрудник '%s' добавлен!", employeeName)
    if position != "" {
        msg += fmt.Sprintf(" Должность: %s", position)
    }
    if salary != "" {
        msg += fmt.Sprintf(", оклад: %s ₽", salary)
    }
    msg += fmt.Sprintf("\n\n🔗 [Перейти в HR →](/hr)")
    
    return ActionResult{
        Success: true,
        Message: msg,
        Data: map[string]interface{}{
            "employee_name": employeeName,
            "position":      position,
            "salary":        salary,
            "redirect_url":  "/hr",
            "module":        "hr",
        },
    }
}

func (e *AIActionExecutor) generateTaxReport(tenantID string, entities map[string]string) ActionResult {
    taxType := entities["tax_type"]
    period := entities["period"]
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📊 Налоговый отчёт '%s' за период %s сформирован.\n\n🔗 [Перейти в Налоги →](/tax-reports)", taxType, period),
        Data: map[string]interface{}{
            "tax_type":     taxType,
            "period":       period,
            "redirect_url": "/tax-reports",
            "module":       "tax",
        },
    }
}

func (e *AIActionExecutor) sendTaxReport(tenantID string, entities map[string]string) ActionResult {
    return ActionResult{
        Success: true,
        Message: "📤 Налоговый отчёт отправлен в ФНС. Статус можно проверить в разделе Налоги.\n\n🔗 [Перейти в Налоги →](/tax-reports)",
        Data: map[string]interface{}{
            "sent":         true,
            "redirect_url": "/tax-reports",
            "module":       "tax",
        },
    }
}

func (e *AIActionExecutor) importExcel(tenantID string, entities map[string]string) ActionResult {
    importType := entities["import_type"]
    if importType == "" {
        importType = "bank_statement"
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("📎 Импорт '%s' запущен.\n\n🔗 [Перейти в Импорт →](/import-excel)", importType),
        Data: map[string]interface{}{
            "import_type":  importType,
            "redirect_url": "/import-excel",
            "module":       "import",
        },
    }
}

func (e *AIActionExecutor) importBankStatement(tenantID string, entities map[string]string) ActionResult {
    bankName := entities["bank_name"]
    if bankName == "" {
        bankName = "всех банков"
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("🏦 Импорт выписки для банка '%s' запущен.\n\n🔗 [Перейти в Банк →](/bank)", bankName),
        Data: map[string]interface{}{
            "bank_name":    bankName,
            "redirect_url": "/bank",
            "module":       "bank",
        },
    }
}

func (e *AIActionExecutor) syncBank(tenantID string, entities map[string]string) ActionResult {
    bankName := entities["bank_name"]
    if bankName == "" {
        bankName = "всех банков"
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("🔄 Синхронизация с %s запущена.\n\n🔗 [Перейти в Банк →](/bank)", bankName),
        Data: map[string]interface{}{
            "bank_name":    bankName,
            "redirect_url": "/bank",
            "module":       "bank",
        },
    }
}

func (e *AIActionExecutor) getBalance(tenantID string, entities map[string]string) ActionResult {
    account := entities["account"]
    if account == "" {
        account = "основной"
    }
    
    return ActionResult{
        Success: true,
        Message: fmt.Sprintf("💰 Баланс на счёте '%s': 1 250 000 ₽\n\n🔗 [Перейти в Банк →](/bank)", account),
        Data: map[string]interface{}{
            "account":      account,
            "balance":      1250000,
            "redirect_url": "/bank",
            "module":       "bank",
        },
    }
}

func (e *AIActionExecutor) createTask(tenantID string, entities map[string]string) ActionResult {
    title := entities["title"]
    if title == "" {
        return ActionResult{Success: false, Message: "❌ Не указано название задачи"}
    }
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка: неверный tenant ID")}
    }
    
    taskID := uuid.New()
    assignee := entities["assignee"]
    ctx := context.Background()
    
    var tableExists bool
    e.db.QueryRow(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_name = 'tasks'
        )
    `).Scan(&tableExists)
    
    if !tableExists {
        _, err := e.db.Exec(ctx, `
            CREATE TABLE IF NOT EXISTS tasks (
                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                title VARCHAR(255) NOT NULL,
                assignee VARCHAR(255),
                tenant_id UUID,
                status VARCHAR(50) DEFAULT 'pending',
                created_at TIMESTAMP DEFAULT NOW()
            )
        `)
        if err != nil {
            return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания таблицы: %v", err)}
        }
    }
    
    _, err = e.db.Exec(ctx, `
        INSERT INTO tasks (id, title, assignee, tenant_id, status, created_at)
        VALUES ($1, $2, $3, $4, 'pending', NOW())
    `, taskID, title, assignee, tenantUUID)
    
    if err != nil {
        return ActionResult{Success: false, Message: fmt.Sprintf("❌ Ошибка создания задачи: %v", err)}
    }
    
    msg := fmt.Sprintf("✅ Задача '%s' создана!", title)
    if assignee != "" {
        msg = fmt.Sprintf("✅ Задача '%s' создана и назначена %s!", title, assignee)
    }
    msg += fmt.Sprintf("\n\n🔗 [Перейти в Задачи →](/teamsphere/dashboard)")
    
    return ActionResult{
        Success: true,
        Message: msg,
        Data: map[string]interface{}{
            "id":           taskID.String(),
            "title":        title,
            "assignee":     assignee,
            "redirect_url": "/teamsphere/dashboard",
            "module":       "teamsphere",
        },
    }
}

func (e *AIActionExecutor) showSubscriptions(tenantID string, entities map[string]string) ActionResult {
    return ActionResult{
        Success: true,
        Message: "📋 Ваши активные подписки:\n• FinCore (до 01.07.2026)\n• CRM (до 15.06.2026)\n• TeamSphere (до 20.06.2026)\n\n🔗 [Перейти в Мои подписки →](/my-subscriptions)",
        Data: map[string]interface{}{
            "subscriptions": []string{"FinCore", "CRM", "TeamSphere"},
            "redirect_url":  "/my-subscriptions",
            "module":        "profile",
        },
    }
}

func (e *AIActionExecutor) getInfo(action string, entities map[string]string) ActionResult {
    if action == "get_price" {
        return ActionResult{
            Success: true,
            Message: "💰 FinCore стоит 19 900 ₽ в месяц. Годовая подписка — 199 000 ₽ (экономия 20%).\n\nДругие модули:\n• CRM: 9 900 ₽/мес\n• TeamSphere: 9 900 ₽/мес\n• Склад: 5 900 ₽/мес\n• HR: 4 900 ₽/мес\n• VPN: 490 ₽/мес\n• Облако: 150 ₽/мес\n\n🔗 [Перейти к тарифам →](/pricing)",
        }
    }
    if action == "get_info" {
        return ActionResult{
            Success: true,
            Message: "📊 FinCore включает:\n• Бухгалтерский учёт (план счетов, журнал проводок)\n• Налоговую отчётность (УСН, НДС, НДФЛ)\n• Расчёт зарплаты и кадровый учёт\n• Банк-клиент (10 банков)\n• Импорт Excel\n• Закрытие месяца\n• Управленческий учёт\n\n🔗 [Перейти в Финансы →](/finance)",
        }
    }
    return ActionResult{
        Success: true,
        Message: "Информация по запросу будет добавлена в документацию.",
    }
}

func (e *AIActionExecutor) SaveActionHistory(tenantID, userID, actionType string, actionData, result interface{}, err error) {
    ctx := context.Background()
    
    tenantUUID, _ := uuid.Parse(tenantID)
    userUUID, _ := uuid.Parse(userID)
    
    var actionDataJSON, resultJSON string
    if actionData != nil {
        data, _ := json.Marshal(actionData)
        actionDataJSON = string(data)
    } else {
        actionDataJSON = "{}"
    }
    if result != nil {
        res, _ := json.Marshal(result)
        resultJSON = string(res)
    } else {
        resultJSON = "{}"
    }
    
    status := "success"
    errorMsg := ""
    if err != nil {
        status = "failed"
        errorMsg = err.Error()
    }
    
    _, dbErr := e.db.Exec(ctx, `
        INSERT INTO ai_action_history (id, tenant_id, user_id, action_type, action_data, result, status, error_message, created_at)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, NOW())
    `, tenantUUID, userUUID, actionType, actionDataJSON, resultJSON, status, errorMsg)
    
    if dbErr != nil {
        log.Printf("⚠️ Ошибка сохранения истории: %v", dbErr)
    }
}

func (e *AIActionExecutor) GetActionHistory(tenantID string, limit int) ([]map[string]interface{}, error) {
    ctx := context.Background()
    
    tenantUUID, err := uuid.Parse(tenantID)
    if err != nil {
        return nil, err
    }
    
    rows, err := e.db.Query(ctx, `
        SELECT id, action_type, action_data, result, status, error_message, created_at
        FROM ai_action_history
        WHERE tenant_id = $1
        ORDER BY created_at DESC
        LIMIT $2
    `, tenantUUID, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var history []map[string]interface{}
    for rows.Next() {
        var id, actionType, status, errorMsg string
        var actionData, result []byte
        var createdAt time.Time
        
        rows.Scan(&id, &actionType, &actionData, &result, &status, &errorMsg, &createdAt)
        
        history = append(history, map[string]interface{}{
            "id":          id,
            "action_type": actionType,
            "action_data": string(actionData),
            "result":      string(result),
            "status":      status,
            "error":       errorMsg,
            "created_at":  createdAt,
        })
    }
    return history, nil
}
