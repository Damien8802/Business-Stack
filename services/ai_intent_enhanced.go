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
    originalMessage := message
    message = strings.ToLower(message)
    entities := make(map[string]string)

    // ============================================
    // 0. ПРОВЕРКА НА ПРОСТОЙ ЧАТ (без команд)
    // ============================================
    chatPatterns := []string{
        "привет", "здравствуй", "доброе утро", "добрый день", "добрый вечер",
        "как дела", "как ты", "что нового", "как жизнь", "как настроение",
        "расскажи", "шутку", "анекдот", "смешное", "поговорим", "давай поболтаем",
        "кто ты", "что ты умеешь", "как тебя зовут",
    }
    
    for _, pattern := range chatPatterns {
        if strings.Contains(message, pattern) {
            return IntentExtended{
                Action: "chat",
                Type:   "general",
                Module: "general",
            }, entities
        }
    }
    
    // Короткие сообщения без явных команд - тоже чат
    words := strings.Fields(message)
    if len(words) <= 3 {
        hasCommand := strings.Contains(message, "создай") || 
                      strings.Contains(message, "покажи") ||
                      strings.Contains(message, "сделки") ||
                      strings.Contains(message, "платеж") ||
                      strings.Contains(message, "платёж") ||
                      strings.Contains(message, "архивируй") ||
                      strings.Contains(message, "восстанови") ||
                      strings.Contains(message, "закрой") ||
                      strings.Contains(message, "рассчитай") ||
                      strings.Contains(message, "экспортируй")
        if !hasCommand {
            return IntentExtended{
                Action: "chat",
                Type:   "general",
                Module: "general",
            }, entities
        }
    }

    // ============================================
    // ФИНАНСЫ - СОЗДАНИЕ ПЛАТЕЖА
    // ============================================
    if strings.Contains(message, "создай плат") || strings.Contains(message, "создать плат") {
        // Пытаемся извлечь получателя и сумму из полной фразы
        if match := regexp.MustCompile(`создай\s+плат[её]ж\s+для\s+([А-Яа-яA-Za-z0-9\s\.\-]+)\s+на\s+сумму\s+(\d+)`).FindStringSubmatch(originalMessage); len(match) > 2 {
            entities["recipient"] = strings.TrimSpace(match[1])
            entities["amount"] = match[2]
        } else if match := regexp.MustCompile(`создай\s+плат[её]ж\s+([А-Яа-яA-Za-z0-9\s\.\-]+)\s+(\d+)`).FindStringSubmatch(originalMessage); len(match) > 2 {
            entities["recipient"] = strings.TrimSpace(match[1])
            entities["amount"] = match[2]
        }
        
        return IntentExtended{
            Type:       "fincore",
            Action:     "create_payment",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ФИНАНСЫ - АРХИВАЦИЯ СЧЕТА
    // ============================================
    if match := regexp.MustCompile(`(архивируй|удали|перемести)\s+счёт\s+(\d+)`).FindStringSubmatch(message); len(match) > 2 {
        entities["account_code"] = strings.TrimSpace(match[2])
        return IntentExtended{
            Type:       "fincore",
            Action:     "archive_account",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ФИНАНСЫ - ВОССТАНОВЛЕНИЕ СЧЕТА
    // ============================================
    if match := regexp.MustCompile(`(восстанови|верни)\s+счёт\s+(\d+)`).FindStringSubmatch(message); len(match) > 2 {
        entities["account_code"] = strings.TrimSpace(match[2])
        return IntentExtended{
            Type:       "fincore",
            Action:     "restore_account",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ФИНАНСЫ - ПОКАЗАТЬ АРХИВ
    // ============================================
    if strings.Contains(message, "покажи архив счетов") || strings.Contains(message, "архив счетов") {
        return IntentExtended{
            Type:       "fincore",
            Action:     "show_archive",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ФИНАНСЫ - ПОЛУЧИТЬ ОСВ
    // ============================================
    if strings.Contains(message, "покажи осв") || strings.Contains(message, "осв за") {
        if match := regexp.MustCompile(`осв\s+за\s+(.+)$`).FindStringSubmatch(originalMessage); len(match) > 1 {
            entities["period"] = strings.TrimSpace(match[1])
        }
        return IntentExtended{
            Type:       "fincore",
            Action:     "get_osv",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ФИНАНСЫ - БУХГАЛТЕРСКИЙ БАЛАНС
    // ============================================
    if strings.Contains(message, "бухгалтерский баланс") || strings.Contains(message, "покажи баланс") {
        if match := regexp.MustCompile(`баланс\s+на\s+(.+)$`).FindStringSubmatch(originalMessage); len(match) > 1 {
            entities["date"] = strings.TrimSpace(match[1])
        }
        return IntentExtended{
            Type:       "fincore",
            Action:     "get_balance_sheet",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ФИНАНСЫ - ЭКСПОРТ В EXCEL
    // ============================================
    if strings.Contains(message, "экспортируй осв") || strings.Contains(message, "экспорт осв") {
        entities["report_type"] = "ОСВ"
        return IntentExtended{
            Type:       "fincore",
            Action:     "export_to_excel",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ЗАКРЫТИЕ МЕСЯЦА
    // ============================================
    if strings.Contains(message, "закрой месяц") || strings.Contains(message, "закрыть месяц") {
        return IntentExtended{
            Type:       "fincore",
            Action:     "close_month",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ПОКАЗАТЬ ИНФОРМАЦИЮ О ПАРТНЁРЕ
    // ============================================
    if match := regexp.MustCompile(`покажи\s+информацию\s+о\s+([А-Яа-яA-Za-z0-9\s\.\-]+)`).FindStringSubmatch(originalMessage); len(match) > 1 {
        entities["partner_name"] = strings.TrimSpace(match[1])
        return IntentExtended{
            Type:       "crm",
            Action:     "show_partner_info",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ПОКАЗАТЬ СДЕЛКИ ПАРТНЁРА
    // ============================================
    if match := regexp.MustCompile(`покажи\s+сделки\s+([А-Яа-яA-Za-z0-9\s\.\-]+)`).FindStringSubmatch(originalMessage); len(match) > 1 {
        entities["partner_name"] = strings.TrimSpace(match[1])
        return IntentExtended{
            Type:       "crm",
            Action:     "show_partner_deals",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // СОЗДАНИЕ ПАРТНЁРА
    // ============================================
    if match := regexp.MustCompile(`создай\s+партн[её]ра\s+(.+)$`).FindStringSubmatch(originalMessage); len(match) > 1 {
        entities["name"] = strings.TrimSpace(match[1])
        return IntentExtended{
            Type:       "crm",
            Action:     "create_partner",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // СОЗДАНИЕ СДЕЛКИ
    // ============================================
    if match := regexp.MustCompile(`создай\s+сделку\s+для\s+([А-Яа-яA-Za-z0-9\s\.\-]+)\s+на\s+(\d+)`).FindStringSubmatch(originalMessage); len(match) > 2 {
        entities["customer_name"] = strings.TrimSpace(match[1])
        entities["amount"] = match[2]
        return IntentExtended{
            Type:       "crm",
            Action:     "create_deal",
            Module:     "crm",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // АКТ СВЕРКИ
    // ============================================
    if strings.Contains(message, "акт сверки") {
        if match := regexp.MustCompile(`акт сверки\s+с\s+([А-Яа-яA-Za-z0-9\s\.\-]+)`).FindStringSubmatch(originalMessage); len(match) > 1 {
            entities["partner_name"] = strings.TrimSpace(match[1])
        }
        return IntentExtended{
            Type:       "fincore",
            Action:     "create_act",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // СОЗДАНИЕ ЗАДАЧИ
    // ============================================
    if strings.Contains(message, "создай задачу") {
        if match := regexp.MustCompile(`создай задачу\s+([А-Яа-я0-9\s]+?)(?:\s+для\s+|\s+исполнителю\s+)([А-Яа-я\s]+)$`).FindStringSubmatch(originalMessage); len(match) > 2 {
            entities["title"] = strings.TrimSpace(match[1])
            entities["assignee"] = strings.TrimSpace(match[2])
        } else if match := regexp.MustCompile(`создай задачу\s+([А-Яа-я0-9\s]+)$`).FindStringSubmatch(originalMessage); len(match) > 1 {
            entities["title"] = strings.TrimSpace(match[1])
        }
        return IntentExtended{
            Type:       "teamsphere",
            Action:     "create_task",
            Module:     "teamsphere",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // РАСЧЁТ ЗАРПЛАТЫ
    // ============================================
    if strings.Contains(message, "рассчитай зарплату") {
        if match := regexp.MustCompile(`зарплату\s+за\s+([А-Яа-я]+)`).FindStringSubmatch(originalMessage); len(match) > 1 {
            entities["month"] = match[1]
        }
        return IntentExtended{
            Type:       "payroll",
            Action:     "calculate_payroll",
            Module:     "payroll",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // СКАЧАТЬ ОТЧЁТ
    // ============================================
    if strings.Contains(message, "скачай отчёт") || strings.Contains(message, "скачать отчет") {
        entities["report_type"] = "осв"
        if strings.Contains(message, "excel") {
            entities["format"] = "excel"
        }
        if strings.Contains(message, "январь") {
            entities["period"] = "январь"
        }
        return IntentExtended{
            Type:       "reports",
            Action:     "download_report",
            Module:     "reports",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // СФОРМИРОВАТЬ ОСВ
    // ============================================
    if strings.Contains(message, "сформируй осв") {
        entities["report_type"] = "ОСВ"
        if match := regexp.MustCompile(`осв\s+за\s+(.+)$`).FindStringSubmatch(originalMessage); len(match) > 1 {
            entities["period"] = match[1]
        }
        return IntentExtended{
            Type:       "reports",
            Action:     "generate_report",
            Module:     "reports",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ПОМОЩЬ
    // ============================================
    if strings.Contains(message, "помощь") || strings.Contains(message, "что умеешь") || strings.Contains(message, "help") || message == "?" {
        return IntentExtended{
            Type:       "general",
            Action:     "help",
            Module:     "general",
            Confidence: 1.0,
        }, entities
    }

    // ============================================
    // ЦЕНА
    // ============================================
    if strings.Contains(message, "сколько стоит") || strings.Contains(message, "цена") {
        return IntentExtended{
            Type:       "info",
            Action:     "get_price",
            Module:     "fincore",
            Confidence: 0.95,
        }, entities
    }

    // ============================================
    // ПО УМОЛЧАНИЮ - ОБЫЧНЫЙ ЧАТ
    // ============================================
    return IntentExtended{
        Type:       "general",
        Action:     "chat",
        Module:     "general",
        Confidence: 0.5,
    }, entities
}

func GetHelpMessage() string {
    return `🤖 **Я - AI ассистент Business Stack**

Вот что я умею:

💰 **Финансы:**
• "создай платёж для ООО Ромашка на сумму 50000"
• "покажи ОСВ за январь"
• "покажи бухгалтерский баланс"
• "закрой месяц"

👥 **CRM:**
• "создай партнёра ООО Ромашка +7 999 123-45-67"
• "создай сделку для Ромашки на 500000"
• "покажи информацию о Ромашка"

✅ **Задачи:**
• "создай задачу Позвонить клиенту для Иванова"

Просто спроси меня о чём угодно!`
}

// DetectIntent - определяет намерение
func DetectIntent(message string) IntentExtended {
    intent, _ := AnalyzeIntentExtended(message)
    return intent
}

// ExtractEntities - извлекает сущности
func ExtractEntities(message string) map[string]string {
    _, entities := AnalyzeIntentExtended(message)
    return entities
}