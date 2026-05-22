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
    subject := fmt.Sprintf("🔐 Уведомление безопасности - Business Stack")
    
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
        <p>С уважением,<br>Команда Business Stack</p>
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
    subject := "🔐 Подтверждение регистрации - Business Stack"
    
    body := fmt.Sprintf(`
        <h2>Добро пожаловать в Business Stack!</h2>
        <p>Здравствуйте, <strong>%s</strong>!</p>
        <p>Ваш код подтверждения:</p>
        <h1 style="font-size: 32px; letter-spacing: 5px; background: #f0f0f0; padding: 10px; text-align: center;">%s</h1>
        <p>Код действителен в течение 15 минут.</p>
        <p>Если вы не регистрировались на нашем сайте, проигнорируйте это письмо.</p>
        <p>С уважением,<br>Команда Business Stack</p>
    `, name, code)
    
    return s.SendEmail(to, subject, body)
}

// SendVerificationLink отправляет письмо со ссылкой для подтверждения email
func (s *EmailService) SendVerificationLink(to, name, link string) error {
    subject := "✅ Подтверждение регистрации — Business Stack"
    
    body := fmt.Sprintf(`
        <!DOCTYPE html>
        <html>
        <head>
            <meta charset="UTF-8">
            <style>
                body {
                    font-family: Arial, sans-serif;
                    background: #f5f5f5;
                    padding: 40px;
                    margin: 0;
                }
                .container {
                    max-width: 550px;
                    margin: 0 auto;
                    background: white;
                    border-radius: 16px;
                    overflow: hidden;
                    box-shadow: 0 4px 20px rgba(0,0,0,0.1);
                }
                .header {
                    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
                    padding: 30px;
                    text-align: center;
                }
                .header h1 {
                    color: white;
                    margin: 0;
                    font-size: 28px;
                }
                .header p {
                    color: rgba(255,255,255,0.8);
                    margin: 5px 0 0;
                }
                .content {
                    padding: 30px;
                }
                .greeting {
                    font-size: 16px;
                    color: #333;
                    margin-bottom: 20px;
                }
                .button {
                    text-align: center;
                    margin: 30px 0;
                }
                .btn {
                    display: inline-block;
                    padding: 14px 35px;
                    background: linear-gradient(135deg, #10b981, #059669);
                    color: white;
                    text-decoration: none;
                    border-radius: 50px;
                    font-weight: 600;
                    font-size: 16px;
                }
                .warning {
                    background: #fef3c7;
                    padding: 15px;
                    border-radius: 8px;
                    margin: 20px 0;
                    font-size: 13px;
                    color: #92400e;
                    text-align: center;
                }
                .footer {
                    text-align: center;
                    padding: 20px;
                    background: #f8f9fa;
                    font-size: 12px;
                    color: #999;
                    border-top: 1px solid #eee;
                }
                .footer a {
                    color: #667eea;
                    text-decoration: none;
                }
            </style>
        </head>
        <body>
            <div class="container">
                <div class="header">
                    <h1>🚀 Business Stack</h1>
                    <p>Платформа управления подписками</p>
                </div>
                <div class="content">
                    <div class="greeting">
                        <strong>Здравствуйте, %s!</strong>
                    </div>
                    <p>Спасибо за регистрацию на платформе <strong>Business Stack</strong>!</p>
                    <p>Для завершения регистрации, пожалуйста, подтвердите ваш email адрес:</p>
                    <div class="button">
                        <a href="%s" class="btn">✅ Подтвердить регистрацию</a>
                    </div>
                    <div class="warning">
                        <strong>⚠️ Внимание!</strong><br>
                        Если вы не подтвердите email в течение 24 часов, регистрация будет автоматически отменена.
                    </div>
                    <p style="font-size: 13px; color: #666; text-align: center;">
                        Если вы не регистрировались на Business Stack, просто проигнорируйте это письмо.
                    </p>
                </div>
                <div class="footer">
                    <p>© 2025 Business Stack. Все права защищены.</p>
                    <p>Вопросы? Напишите нам: <a href="mailto:dev@businesstack.ru">dev@businesstack.ru</a></p>
                </div>
            </div>
        </body>
        </html>
    `, name, link)
    
    return s.SendEmail(to, subject, body)
}

// SendPasswordResetEmail отправляет письмо для восстановления пароля
func (s *EmailService) SendPasswordResetEmail(to, name, resetLink string) error {
    subject := "🔐 Восстановление пароля - Business Stack"
    
    body := fmt.Sprintf(`
        <!DOCTYPE html>
        <html>
        <head>
            <meta charset="UTF-8">
        </head>
        <body style="font-family: Arial, sans-serif; background: #f5f5f5; padding: 40px;">
            <div style="max-width: 500px; margin: 0 auto; background: white; border-radius: 16px; overflow: hidden; box-shadow: 0 4px 20px rgba(0,0,0,0.1);">
                <div style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 30px; text-align: center;">
                    <h1 style="color: white; margin: 0; font-size: 24px;">Business Stack</h1>
                    <p style="color: rgba(255,255,255,0.8); margin: 5px 0 0;">Восстановление пароля</p>
                </div>
                <div style="padding: 30px;">
                    <p>Здравствуйте, <strong>%s</strong>!</p>
                    <p>Вы запросили восстановление пароля на платформе Business Stack.</p>
                    <p>Для установки нового пароля нажмите на кнопку ниже:</p>
                    <div style="text-align: center; margin: 30px 0;">
                        <a href="%s" style="display: inline-block; padding: 12px 30px; background: linear-gradient(135deg, #667eea, #764ba2); color: white; text-decoration: none; border-radius: 8px; font-weight: 600;">Сбросить пароль</a>
                    </div>
                    <p style="font-size: 14px; color: #666;">Ссылка действительна в течение <strong>24 часов</strong>. Если вы не запрашивали восстановление пароля, просто проигнорируйте это письмо.</p>
                    <hr style="margin: 20px 0; border: none; border-top: 1px solid #eee;">
                    <p style="font-size: 12px; color: #999; text-align: center;">© 2025 Business Stack. Все права защищены.</p>
                </div>
            </div>
        </body>
        </html>
    `, name, resetLink)
    
    return s.SendEmail(to, subject, body)
}

