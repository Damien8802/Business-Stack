package services

import (
    "fmt"
    "regexp"
    "strings"
)

type IntentExtended struct {
    Type            string
    Action          string
    Module          string
    RequiresPayment bool
    Entities        map[string]string
    Confidence      float64
}

func AnalyzeIntentExtended(message string) (IntentExtended, map[string]string) {
    fmt.Println("🔍 [DEBUG] Анализ сообщения:", message)
    message = strings.ToLower(message)
    entities := make(map[string]string)

    // ============================================
    // 0. СОЗДАНИЕ ЗАДАЧИ (САМЫЙ ПЕРВЫЙ!)
    // ============================================
    if strings.Contains(message, "создай задачу") {
        parts := strings.SplitN(message, "создай задачу", 2)
        if len(parts) > 1 {
            rest := strings.TrimSpace(parts[1])
            if idx := strings.Index(rest, " для "); idx != -1 {
                entities["title"] = strings.TrimSpace(rest[:idx])
                entities["assignee"] = strings.TrimSpace(rest[idx+5:])
            } else if idx := strings.Index(rest, " исполнителю "); idx != -1 {
                entities["title"] = strings.TrimSpace(rest[:idx])
                entities["assignee"] = strings.TrimSpace(rest[idx+12:])
            } else {
                entities["title"] = rest
                entities["assignee"] = ""
            }
            return IntentExtended{
                Type:       "teamsphere",
                Action:     "create_task",
                Module:     "teamsphere",
                Confidence: 0.95,
            }, entities
        }
    }

    // ============================================
    // 1. ПОКАЗАТЬ ИНФОРМАЦИЮ О ПАРТНЁРЕ
    // ============================================
    if match := regexp.MustCompile(`(покажи|найди|информация)\s+(о|про|об)\s+([А-Яа-яA-Za-z0-9\s\.\-]+)`).FindStringSubmatch(message); len(match) > 3 {
        entities["partner_name"] = strings.TrimSpace(match[3])
        return IntentExtended{
            Type:       "crm",
            Action:     "show_partner_info",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 2. ПОКАЗАТЬ СДЕЛКИ ПАРТНЁРА
    // ============================================
    if match := regexp.MustCompile(`(покажи|список)\s+сделк[и]?\s+([А-Яа-яA-Za-z0-9\s\.\-]+)`).FindStringSubmatch(message); len(match) > 2 {
        entities["partner_name"] = strings.TrimSpace(match[2])
        return IntentExtended{
            Type:       "crm",
            Action:     "show_partner_deals",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 3. ЗАКРЫТИЕ МЕСЯЦА
    // ============================================
    if match := regexp.MustCompile(`(закрой|закрыть)\s+месяц`).FindString(message); match != "" {
        return IntentExtended{
            Type:       "fincore",
            Action:     "close_month",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 4. СОЗДАНИЕ ПАРТНЁРА / КЛИЕНТА
    // ============================================
    if match := regexp.MustCompile(`(создай|добавь|новый|нового)\s+(партн[её]р[а]?|клиент[а]?)\s+([А-Яа-яA-Za-z0-9\s\.\-]+)`).FindStringSubmatch(message); len(match) > 3 {
        entities["name"] = strings.TrimSpace(match[3])
        
        phoneRe := regexp.MustCompile(`(\+?7[\d\s\-\(\)]{10,})`)
        if phoneMatch := phoneRe.FindStringSubmatch(message); len(phoneMatch) > 0 {
            entities["phone"] = strings.TrimSpace(phoneMatch[0])
        }
        
        emailRe := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
        if emailMatch := emailRe.FindStringSubmatch(message); len(emailMatch) > 0 {
            entities["email"] = emailMatch[0]
        }
        
        return IntentExtended{
            Type:       "crm",
            Action:     "create_partner",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 5. СОЗДАНИЕ СДЕЛКИ
    // ============================================
    if match := regexp.MustCompile(`создай\s+сделк[уа]\s+["']?([А-Яа-яA-Za-z0-9\s\.\-]+)["']?\s+(для|с)\s+([А-Яа-яA-Za-z0-9\s\.\-]+)\s+на\s+(\d+[\s]?\d*)`).FindStringSubmatch(message); len(match) > 4 {
        entities["deal_name"] = strings.TrimSpace(match[1])
        entities["customer_name"] = strings.TrimSpace(match[3])
        amount := strings.ReplaceAll(match[4], " ", "")
        entities["amount"] = amount
        return IntentExtended{
            Type:       "crm",
            Action:     "create_deal",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }
    
    if match := regexp.MustCompile(`создай\s+сделк[уа]\s+(для|с)\s+([А-Яа-яA-Za-z0-9\s\.\-]+)\s+на\s+(\d+[\s]?\d*)`).FindStringSubmatch(message); len(match) > 3 {
        entities["customer_name"] = strings.TrimSpace(match[2])
        amount := strings.ReplaceAll(match[3], " ", "")
        entities["amount"] = amount
        entities["deal_name"] = fmt.Sprintf("Сделка с %s на %s ₽", entities["customer_name"], amount)
        return IntentExtended{
            Type:       "crm",
            Action:     "create_deal",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }
    
    if match := regexp.MustCompile(`создай\s+сделк[уа]\s+на\s+(\d+[\s]?\d*)\s+(для|с)\s+([А-Яа-яA-Za-z0-9\s\.\-]+)`).FindStringSubmatch(message); len(match) > 3 {
        amount := strings.ReplaceAll(match[1], " ", "")
        entities["amount"] = amount
        entities["customer_name"] = strings.TrimSpace(match[3])
        entities["deal_name"] = fmt.Sprintf("Сделка с %s на %s ₽", entities["customer_name"], amount)
        return IntentExtended{
            Type:       "crm",
            Action:     "create_deal",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 6. ВЫСТАВЛЕНИЕ СЧЁТА
    // ============================================
    if match := regexp.MustCompile(`(выстав[ьи]|создай)\s+счёт\s+([А-Яа-яA-Za-z0-9\s\.\-]+)\s+на\s+(\d+[\s]?\d*)`).FindStringSubmatch(message); len(match) > 3 {
        entities["partner_name"] = strings.TrimSpace(match[2])
        amount := strings.ReplaceAll(match[3], " ", "")
        entities["amount"] = amount
        return IntentExtended{
            Type:       "fincore",
            Action:     "create_invoice",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 7. СОЗДАНИЕ ПЛАТЕЖА
    // ============================================
    if match := regexp.MustCompile(`(создай|сделай)\s+плат[её]ж\s+(для|на|в)\s+([А-Яа-яA-Za-z0-9\s\.\-]+)\s+на\s+(\d+[\s]?\d*)`).FindStringSubmatch(message); len(match) > 4 {
        entities["recipient"] = strings.TrimSpace(match[3])
        amount := strings.ReplaceAll(match[4], " ", "")
        entities["amount"] = amount
        return IntentExtended{
            Type:       "fincore",
            Action:     "create_payment",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 8. СОЗДАНИЕ АКТА СВЕРКИ
    // ============================================
    if match := regexp.MustCompile(`(создай|сделай)\s+акт\s+сверк[и]\s+(с|для)\s+([А-Яа-яA-Za-z0-9\s\.\-]+)`).FindStringSubmatch(message); len(match) > 3 {
        entities["partner_name"] = strings.TrimSpace(match[3])
        return IntentExtended{
            Type:       "reconciliation",
            Action:     "create_act",
            Module:     "reconciliation",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 9. ФОРМИРОВАНИЕ ОТЧЁТА
    // ============================================
    if match := regexp.MustCompile(`(сформируй|покажи|сделай)\s+(отч[её]т|ОСВ)\s+(за|по)\s+([А-Яа-я0-9\s\.\-]+)`).FindStringSubmatch(message); len(match) > 3 {
        entities["report_type"] = strings.TrimSpace(match[2])
        entities["period"] = strings.TrimSpace(match[4])
        return IntentExtended{
            Type:       "reports",
            Action:     "generate_report",
            Module:     "reports",
            Confidence: 0.9,
        }, entities
    }

    // ============================================
    // 10. ОТПРАВКА ОТЧЁТА
    // ============================================
    if match := regexp.MustCompile(`(отправь|пошли)\s+отч[её]т\s+на\s+email\s+([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`).FindStringSubmatch(message); len(match) > 2 {
        entities["email"] = match[2]
        return IntentExtended{
            Type:       "reports",
            Action:     "send_report",
            Module:     "reports",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 11. РАСЧЁТ ЗАРПЛАТЫ
    // ============================================
    if match := regexp.MustCompile(`(рассчитай|сделай)\s+зарплат[уы]\s+за\s+([А-Яа-я0-9]+)`).FindStringSubmatch(message); len(match) > 2 {
        entities["month"] = strings.TrimSpace(match[2])
        return IntentExtended{
            Type:       "payroll",
            Action:     "calculate_payroll",
            Module:     "payroll",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 12. ДОБАВЛЕНИЕ СОТРУДНИКА
    // ============================================
    if match := regexp.MustCompile(`добав[ьи]\s+сотрудник[а]?\s+([А-Яа-яA-Za-z\s\.\-]+)\s+на\s+должност[ьи]\s+([А-Яа-яA-Za-z\s\.\-]+)\s+с\s+оклад[оа]м\s+(\d+[\s]?\d*)`).FindStringSubmatch(message); len(match) > 3 {
        entities["employee_name"] = strings.TrimSpace(match[1])
        entities["position"] = strings.TrimSpace(match[2])
        salary := strings.ReplaceAll(match[3], " ", "")
        entities["salary"] = salary
        return IntentExtended{
            Type:       "hr",
            Action:     "add_employee",
            Module:     "hr",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 13. ПОКАЗ ПОДПИСОК
    // ============================================
    if match := regexp.MustCompile(`покажи\s+мои\s+подписки`).FindString(message); match != "" {
        return IntentExtended{
            Type:       "profile",
            Action:     "show_subscriptions",
            Module:     "profile",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 14. СКАЧИВАНИЕ ФАЙЛОВ
    // ============================================
    if match := regexp.MustCompile(`(скачай|скачать|download)\s+отч[её]т\s+([А-Яа-яA-Za-z0-9]+)\s+за\s+([А-Яа-я0-9\s\.\-]+)(?:\s+в\s+)?(\w+)?`).FindStringSubmatch(message); len(match) > 3 {
        entities["report_type"] = strings.TrimSpace(match[2])
        entities["period"] = strings.TrimSpace(match[3])
        format := "xlsx"
        if len(match) > 4 && match[4] != "" {
            format = strings.ToLower(match[4])
        }
        entities["format"] = format
        return IntentExtended{
            Type:       "reports",
            Action:     "download_report",
            Module:     "reports",
            Confidence: 0.95,
        }, entities
    }

    if match := regexp.MustCompile(`(скачай|скачать|download)\s+счёт\s+([A-Za-z0-9\-]+)(?:\s+в\s+)?(\w+)?`).FindStringSubmatch(message); len(match) > 2 {
        entities["invoice_number"] = strings.TrimSpace(match[2])
        format := "pdf"
        if len(match) > 3 && match[3] != "" {
            format = strings.ToLower(match[3])
        }
        entities["format"] = format
        return IntentExtended{
            Type:       "fincore",
            Action:     "download_invoice",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    if match := regexp.MustCompile(`(скачай|скачать|download)\s+акт\s+([A-Za-z0-9\-]+)(?:\s+в\s+)?(\w+)?`).FindStringSubmatch(message); len(match) > 2 {
        entities["act_number"] = strings.TrimSpace(match[2])
        format := "pdf"
        if len(match) > 3 && match[3] != "" {
            format = strings.ToLower(match[3])
        }
        entities["format"] = format
        return IntentExtended{
            Type:       "reconciliation",
            Action:     "download_act",
            Module:     "reconciliation",
            Confidence: 0.95,
        }, entities
    }

    if match := regexp.MustCompile(`(скачай|скачать|download)\s+расчётный\s+листок\s+для\s+([А-Яа-яA-Za-z0-9\s\.\-]+)\s+за\s+([А-Яа-я0-9\s\.\-]+)`).FindStringSubmatch(message); len(match) > 3 {
        entities["employee_name"] = strings.TrimSpace(match[2])
        entities["month"] = strings.TrimSpace(match[3])
        return IntentExtended{
            Type:       "payroll",
            Action:     "generate_payslip",
            Module:     "payroll",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 15. ПРОВОДКИ (ЖУРНАЛ)
    // ============================================
    if match := regexp.MustCompile(`создай\s+проводк[уа]\s+дт\s+(\d+)\s+кт\s+(\d+)\s+на\s+(\d+[\s]?\d*)`).FindStringSubmatch(message); len(match) > 3 {
        entities["debit_account"] = match[1]
        entities["credit_account"] = match[2]
        amount := strings.ReplaceAll(match[3], " ", "")
        entities["amount"] = amount
        return IntentExtended{
            Type:       "fincore",
            Action:     "create_journal_entry",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 16. БЮДЖЕТ
    // ============================================
    if match := regexp.MustCompile(`создай\s+бюджет\s+([А-Яа-яA-Za-z0-9\s\.\-]+)\s+на\s+(\d+[\s]?\d*)\s+(на\s+)?([А-Яа-я0-9\s\.\-]+)`).FindStringSubmatch(message); len(match) > 3 {
        entities["budget_name"] = strings.TrimSpace(match[1])
        amount := strings.ReplaceAll(match[2], " ", "")
        entities["amount"] = amount
        entities["period"] = strings.TrimSpace(match[4])
        return IntentExtended{
            Type:       "fincore",
            Action:     "create_budget",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 17. ТЕГИ
    // ============================================
    if match := regexp.MustCompile(`создай\s+тег\s+([А-Яа-яA-Za-z0-9\s\.\-]+)`).FindStringSubmatch(message); len(match) > 1 {
        entities["tag_name"] = strings.TrimSpace(match[1])
        return IntentExtended{
            Type:       "fincore",
            Action:     "create_tag",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 18. ИМПОРТ
    // ============================================
    if match := regexp.MustCompile(`(импорт(ируй|ировать))\s+(выписку|банка|excel)`).FindString(message); match != "" {
        return IntentExtended{
            Type:       "import",
            Action:     "import_excel",
            Module:     "import",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 19. БАНК
    // ============================================
    if match := regexp.MustCompile(`синхронизируй\s+банк`).FindString(message); match != "" {
        return IntentExtended{
            Type:       "bank",
            Action:     "sync_bank",
            Module:     "bank",
            Confidence: 0.95,
        }, entities
    }

    if match := regexp.MustCompile(`покажи\s+баланс`).FindString(message); match != "" {
        return IntentExtended{
            Type:       "bank",
            Action:     "get_balance",
            Module:     "bank",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 20. ВОПРОСЫ О ЦЕНАХ
    // ============================================
    if match := regexp.MustCompile(`(сколько стоит|цена|стоимость)\s+(fincore|финкор|финансы)`).FindString(message); match != "" {
        return IntentExtended{
            Type:       "info",
            Action:     "get_price",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }
    
    if match := regexp.MustCompile(`(что входит|функции|описание)\s+(fincore|финкор|финансы)`).FindString(message); match != "" {
        return IntentExtended{
            Type:       "info",
            Action:     "get_info",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // 21. ПОМОЩЬ
    // ============================================
    if match := regexp.MustCompile(`(помощь|что умеешь|как использовать|help|команды)`).FindString(message); match != "" {
        return IntentExtended{
            Type:       "general",
            Action:     "help",
            Module:     "general",
            Confidence: 1.0,
        }, entities
    }

    // ============================================
    // 22. ПО УМОЛЧАНИЮ
    // ============================================
    return IntentExtended{
        Type:       "general",
        Action:     "chat",
        Module:     "general",
        Confidence: 0.5,
    }, entities
}

func GetHelpMessage() string {
    return `🤖 **Я - автономный AI-ассистент Business Stack**

✅ **Я САМ ВЫПОЛНЯЮ команды, а не просто подсказываю!**

📋 **CRM:**
• "Создай партнёра ООО Ромашка +7 999 123-45-67"
• "Создай сделку для Ромашки на 500 000"
• "Покажи информацию о Ромашка"
• "Покажи сделки Ромашка"

💰 **FinCore - Финансы:**
• "Выставь счёт Ромашке на 500 000"
• "Создай платёж для ООО Поставщик на 50 000"
• "Создай проводку Дт 51 Кт 62 на 100 000"
• "Создай бюджет Маркетинг на 100 000 на январь"
• "Создай тег VIP-клиент"
• "Закрой месяц"

📄 **Акты сверки:**
• "Создай акт сверки с ООО Ромашка"

✅ **Задачи:**
• "Создай задачу Позвонить клиенту для Иванова"

📊 **Отчёты (с выбором формата):**
• "Сформируй ОСВ за 1 квартал"
• "Скачай отчёт ОСВ за январь в excel"
• "Скачай отчёт ОСВ за январь в pdf"
• "Скачай отчёт ОСВ за январь в word"
• "Скачай отчёт ОСВ за январь в html"

📎 **Скачивание 파일ов:**
• "Скачай счёт INV-12345 в pdf"
• "Скачай акт ACT-12345 в pdf"
• "Скачай расчётный листок для Иванова за январь"

💰 **Зарплата:**
• "Рассчитай зарплату за май"
• "Добавь сотрудника Иванов Иван на должность Бухгалтер с окладом 50000"

🏦 **Банк:**
• "Синхронизируй банк"
• "Покажи баланс"

📎 **Импорт:**
• "Импортируй выписку из банка"

❓ **Вопросы:**
• "Сколько стоит FinCore?"
• "Что входит в FinCore?"

Что нужно сделать?`
}