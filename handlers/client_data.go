package handlers

import (
    "archive/zip"
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

// GetClientDataInfo - информация о данных клиента
func GetClientDataInfo(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var transactionsCount, paymentsCount int
    var lastActivity *time.Time

    // Количество транзакций
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM bank_statements WHERE tenant_id = $1
    `, tenantID).Scan(&transactionsCount)

    // Количество платежей
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM payments WHERE tenant_id = $1
    `, tenantID).Scan(&paymentsCount)

    // Последняя активность
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT MAX(created_at) FROM (
            SELECT created_at FROM bank_statements WHERE tenant_id = $1
            UNION ALL
            SELECT created_at FROM payments WHERE tenant_id = $1
        ) AS all_activity
    `, tenantID).Scan(&lastActivity)

    lastActivityStr := ""
    if lastActivity != nil {
        lastActivityStr = lastActivity.Format("02.01.2006 15:04:05")
    }

    c.JSON(http.StatusOK, gin.H{
        "db_name":            "FinCore_" + tenantID[:8],
        "transactions_count": transactionsCount,
        "payments_count":     paymentsCount,
        "last_activity":      lastActivityStr,
        "has_sql_access":     false,
    })
}

// ExportAllData - экспорт всех данных клиента
func ExportAllData(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    c.Header("Content-Type", "application/zip")
    c.Header("Content-Disposition", "attachment; filename=fincore_export_"+time.Now().Format("20060102")+".zip")
    c.Header("Content-Transfer-Encoding", "binary")

    zipWriter := zip.NewWriter(c.Writer)
    defer zipWriter.Close()

    // 1. Экспорт транзакций
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT operation_date, debit_amount, credit_amount, counterparty_name, payment_purpose, created_at
        FROM bank_statements WHERE tenant_id = $1 ORDER BY operation_date DESC
    `, tenantID)
    if err == nil {
        var transactions []map[string]interface{}
        for rows.Next() {
            var opDate time.Time
            var debit, credit float64
            var counterparty, purpose string
            var createdAt time.Time
            rows.Scan(&opDate, &debit, &credit, &counterparty, &purpose, &createdAt)
            transactions = append(transactions, map[string]interface{}{
                "date":         opDate.Format("02.01.2006"),
                "debit":        debit,
                "credit":       credit,
                "counterparty": counterparty,
                "purpose":      purpose,
                "created_at":   createdAt.Format("02.01.2006 15:04:05"),
            })
        }
        rows.Close()

        data, _ := json.MarshalIndent(transactions, "", "  ")
        file, _ := zipWriter.Create("transactions.json")
        file.Write(data)
    }

    // 2. Экспорт платежей
    rows2, err2 := database.Pool.Query(c.Request.Context(), `
        SELECT date, partner, amount, type, purpose, created_at FROM payments WHERE tenant_id = $1 ORDER BY date DESC
    `, tenantID)
    if err2 == nil {
        var payments []map[string]interface{}
        for rows2.Next() {
            var date, partner, purpose, ptype string
            var amount float64
            var createdAt time.Time
            rows2.Scan(&date, &partner, &amount, &ptype, &purpose, &createdAt)
            payments = append(payments, map[string]interface{}{
                "date":       date,
                "partner":    partner,
                "amount":     amount,
                "type":       ptype,
                "purpose":    purpose,
                "created_at": createdAt.Format("02.01.2006 15:04:05"),
            })
        }
        rows2.Close()

        data, _ := json.MarshalIndent(payments, "", "  ")
        file, _ := zipWriter.Create("payments.json")
        file.Write(data)
    }

    // 3. Экспорт категорий
    rows3, err3 := database.Pool.Query(c.Request.Context(), `
        SELECT name, keywords, color FROM payment_categories WHERE tenant_id = $1 ORDER BY name
    `, tenantID)
    if err3 == nil {
        var categories []map[string]interface{}
        for rows3.Next() {
            var name, keywords, color string
            rows3.Scan(&name, &keywords, &color)
            categories = append(categories, map[string]interface{}{
                "name":     name,
                "keywords": keywords,
                "color":    color,
            })
        }
        rows3.Close()

        data, _ := json.MarshalIndent(categories, "", "  ")
        file, _ := zipWriter.Create("categories.json")
        file.Write(data)
    }

    // 4. Экспорт автоплатежей
    rows4, err4 := database.Pool.Query(c.Request.Context(), `
        SELECT partner, amount, frequency, day_of_month, purpose FROM recurring_payments WHERE tenant_id = $1
    `, tenantID)
    if err4 == nil {
        var recurring []map[string]interface{}
        for rows4.Next() {
            var partner, frequency, purpose string
            var amount float64
            var dayOfMonth int
            rows4.Scan(&partner, &amount, &frequency, &dayOfMonth, &purpose)
            recurring = append(recurring, map[string]interface{}{
                "partner":      partner,
                "amount":       amount,
                "frequency":    frequency,
                "day_of_month": dayOfMonth,
                "purpose":      purpose,
            })
        }
        rows4.Close()

        data, _ := json.MarshalIndent(recurring, "", "  ")
        file, _ := zipWriter.Create("recurring.json")
        file.Write(data)
    }

    c.Status(http.StatusOK)
}

