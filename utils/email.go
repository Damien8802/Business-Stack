package utils

import (
    "fmt"
    "net/smtp"
    "time"
    "subscription-system/config"
)

type EmailService struct {
    config *config.Config
}

func NewEmailService(cfg *config.Config) *EmailService {
    return &EmailService{config: cfg}
}

// SendEmail отправляет email через SMTP
func (s *EmailService) SendEmail(to, subject, body string) error {
    if s.config.SMTPHost == "" || s.config.SMTPUser == "" {
        return fmt.Errorf("SMTP not configured")
    }

    auth := smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPassword, s.config.SMTPHost)
    
    msg := []byte(fmt.Sprintf("To: %s\r\n"+
        "Subject: %s\r\n"+
        "Content-Type: text/html; charset=utf-8\r\n"+
        "\r\n"+
        "%s\r\n", to, subject, body))

    addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)
    return smtp.SendMail(addr, auth, s.config.EmailFrom, []string{to}, msg)
}

// SendSecurityAlert отправляет уведомление о безопасности
func (s *EmailService) SendSecurityAlert(to, username, alertType string, details map[string]string) error {
    subject := fmt.Sprintf("🔐 Уведомление безопасности - SaaSPro")
    
    body := fmt.Sprintf(`
        <h2>Уведомление безопасности</h2>
        <p>Здравствуйте, <strong>%s</strong>!</p>
        <p>Тип события: <strong>%s</strong></p>
        <table border="1" cellpadding="5" style="border-collapse: collapse;">
    `, username, alertType)
    
    for key, value := range details {
        body += fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>", key, value)
    }
    
    body += `
        </table>
        <p>Если это были не вы, немедленно смените пароль.</p>
        <p>С уважением,<br>Команда SaaSPro</p>
    `
    
    return s.SendEmail(to, subject, body)
}

// SendLoginNotification уведомление о входе
func (s *EmailService) SendLoginNotification(to, username, ip, location, device string) error {
    details := map[string]string{
        "IP адрес":        ip,
        "Местоположение": location,
        "Устройство":     device,
        "Время":          time.Now().Format("02.01.2006 15:04:05"),
    }
    return s.SendSecurityAlert(to, username, "Новый вход в аккаунт", details)
}

// Send2FANotification уведомление о 2FA
func (s *EmailService) Send2FANotification(to, username, action string) error {
    details := map[string]string{
        "Действие": action,
        "Время":    time.Now().Format("02.01.2006 15:04:05"),
    }
    return s.SendSecurityAlert(to, username, "Изменение 2FA", details)
}

// SendVerificationEmail отправляет код подтверждения
func (s *EmailService) SendVerificationEmail(to, name, code string) error {
    subject := "🔐 Подтверждение регистрации - SaaSPro"
    
    body := fmt.Sprintf(`
        <h2>Добро пожаловать в SaaSPro!</h2>
        <p>Здравствуйте, <strong>%s</strong>!</p>
        <p>Ваш код подтверждения:</p>
        <h1 style="font-size: 32px; letter-spacing: 5px; background: #f0f0f0; padding: 10px; text-align: center;">%s</h1>
        <p>Код действителен в течение 15 минут.</p>
        <p>Если вы не регистрировались на нашем сайте, проигнорируйте это письмо.</p>
        <p>С уважением,<br>Команда SaaSPro</p>
    `, name, code)
    
    return s.SendEmail(to, subject, body)
}

// SendVerificationLink отправляет письмо со ссылкой для подтверждения email
func (s *EmailService) SendVerificationLink(to, name, link string) error {
    subject := "Подтверждение регистрации — SaaSPro"
    
    body := fmt.Sprintf(`
        <h2>Добро пожаловать в SaaSPro, %s!</h2>
        <p>Для подтверждения email перейдите по ссылке:</p>
        <p><a href="%s" style="background: #4f46e5; color: white; padding: 10px 20px; text-decoration: none; border-radius: 8px;">Подтвердить email</a></p>
        <p>Ссылка действительна 24 часа.</p>
        <p>Если вы не регистрировались — проигнорируйте письмо.</p>
    `, name, link)
    
    return s.SendEmail(to, subject, body)
}