// SendAdminNotification - уведомление админу о новом пользователе
func (s *EmailService) SendAdminNotification(userName, userEmail string) error {
    subject := "🆕 Новый пользователь зарегистрировался!"
    
    body := fmt.Sprintf(`
        <!DOCTYPE html>
        <html>
        <head>
            <meta charset="UTF-8">
        </head>
        <body style="font-family: 'Segoe UI', Arial, sans-serif; background: #f5f5f5; padding: 40px;">
            <div style="max-width: 500px; margin: 0 auto; background: white; border-radius: 16px; overflow: hidden; box-shadow: 0 4px 20px rgba(0,0,0,0.1);">
                <div style="background: linear-gradient(135deg, #10b981 0%, #059669 100%); padding: 30px; text-align: center;">
                    <h1 style="color: white; margin: 0; font-size: 24px;">🆕 Новый пользователь!</h1>
                </div>
                <div style="padding: 30px;">
                    <p>Здравствуйте!</p>
                    <p>На платформе Business Stack зарегистрировался новый пользователь:</p>
                    <table style="width: 100%%; margin: 20px 0; border-collapse: collapse;">
                        <tr style="background: #f8f9fa;">
                            <td style="padding: 12px; border: 1px solid #ddd;"><strong>Имя:</strong></td>
                            <td style="padding: 12px; border: 1px solid #ddd;">%s</td>
                        </tr>
                        <tr>
                            <td style="padding: 12px; border: 1px solid #ddd;"><strong>Email:</strong></td>
                            <td style="padding: 12px; border: 1px solid #ddd;">%s</td>
                        </tr>
                        <tr style="background: #f8f9fa;">
                            <td style="padding: 12px; border: 1px solid #ddd;"><strong>Дата:</strong></td>
                            <td style="padding: 12px; border: 1px solid #ddd;">%s</td>
                        </tr>
                    </table>
                    <hr style="margin: 20px 0; border: none; border-top: 1px solid #eee;">
                    <p style="font-size: 12px; color: #999; text-align: center;">© 2025 Business Stack</p>
                </div>
            </div>
        </body>
        </html>
    `, userName, userEmail, time.Now().Format("02.01.2006 15:04:05"))
    
    return s.SendEmail(s.config.EmailFrom, subject, body)
}

// SendNewOrderNotification - уведомление о новой заявке
func (s *EmailService) SendNewOrderNotification(orderName, orderContact, orderDescription string) error {
    subject := "📦 Новая заявка на разработку!"
    
    body := fmt.Sprintf(`
        <!DOCTYPE html>
        <html>
        <head>
            <meta charset="UTF-8">
        </head>
        <body style="font-family: 'Segoe UI', Arial, sans-serif; background: #f5f5f5; padding: 40px;">
            <div style="max-width: 500px; margin: 0 auto; background: white; border-radius: 16px; overflow: hidden; box-shadow: 0 4px 20px rgba(0,0,0,0.1);">
                <div style="background: linear-gradient(135deg, #f59e0b 0%, #ea580c 100%); padding: 30px; text-align: center;">
                    <h1 style="color: white; margin: 0; font-size: 24px;">📦 Новая заявка!</h1>
                </div>
                <div style="padding: 30px;">
                    <p><strong>Клиент:</strong> %s</p>
                    <p><strong>Контакт:</strong> %s</p>
                    <p><strong>Описание:</strong></p>
                    <p style="background: #f8f9fa; padding: 15px; border-radius: 8px;">%s</p>
                    <hr style="margin: 20px 0;">
                    <p style="font-size: 12px; color: #999;">Перейдите в админ-панель для обработки</p>
                </div>
            </div>
        </body>
        </html>
    `, orderName, orderContact, orderDescription)
    
    return s.SendEmail(s.config.EmailFrom, subject, body)
}

// SendNewIdeaNotification - уведомление о новой идее
func (s *EmailService) SendNewIdeaNotification(title, description, userEmail string) error {
    subject := "💡 Новая идея от пользователя!"
    
    body := fmt.Sprintf(`
        <!DOCTYPE html>
        <html>
        <head>
            <meta charset="UTF-8">
        </head>
        <body style="font-family: 'Segoe UI', Arial, sans-serif; background: #f5f5f5; padding: 40px;">
            <div style="max-width: 500px; margin: 0 auto; background: white; border-radius: 16px; overflow: hidden; box-shadow: 0 4px 20px rgba(0,0,0,0.1);">
                <div style="background: linear-gradient(135deg, #8b5cf6 0%, #6d28d9 100%); padding: 30px; text-align: center;">
                    <h1 style="color: white; margin: 0; font-size: 24px;">💡 Новая идея!</h1>
                </div>
                <div style="padding: 30px;">
                    <p><strong>От:</strong> %s</p>
                    <p><strong>Название:</strong> %s</p>
                    <p><strong>Описание:</strong></p>
                    <p style="background: #f8f9fa; padding: 15px; border-radius: 8px;">%s</p>
                </div>
            </div>
        </body>
        </html>
    `, userEmail, title, description)
    
    return s.SendEmail(s.config.EmailFrom, subject, body)
}

