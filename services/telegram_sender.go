package services

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "time"
)

type TelegramSender struct {
    botToken string
    chatID   string
    adminID  string
    client   *http.Client
}

func NewTelegramSender(botToken, chatID, adminID string) *TelegramSender {
    fmt.Println("========================================")
    fmt.Println("📢 ИНИЦИАЛИЗАЦИЯ TELEGRAM SENDER")
    fmt.Println("========================================")
    
    client := &http.Client{
        Timeout: 30 * time.Second,
    }
    
    return &TelegramSender{
        botToken: botToken,
        chatID:   chatID,
        adminID:  adminID,
        client:   client,
    }
}

func (t *TelegramSender) SendOrder(orderData map[string]string) error {
    fmt.Println("📨 Отправка заявки...")
    
    // Сохраняем в файл всегда (для надёжности)
    t.saveToFile(orderData)
    
    // Пытаемся отправить в Telegram
    if t.botToken != "" && t.chatID != "" {
        message := fmt.Sprintf(`🆕 **НОВАЯ ЗАЯВКА НА РАЗРАБОТКУ**

👤 **Клиент:** %s
📞 **Контакт:** %s
📋 **Услуга:** %s
⏰ **Срок:** %s
💰 **Бюджет:** %s
📦 **Дополнительно:** %s

---
📅 %s`,
            orderData["name"],
            orderData["contact"],
            orderData["service"],
            orderData["deadline"],
            orderData["price"],
            orderData["additional"],
            time.Now().Format("02.01.2006 15:04:05"))
        
        go t.sendToTelegram(message)
    }
    
    return nil
}

func (t *TelegramSender) sendToTelegram(message string) {
    apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
    
    body := map[string]interface{}{
        "chat_id":    t.chatID,
        "text":       message,
        "parse_mode": "Markdown",
    }
    
    jsonBody, _ := json.Marshal(body)
    
    req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
    if err != nil {
        return
    }
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := t.client.Do(req)
    if err != nil {
        fmt.Printf("⚠️ Telegram не доступен: %v\n", err)
        return
    }
    defer resp.Body.Close()
    
    if resp.StatusCode == 200 {
        fmt.Println("✅ Заявка отправлена в Telegram!")
    } else {
        fmt.Printf("⚠️ Ошибка Telegram: %d\n", resp.StatusCode)
    }
}

func (t *TelegramSender) saveToFile(orderData map[string]string) {
    os.MkdirAll("orders", 0755)
    
    filename := fmt.Sprintf("orders/заявка_%s.txt", time.Now().Format("20060102_150405"))
    
    content := fmt.Sprintf(`НОВАЯ ЗАЯВКА
================
Клиент: %s
Контакт: %s
Услуга: %s
Срок: %s
Бюджет: %s
Дополнительно: %s
Дата: %s
================`,
        orderData["name"],
        orderData["contact"],
        orderData["service"],
        orderData["deadline"],
        orderData["price"],
        orderData["additional"],
        time.Now().Format("02.01.2006 15:04:05"))
    
    os.WriteFile(filename, []byte(content), 0644)
    fmt.Printf("📁 Заявка сохранена: %s\n", filename)
}

func (t *TelegramSender) SendTestMessage() error {
    if t.botToken == "" {
        return fmt.Errorf("telegram not configured")
    }
    
    testMsg := "✅ Тестовое сообщение от Business Stack!\n\nБот работает и готов принимать заявки!"
    
    apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)
    body := map[string]interface{}{
        "chat_id":    t.chatID,
        "text":       testMsg,
        "parse_mode": "Markdown",
    }
    jsonBody, _ := json.Marshal(body)
    
    resp, err := t.client.Post(apiURL, "application/json", bytes.NewBuffer(jsonBody))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("telegram API error: %d", resp.StatusCode)
    }
    
    return nil
}
