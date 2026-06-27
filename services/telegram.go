package services

import (
    "bytes"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "time"
)

type TelegramService struct {
    BotToken string
    ChatID   string
    Name     string
}

var (
    ClientBot *TelegramService
    AdminBot  *TelegramService
)

func InitTelegramServices() {
    // Клиентский бот
    clientToken := os.Getenv("TELEGRAM_BOT_TOKEN")
    clientChatID := os.Getenv("TELEGRAM_CHAT_ID")
    
    if clientToken != "" && clientChatID != "" {
        ClientBot = &TelegramService{
            BotToken: clientToken,
            ChatID:   clientChatID,
            Name:     "ClientBot",
        }
        log.Println("✅ Клиентский Telegram бот инициализирован")
    }
    
    // Админский бот (личный)
    adminToken := os.Getenv("ADMIN_BOT_TOKEN")
    adminChatID := os.Getenv("ADMIN_CHAT_ID")
    
    if adminToken != "" && adminChatID != "" {
        AdminBot = &TelegramService{
            BotToken: adminToken,
            ChatID:   adminChatID,
            Name:     "AdminBot",
        }
        log.Println("✅ Админский Telegram бот инициализирован")
    }
}

func (t *TelegramService) SendMessage(text string) error {
    if t == nil {
        return fmt.Errorf("telegram service not initialized")
    }
    
    url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)
    
    data := map[string]interface{}{
        "chat_id":    t.ChatID,
        "text":       text,
        "parse_mode": "HTML",
    }
    
    jsonData, err := json.Marshal(data)
    if err != nil {
        return err
    }
    
    resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("telegram error: status %d", resp.StatusCode)
    }
    
    return nil
}

// ========== УВЕДОМЛЕНИЯ ДЛЯ АДМИНА ==========

func NotifyAdminModuleRequest(moduleName, userName, userEmail, message string) {
    if AdminBot == nil {
        log.Printf("⚠️ AdminBot не инициализирован")
        return
    }
    
    text := fmt.Sprintf(`
🔔 <b>НОВАЯ ЗАЯВКА НА МОДУЛЬ!</b>

📦 <b>Модуль:</b> %s
👤 <b>Пользователь:</b> %s
📧 <b>Email:</b> %s
📝 <b>Сообщение:</b> %s
🕐 <b>Время:</b> %s

<a href="http://localhost:8080/developer/admin">👉 Перейти в админ-панель</a>
`,
        moduleName,
        userName,
        userEmail,
        message,
        time.Now().Format("15:04:05 02.01.2006"),
    )
    
    if err := AdminBot.SendMessage(text); err != nil {
        log.Printf("❌ Ошибка отправки уведомления админу: %v", err)
    }
}

func NotifyAdminNewUser(email, name, phone string) {
    if AdminBot == nil {
        return
    }
    
    text := fmt.Sprintf(`
🆕 <b>НОВЫЙ ПОЛЬЗОВАТЕЛЬ!</b>

👤 <b>Имя:</b> %s
📧 <b>Email:</b> %s
📞 <b>Телефон:</b> %s
🕐 <b>Время:</b> %s

<a href="http://localhost:8080/admin/users">👉 Перейти к пользователям</a>
`,
        name,
        email,
        phone,
        time.Now().Format("15:04:05 02.01.2006"),
    )
    
    AdminBot.SendMessage(text)
}

func NotifyAdminPayment(userEmail string, amount float64, paymentMethod, orderInfo string) {
    if AdminBot == nil {
        return
    }
    
    text := fmt.Sprintf(`
💰 <b>НОВЫЙ ПЛАТЕЖ!</b>

👤 <b>Пользователь:</b> %s
💵 <b>Сумма:</b> %.2f ₽
💳 <b>Способ:</b> %s
📦 <b>Заказ:</b> %s
🕐 <b>Время:</b> %s

<a href="http://localhost:8080/admin/payments">👉 Перейти к платежам</a>
`,
        userEmail,
        amount,
        paymentMethod,
        orderInfo,
        time.Now().Format("15:04:05 02.01.2006"),
    )
    
    AdminBot.SendMessage(text)
}

func NotifyAdminServiceOrder(orderName, contact, description string) {
    if AdminBot == nil {
        return
    }
    
    text := fmt.Sprintf(`
📋 <b>НОВАЯ ЗАЯВКА НА УСЛУГУ!</b>

👤 <b>Имя:</b> %s
📞 <b>Контакт:</b> %s
📝 <b>Описание:</b> %s
🕐 <b>Время:</b> %s

<a href="http://localhost:8080/admin/orders-view">👉 Перейти к заявкам</a>
`,
        orderName,
        contact,
        description,
        time.Now().Format("15:04:05 02.01.2006"),
    )
    
    AdminBot.SendMessage(text)
}

func NotifyAdminError(errorMsg string) {
    if AdminBot == nil {
        return
    }
    
    text := fmt.Sprintf(`
🚨 <b>ОШИБКА НА СЕРВЕРЕ!</b>

❌ <b>Ошибка:</b> %s
🕐 <b>Время:</b> %s

⚠️ Требуется внимание!
`,
        errorMsg,
        time.Now().Format("15:04:05 02.01.2006"),
    )
    
    AdminBot.SendMessage(text)
}