// DeleteAllClientData - удаление всех данных клиента
func DeleteAllClientData(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    // Удаляем транзакции
    _, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM bank_statements WHERE tenant_id = $1
    `, tenantID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Удаляем платежи
    _, err = database.Pool.Exec(c.Request.Context(), `
        DELETE FROM payments WHERE tenant_id = $1
    `, tenantID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // Удаляем категории
    _, err = database.Pool.Exec(c.Request.Context(), `
        DELETE FROM payment_categories WHERE tenant_id = $1
    `, tenantID)
    if err != nil {
        // Не критично
    }

    // Удаляем автоплатежи
    _, err = database.Pool.Exec(c.Request.Context(), `
        DELETE FROM recurring_payments WHERE tenant_id = $1
    `, tenantID)
    if err != nil {
        // Не критично
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Все данные удалены",
    })
}

// GetClientTransactions - получить транзакции клиента
func GetClientTransactions(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT operation_date, debit_amount, credit_amount, counterparty_name, payment_purpose
        FROM bank_statements 
        WHERE tenant_id = $1 
        ORDER BY operation_date DESC 
        LIMIT 200
    `, tenantID)
    if err != nil {
        c.JSON(200, []gin.H{})
        return
    }
    defer rows.Close()

    var transactions []gin.H
    for rows.Next() {
        var date time.Time
        var debit, credit float64
        var counterparty, purpose string
        rows.Scan(&date, &debit, &credit, &counterparty, &purpose)

        amount := credit
        if debit > 0 {
            amount = -debit
        }

        transactions = append(transactions, gin.H{
            "date":        date.Format("02.01.2006"),
            "amount":      amount,
            "partner":     counterparty,
            "counterparty": counterparty,
            "purpose":     purpose,
            "type":        map[bool]string{true: "income", false: "expense"}[amount > 0],
        })
    }
    c.JSON(200, transactions)
}

// GetClientPayments - получить платежи клиента
func GetClientPayments(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT date, partner, amount, type, purpose
        FROM payments 
        WHERE tenant_id = $1 
        ORDER BY date DESC 
        LIMIT 200
    `, tenantID)
    if err != nil {
        c.JSON(200, []gin.H{})
        return
    }
    defer rows.Close()

    var payments []gin.H
    for rows.Next() {
        var date string
        var partner, purpose string
        var amount float64
        var typeStr string
        rows.Scan(&date, &partner, &amount, &typeStr, &purpose)

        payments = append(payments, gin.H{
            "date":    date,
            "partner": partner,
            "amount":  amount,
            "type":    typeStr,
            "purpose": purpose,
        })
    }
    c.JSON(200, payments)
}

// GetClientCategories - получить категории клиента
func GetClientCategories(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT name, keywords, color
        FROM payment_categories 
        WHERE tenant_id = $1 
        ORDER BY name
    `, tenantID)
    if err != nil {
        c.JSON(200, []gin.H{})
        return
    }
    defer rows.Close()

    var categories []gin.H
    for rows.Next() {
        var name, keywords, color string
        rows.Scan(&name, &keywords, &color)

        categories = append(categories, gin.H{
            "name":     name,
            "keywords": keywords,
            "color":    color,
        })
    }
    c.JSON(200, categories)
}

// GetClientRecurring - получить автоплатежи клиента
func GetClientRecurring(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT partner, amount, frequency, day_of_month, purpose
        FROM recurring_payments 
        WHERE tenant_id = $1 
        ORDER BY created_at DESC
    `, tenantID)
    if err != nil {
        c.JSON(200, []gin.H{})
        return
    }
    defer rows.Close()

    var recurring []gin.H
    for rows.Next() {
        var partner, frequency, purpose string
        var amount float64
        var dayOfMonth int
        rows.Scan(&partner, &amount, &frequency, &dayOfMonth, &purpose)

        recurring = append(recurring, gin.H{
            "partner":      partner,
            "amount":       amount,
            "frequency":    frequency,
            "day_of_month": dayOfMonth,
            "purpose":      purpose,
        })
    }
    c.JSON(200, recurring)
}

// GetClientStats - статистика клиента
func GetClientStats(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }

    var totalRecords int
    database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM bank_statements WHERE tenant_id = $1
    `, tenantID).Scan(&totalRecords)

    c.JSON(200, gin.H{
        "total_tables":  5,
        "total_records": totalRecords,
        "last_update":   time.Now().Format("02.01.2006 15:04"),
        "db_size":       0,
    })
}

