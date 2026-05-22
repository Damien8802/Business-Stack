package services

import (
    "bytes"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "net/smtp"
    "subscription-system/config"
)

type NotificationService struct {
    cfg *config.Config
}

func NewNotificationService(cfg *config.Config) *NotificationService {
    return &NotificationService{cfg: cfg}
}

// SendTelegram отправляет сообщение в Telegram через бота
func (ns *NotificationService) SendTelegram(message string) error {
    if ns.cfg.TelegramBotToken == "" || ns.cfg.TelegramChatID == "" {
        log.Println("Telegram не настроен, пропускаем уведомление")
        return nil
    }

    url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", ns.cfg.TelegramBotToken)
    payload := map[string]interface{}{
        "chat_id":    ns.cfg.TelegramChatID,
        "text":       message,
        "parse_mode": "HTML",
    }
    jsonData, _ := json.Marshal(payload)

    resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return fmt.Errorf("ошибка отправки в Telegram: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("Telegram вернул статус %s", resp.Status)
    }
    return nil
}

// SendEmail отправляет email через SMTP
func (ns *NotificationService) SendEmail(to, subject, body string) error {
    if ns.cfg.SMTPHost == "" || ns.cfg.SMTPUser == "" || ns.cfg.EmailFrom == "" {
        log.Println("SMTP не настроен, пропускаем email")
        return nil
    }

    auth := smtp.PlainAuth("", ns.cfg.SMTPUser, ns.cfg.SMTPPassword, ns.cfg.SMTPHost)
    addr := fmt.Sprintf("%s:%d", ns.cfg.SMTPHost, ns.cfg.SMTPPort)

    // HTML письмо с красивым оформлением
    htmlBody := fmt.Sprintf(`
        <!DOCTYPE html>
        <html>
        <head>
            <style>
                body { font-family: Arial, sans-serif; }
                .container { max-width: 600px; margin: 0 auto; padding: 20px; }
                .header { background: linear-gradient(135deg, #667eea, #764ba2); color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
                .content { background: #f9f9f9; padding: 20px; border-radius: 0 0 10px 10px; }
                .field { margin-bottom: 10px; }
                .label { font-weight: bold; color: #333; }
                .value { color: #666; }
                .footer { text-align: center; padding: 20px; font-size: 12px; color: #999; }
            </style>
        </head>
        <body>
            <div class="container">
                <div class="header">
                    <h2>📧 %s</h2>
                </div>
                <div class="content">
                    %s
                </div>
                <div class="footer">
                    Business Stack CRM • Автоматическое уведомление
                </div>
            </div>
        </body>
        </html>
    `, subject, body)

    msg := []byte("To: " + to + "\r\n" +
        "From: " + ns.cfg.EmailFrom + "\r\n" +
        "Subject: " + subject + "\r\n" +
        "Content-Type: text/html; charset=UTF-8\r\n" +
        "\r\n" +
        htmlBody + "\r\n")

    err := smtp.SendMail(addr, auth, ns.cfg.EmailFrom, []string{to}, msg)
    if err != nil {
        return fmt.Errorf("ошибка отправки email: %w", err)
    }
    return nil
}

// NotifyCustomerCreated уведомление о создании клиента
func (ns *NotificationService) NotifyCustomerCreated(name, email, phone, company, responsible string) {
    // Telegram
    msgTelegram := fmt.Sprintf("🆕 Новый клиент создан:\n<b>Имя:</b> %s\n<b>Email:</b> %s\n<b>Телефон:</b> %s\n<b>Компания:</b> %s\n<b>Ответственный:</b> %s",
        name, email, phone, company, responsible)
    ns.SendTelegram(msgTelegram)

    // Email
    msgEmail := fmt.Sprintf(`
        <div class="field">
            <span class="label">👤 Имя:</span>
            <span class="value">%s</span>
        </div>
        <div class="field">
            <span class="label">📧 Email:</span>
            <span class="value">%s</span>
        </div>
        <div class="field">
            <span class="label">📞 Телефон:</span>
            <span class="value">%s</span>
        </div>
        <div class="field">
            <span class="label">🏢 Компания:</span>
            <span class="value">%s</span>
        </div>
        <div class="field">
            <span class="label">👔 Ответственный:</span>
            <span class="value">%s</span>
        </div>
    `, name, email, phone, company, responsible)
    
    go ns.SendEmail(ns.cfg.NotifyEmail, "🆕 Новый клиент в CRM", msgEmail)
}

// NotifyCustomerUpdated уведомление об изменении клиента
func (ns *NotificationService) NotifyCustomerUpdated(id, name, email, phone string) {
    msg := fmt.Sprintf("✏️ Клиент обновлён:\n<b>ID:</b> %s\n<b>Имя:</b> %s\n<b>Email:</b> %s\n<b>Телефон:</b> %s",
        id, name, email, phone)
    ns.SendTelegram(msg)
}

// NotifyDealCreated уведомление о создании сделки
func (ns *NotificationService) NotifyDealCreated(title string, value float64, stage, responsible, customerID string) {
    // Telegram
    msgTelegram := fmt.Sprintf("💰 Новая сделка:\n<b>Название:</b> %s\n<b>Сумма:</b> %.2f\n<b>Стадия:</b> %s\n<b>Ответственный:</b> %s\n<b>Клиент ID:</b> %s",
        title, value, stage, responsible, customerID)
    ns.SendTelegram(msgTelegram)

    // Email
    msgEmail := fmt.Sprintf(`
        <div class="field">
            <span class="label">📋 Название:</span>
            <span class="value">%s</span>
        </div>
        <div class="field">
            <span class="label">💰 Сумма:</span>
            <span class="value">%.2f ₽</span>
        </div>
        <div class="field">
            <span class="label">📊 Стадия:</span>
            <span class="value">%s</span>
        </div>
        <div class="field">
            <span class="label">👔 Ответственный:</span>
            <span class="value">%s</span>
        </div>
        <div class="field">
            <span class="label">🆔 Клиент ID:</span>
            <span class="value">%s</span>
        </div>
    `, title, value, stage, responsible, customerID)
    
    go ns.SendEmail(ns.cfg.NotifyEmail, "💰 Новая сделка в CRM", msgEmail)
}

// NotifyDealUpdated уведомление об изменении сделки
func (ns *NotificationService) NotifyDealUpdated(id, title string, value float64, stage string) {
    msg := fmt.Sprintf("🔄 Сделка обновлена:\n<b>ID:</b> %s\n<b>Название:</b> %s\n<b>Сумма:</b> %.2f\n<b>Стадия:</b> %s",
        id, title, value, stage)
    ns.SendTelegram(msg)
}