package services

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type TelegramNotifier struct {
    botToken string
    chatID   string
    adminID  string
    client   *http.Client
}

func NewTelegramNotifier(botToken, chatID, adminID string) *TelegramNotifier {
    return &TelegramNotifier{
        botToken: botToken,
        chatID:   chatID,
        adminID:  adminID,
        client:   &http.Client{Timeout: 10 * time.Second},
    }
}

func (t *TelegramNotifier) SendOrderNotification(order interface{}) error {
    if t.botToken == "" || t.chatID == "" {
        return nil
    }
    
    message := "🆕 **Новая заявка на разработку!**\n\n📋 Проверьте админ-панель для деталей."
    return t.sendMessage(t.chatID, message)
}

func (t *TelegramNotifier) sendMessage(chatID, text string) error {
    apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
    
    body := map[string]interface{}{
        "chat_id":    chatID,
        "text":       text,
        "parse_mode": "Markdown",
    }
    
    jsonBody, _ := json.Marshal(body)
    resp, err := t.client.Post(apiURL, "application/json", bytes.NewBuffer(jsonBody))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    return nil
}

func (t *TelegramNotifier) SendTestMessage() error {
    if t.botToken == "" || t.chatID == "" {
        return nil
    }
    message := "✅ **Тестовое сообщение от Business Stack**\n\nВаш Telegram бот успешно настроен!"
    return t.sendMessage(t.chatID, message)
}