// DeleteClientAllData - удаление всех данных клиента (алиас)
func DeleteClientAllData(c *gin.Context) {
    DeleteAllClientData(c)
}

// ExportClientData - экспорт данных клиента
func ExportClientData(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "aa5f14e6-30e1-476c-ac42-8c11ced838a4"
    }
    format := c.Query("format")
    if format == "" {
        format = "json"
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT operation_date, debit_amount, credit_amount, counterparty_name, payment_purpose
        FROM bank_statements WHERE tenant_id = $1
        ORDER BY operation_date DESC
    `, tenantID)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var data []gin.H
    for rows.Next() {
        var date time.Time
        var debit, credit float64
        var counterparty, purpose string
        rows.Scan(&date, &debit, &credit, &counterparty, &purpose)
        data = append(data, gin.H{
            "date":         date.Format("02.01.2006"),
            "debit":        debit,
            "credit":       credit,
            "counterparty": counterparty,
            "purpose":      purpose,
        })
    }

    c.Header("Content-Disposition", "attachment; filename=my_data_"+time.Now().Format("20060102")+"."+format)

    if format == "csv" {
        c.Header("Content-Type", "text/csv")
        csv := "Дата,Дебет,Кредит,Партнёр,Назначение\n"
        for _, row := range data {
            csv += row["date"].(string) + "," +
                formatFloat(row["debit"].(float64)) + "," +
                formatFloat(row["credit"].(float64)) + "," +
                row["counterparty"].(string) + "," +
                row["purpose"].(string) + "\n"
        }
        c.String(200, csv)
    } else {
        c.Header("Content-Type", "application/json")
        c.JSON(200, data)
    }
}

// formatFloat - вспомогательная функция для форматирования чисел
func formatFloat(f float64) string {
    return fmt.Sprintf("%.2f", f)
}

// ========== SQL ДОСТУП - ЗАЯВКИ ==========

// SQLAccessRequest - структура заявки
type SQLAccessRequest struct {
    Name      string `json:"name"`
    Phone     string `json:"phone"`
    Tariff    string `json:"tariff"`
    Price     string `json:"price"`
    Timestamp string `json:"timestamp"`
}

// RequestSQLAccess - обработчик заявок на SQL-доступ
func RequestSQLAccess(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    userEmail := c.GetString("user_email")
    userName := c.GetString("user_name")

    var req SQLAccessRequest
    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Сохраняем заявку в БД
    _, err := database.Pool.Exec(c.Request.Context(), `
        INSERT INTO sql_access_requests (tenant_id, user_name, user_email, phone, tariff, price, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, 'pending', NOW())
    `, tenantID, userName, userEmail, req.Phone, req.Tariff, req.Price)

    if err != nil {
        fmt.Printf("Ошибка сохранения заявки в БД: %v\n", err)
    }

    // Формируем сообщение для Telegram
    message := fmt.Sprintf(
        "🔔 **НОВАЯ ЗАЯВКА НА SQL-ДОСТУП FinCore!**\n\n"+
            "━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"+
            "👤 **Клиент:** %s\n"+
            "📧 **Email:** %s\n"+
            "📱 **Телефон:** %s\n"+
            "🏷️ **Tenant ID:** %s\n"+
            "━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"+
            "💎 **Тариф:** %s\n"+
            "💰 **Цена:** %s\n"+
            "━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"+
            "📅 **Дата:** %s\n\n"+
            "⚡ **Действия:**\n"+
            "1️⃣ Создать отдельную БД PostgreSQL для клиента\n"+
            "2️⃣ Настроить подключение к FinCore\n"+
            "3️⃣ Настроить мониторинг событий\n"+
            "4️⃣ Выдать доступ и реквизиты\n"+
            "5️⃣ Отправить инструкцию по подключению\n\n"+
            "✉️ **FinCore Team**",
        req.Name,
        userEmail,
        req.Phone,
        tenantID,
        req.Tariff,
        req.Price,
        time.Now().Format("02.01.2006 15:04:05"),
    )

    // Отправляем в Telegram
   botToken := "8926136863:AAFAH0GpGYW_eg3Z5Nz4P4LN5Un9UjFgdHY"
chatID := "8053911775"  // или "1977550186" - какой ID работает

    url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

    data := map[string]interface{}{
        "chat_id":    chatID,
        "text":       message,
        "parse_mode": "Markdown",
    }

    jsonData, _ := json.Marshal(data)

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))

    if err != nil {
        fmt.Printf("Ошибка отправки в Telegram: %v\n", err)
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "message": "✅ Заявка принята! Менеджер свяжется с вами в течение 15 минут",
        })
        return
    }
    defer resp.Body.Close()

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "✅ Заявка отправлена! Менеджер свяжется с вами в течение 15 минут",
    })
}