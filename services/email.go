package services

import (
    "fmt"
    "net/smtp"
    "subscription-system/config"
)

type EmailService struct {
    Host     string
    Port     int
    User     string
    Password string
    From     string
    FromName string
    TLS      bool
}

var emailService *EmailService

func InitEmailService(cfg *config.Config) {
    emailService = &EmailService{
        Host:     cfg.SMTPHost,
        Port:     cfg.SMTPPort,
        User:     cfg.SMTPUser,
        Password: cfg.SMTPPassword,
        From:     cfg.SMTPFrom,
        FromName: cfg.SMTPFromName,
        TLS:      cfg.SMTPTLS,
    }
}

func SendEmail(to, subject, body string) error {
    if emailService == nil {
        return fmt.Errorf("email service not initialized")
    }

    auth := smtp.PlainAuth("", emailService.User, emailService.Password, emailService.Host)
    addr := fmt.Sprintf("%s:%d", emailService.Host, emailService.Port)

    htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><style>
body { font-family: Arial, sans-serif; }
.container { max-width: 600px; margin: 0 auto; padding: 20px; }
.header { background: linear-gradient(135deg, #667eea, #764ba2); color: white; padding: 20px; text-align: center; border-radius: 10px 10px 0 0; }
.content { background: #f9f9f9; padding: 20px; border-radius: 0 0 10px 10px; }
.field { margin-bottom: 10px; }
.label { font-weight: bold; color: #333; }
.value { color: #666; }
.footer { text-align: center; padding: 20px; font-size: 12px; color: #999; }
</style></head>
<body>
<div class="container">
<div class="header"><h2>%s</h2></div>
<div class="content">%s</div>
<div class="footer">Business Stack CRM • Автоматическое уведомление</div>
</div>
</body>
</html>`, subject, body)

    msg := []byte("To: " + to + "\r\n" +
        "From: " + emailService.From + "\r\n" +
        "Subject: " + subject + "\r\n" +
        "Content-Type: text/html; charset=UTF-8\r\n" +
        "\r\n" +
        htmlBody + "\r\n")

    err := smtp.SendMail(addr, auth, emailService.From, []string{to}, msg)
    if err != nil {
        return fmt.Errorf("ошибка отправки email: %w", err)
    }
    return nil
}
