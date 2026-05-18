package handlers

import (
    "crypto/sha256"
    "database/sql"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "strings"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    
    "subscription-system/database"
    "subscription-system/middleware"
)

func GenerateReconciliationAct(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.GetString("user_id")
    
    body, _ := io.ReadAll(c.Request.Body)
    log.Printf("🔍 Получен запрос на создание акта. TenantID: %v, UserID: %v", tenantID, userID)
    log.Printf("🔍 Raw body: %s", string(body))
    
    c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
    
    var req struct {
        CounterpartyName string `json:"counterparty_name"`
        CounterpartyINN  string `json:"counterparty_inn"`
        PeriodStart      string `json:"period_start"`
        PeriodEnd        string `json:"period_end"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{
            "error":   "Неверные данные запроса",
            "details": err.Error(),
        })
        return
    }
    
    log.Printf("📝 Распарсенные данные: Name=%s, INN=%s, Start=%s, End=%s", 
        req.CounterpartyName, req.CounterpartyINN, req.PeriodStart, req.PeriodEnd)
    
    if req.CounterpartyName == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "counterparty_name обязателен"})
        return
    }
    if req.CounterpartyINN == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "counterparty_inn обязателен"})
        return
    }
    if req.PeriodStart == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "period_start обязателен"})
        return
    }
    if req.PeriodEnd == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "period_end обязателен"})
        return
    }
    
    periodStart, err := time.Parse("2006-01-02", req.PeriodStart)
    if err != nil {
        log.Printf("❌ Ошибка парсинга period_start: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат даты начала. Используйте YYYY-MM-DD"})
        return
    }
    
    periodEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
    if err != nil {
        log.Printf("❌ Ошибка парсинга period_end: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат даты окончания. Используйте YYYY-MM-DD"})
        return
    }
    
    if periodEnd.Before(periodStart) {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Дата окончания не может быть раньше даты начала"})
        return
    }
    
    actID := uuid.New()
    
    totalDebit := 0.0
    totalCredit := 0.0
    closingBalance := 0.0
    
    query := `
        INSERT INTO reconciliation_acts (
            id, tenant_id, counterparty_name, counterparty_inn,
            period_start, period_end, total_debit, total_credit, 
            closing_balance, status, created_by, created_at, 
            signed_by_our, signed_by_their, signature_type
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'generated', $10, NOW(), false, false, 'simple')
        RETURNING id
    `
    
    var newID string
    err = database.Pool.QueryRow(c.Request.Context(), query,
        actID, tenantID, req.CounterpartyName, req.CounterpartyINN,
        periodStart, periodEnd, totalDebit, totalCredit, closingBalance, userID).Scan(&newID)
    
    if err != nil {
        log.Printf("❌ Ошибка создания акта: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Ошибка создания акта: " + err.Error(),
        })
        return
    }
    
    log.Printf("✅ Акт сверки создан и сохранен в БД: id=%s", newID)
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Акт сверки успешно создан",
        "act_id":  newID,
        "data": gin.H{
            "id":                newID,
            "counterparty_name": req.CounterpartyName,
            "period_start":      req.PeriodStart,
            "period_end":        req.PeriodEnd,
            "total_debit":       totalDebit,
            "total_credit":      totalCredit,
            "closing_balance":   closingBalance,
            "status":            "generated",
        },
    })
}

func GetReconciliationActs(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    
    log.Printf("📝 Получение списка актов для tenantID: %v", tenantID)
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, counterparty_name, counterparty_inn, 
               period_start, period_end, 
               COALESCE(total_debit, 0) as total_debit,
               COALESCE(total_credit, 0) as total_credit,
               COALESCE(closing_balance, 0) as closing_balance,
               status, 
               COALESCE(signed_by_our, false) as signed_by_our,
               COALESCE(signed_by_their, false) as signed_by_their,
               COALESCE(signed_by_our_name, ''), COALESCE(signed_by_their_name, ''),
               COALESCE(signature_type, 'simple') as signature_type,
               created_at
        FROM reconciliation_acts
        WHERE tenant_id = $1
        ORDER BY created_at DESC
    `, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка получения списка актов: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Ошибка получения списка актов",
        })
        return
    }
    defer rows.Close()
    
    var acts []gin.H
    for rows.Next() {
        var id uuid.UUID
        var counterpartyName, counterpartyINN, status string
        var signedByOurName, signedByTheirName, signatureType string
        var periodStart, periodEnd, createdAt time.Time
        var totalDebit, totalCredit, closingBalance float64
        var signedByOur, signedByTheir bool
        
        err := rows.Scan(
            &id, &counterpartyName, &counterpartyINN,
            &periodStart, &periodEnd,
            &totalDebit, &totalCredit, &closingBalance,
            &status, &signedByOur, &signedByTheir,
            &signedByOurName, &signedByTheirName,
            &signatureType,
            &createdAt,
        )
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования строки: %v", err)
            continue
        }
        
        acts = append(acts, gin.H{
            "id":                    id.String(),
            "counterparty_name":     counterpartyName,
            "counterparty_inn":      counterpartyINN,
            "period_start":          periodStart.Format("2006-01-02"),
            "period_end":            periodEnd.Format("2006-01-02"),
            "total_debit":           totalDebit,
            "total_credit":          totalCredit,
            "closing_balance":       closingBalance,
            "status":                status,
            "signed_by_our":         signedByOur,
            "signed_by_their":       signedByTheir,
            "signed_by_our_name":    signedByOurName,
            "signed_by_their_name":  signedByTheirName,
            "signature_type":        signatureType,
            "created_at":            createdAt.Format("2006-01-02 15:04:05"),
        })
    }
    
    log.Printf("✅ Найдено актов: %d", len(acts))
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    acts,
    })
}

func GetReconciliationActByID(c *gin.Context) {
    actID := c.Param("id")
    tenantID := middleware.GetTenantIDFromContext(c)
    
    log.Printf("📝 Получение акта по ID: %s", actID)
    
    var id uuid.UUID
    var counterpartyName, counterpartyINN, status string
    var signedByOurName, signedByTheirName, signatureType string
    var periodStart, periodEnd, createdAt, signedAt time.Time
    var totalDebit, totalCredit, closingBalance float64
    var signedByOur, signedByTheir bool
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, counterparty_name, counterparty_inn, 
               period_start, period_end, 
               COALESCE(total_debit, 0), COALESCE(total_credit, 0), COALESCE(closing_balance, 0),
               status, COALESCE(signed_by_our, false), COALESCE(signed_by_their, false),
               COALESCE(signed_by_our_name, ''), COALESCE(signed_by_their_name, ''),
               COALESCE(signature_type, 'simple'),
               created_at, signed_at
        FROM reconciliation_acts
        WHERE id = $1 AND tenant_id = $2
    `, actID, tenantID).Scan(
        &id, &counterpartyName, &counterpartyINN,
        &periodStart, &periodEnd,
        &totalDebit, &totalCredit, &closingBalance,
        &status, &signedByOur, &signedByTheir,
        &signedByOurName, &signedByTheirName,
        &signatureType,
        &createdAt, &signedAt,
    )
    
    if err != nil {
        log.Printf("❌ Акт не найден: %v", err)
        c.JSON(http.StatusNotFound, gin.H{
            "success": false,
            "error":   "Акт не найден",
        })
        return
    }
    
    result := gin.H{
        "id":                    id.String(),
        "counterparty_name":     counterpartyName,
        "counterparty_inn":      counterpartyINN,
        "period_start":          periodStart.Format("2006-01-02"),
        "period_end":            periodEnd.Format("2006-01-02"),
        "total_debit":           totalDebit,
        "total_credit":          totalCredit,
        "closing_balance":       closingBalance,
        "status":                status,
        "signed_by_our":         signedByOur,
        "signed_by_their":       signedByTheir,
        "signed_by_our_name":    signedByOurName,
        "signed_by_their_name":  signedByTheirName,
        "signature_type":        signatureType,
        "created_at":            createdAt.Format("2006-01-02 15:04:05"),
    }
    
    if !signedAt.IsZero() {
        result["signed_at"] = signedAt.Format("2006-01-02 15:04:05")
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    result,
    })
}

func SignReconciliationAct(c *gin.Context) {
    actID := c.Param("id")
    tenantID := middleware.GetTenantIDFromContext(c)
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    userEmail := c.GetString("user_email")
    
    // Получаем данные ИЗ АКТА (организация - это КОНТРАГЕНТ в акте!)
    var counterpartyName string
    var currentSignedOur, currentSignedTheir bool
    var currentStatus string
    var currentOurSigner, currentTheirSigner string
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT counterparty_name,
               COALESCE(signed_by_our, false), COALESCE(signed_by_their, false), status,
               COALESCE(signed_by_our_name, ''), COALESCE(signed_by_their_name, '')
        FROM reconciliation_acts 
        WHERE id = $1 AND tenant_id = $2
    `, actID, tenantID).Scan(
        &counterpartyName,
        &currentSignedOur, &currentSignedTheir, &currentStatus,
        &currentOurSigner, &currentTheirSigner)
    
    if err != nil {
        log.Printf("❌ Ошибка получения данных акта: %v", err)
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }
    
    // Берем ФИО пользователя из базы
    var userFullName string
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(full_name, name, email) FROM users WHERE id = $1
    `, userID).Scan(&userFullName)
    if err != nil {
        userFullName = userName
        if userFullName == "" {
            userFullName = strings.Split(userEmail, "@")[0]
        }
    }
    
    // Отображаемое имя - это НАЗВАНИЕ ИЗ АКТА (counterparty_name)
    displayName := counterpartyName
    
    log.Printf("📝 Подписание акта: ID=%s, Организация из акта=%s, ФИО пользователя=%s", actID, displayName, userFullName)
    
    var req struct {
        SignOur        bool   `json:"sign_our"`
        SignTheir      bool   `json:"sign_their"`
        Signature      string `json:"signature"`
        Certificate    string `json:"certificate"`
        SignerName     string `json:"signer_name"`
        SignerPosition string `json:"signer_position"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{
            "error":   "Неверные данные запроса",
            "details": err.Error(),
        })
        return
    }
    
    if currentStatus == "signed" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Акт уже полностью подписан"})
        return
    }
    
    documentHash := generateDocumentHash(actID)
    
    newSignedOur := currentSignedOur || req.SignOur
    newSignedTheir := currentSignedTheir || req.SignTheir
    
    newOurSigner := currentOurSigner
    ourSignatureHash := ""
    if req.SignOur && !currentSignedOur {
        signerFullName := req.SignerName
        if signerFullName == "" {
            signerFullName = userFullName
            if signerFullName == "" {
                signerFullName = displayName
            }
        }
        signerPosition := req.SignerPosition
        if signerPosition == "" {
            signerPosition = "Представитель"
        }
        
        // Сохраняем с НАЗВАНИЕМ ИЗ АКТА!
        newOurSigner = fmt.Sprintf("%s | %s | %s | %s | %s", 
            displayName, 
            signerFullName, 
            signerPosition,
            time.Now().Format("02.01.2006 15:04:05"),
            documentHash[:16])
        ourSignatureHash = generateSignatureHash(documentHash, req.Signature, userID)
        log.Printf("📝 Подпись сохранена: %s", newOurSigner)
    }
    
    newTheirSigner := currentTheirSigner
    theirSignatureHash := ""
    
    newStatus := currentStatus
    if newSignedOur && newSignedTheir {
        newStatus = "signed"
    } else if newSignedOur {
        newStatus = "partially_signed"
    }
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE reconciliation_acts 
        SET signed_by_our = $1, 
            signed_by_their = $2,
            signed_by_our_name = $3,
            signed_by_their_name = $4,
            our_signature_hash = $5,
            their_signature_hash = $6,
            status = $7,
            signed_at = NOW(),
            signature_type = 'electronic'
        WHERE id = $8 AND tenant_id = $9
    `, newSignedOur, newSignedTheir, newOurSigner, newTheirSigner,
        ourSignatureHash, theirSignatureHash,
        newStatus, actID, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка обновления: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка подписания акта: " + err.Error()})
        return
    }
    
    log.Printf("✅ Акт %s подписан, статус: %s", actID, newStatus)
    
    c.JSON(http.StatusOK, gin.H{
        "success":            true,
        "message":            fmt.Sprintf("✅ Акт подписан: %s", displayName),
        "status":             newStatus,
        "signed_by_our":      newSignedOur,
        "signed_by_their":    newSignedTheir,
        "signed_by_our_name": newOurSigner,
        "signature_hash":     ourSignatureHash,
        "signature_type":     "electronic",
    })
}

func generateDocumentHash(actID string) string {
    data := fmt.Sprintf("%s-%d", actID, time.Now().UnixNano())
    hash := sha256.Sum256([]byte(data))
    return fmt.Sprintf("%x", hash)
}

func generateSignatureHash(documentHash, signature, signerID string) string {
    data := fmt.Sprintf("%s-%s-%s", documentHash, signature, signerID)
    hash := sha256.Sum256([]byte(data))
    return fmt.Sprintf("%x", hash)
}

func DownloadReconciliationAct(c *gin.Context) {
    actID := c.Param("id")
    tenantID := middleware.GetTenantIDFromContext(c)
    
    log.Printf("📝 Скачивание акта: ID=%s", actID)
    
    var counterpartyName, counterpartyINN, status string
    var signedByOurName, signedByTheirName, signatureType, ourSignatureHash, theirSignatureHash string
    var periodStart, periodEnd time.Time
    var signedAt sql.NullTime
    var totalDebit, totalCredit, closingBalance float64
    var signedByOur, signedByTheir bool
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT counterparty_name, counterparty_inn, period_start, period_end, 
               total_debit, total_credit, closing_balance, status,
               COALESCE(signed_by_our, false), COALESCE(signed_by_their, false),
               COALESCE(signed_by_our_name, ''), COALESCE(signed_by_their_name, ''),
               COALESCE(signature_type, 'simple'),
               COALESCE(our_signature_hash, ''), COALESCE(their_signature_hash, ''),
               signed_at
        FROM reconciliation_acts
        WHERE id = $1 AND tenant_id = $2
    `, actID, tenantID).Scan(
        &counterpartyName, &counterpartyINN, &periodStart, &periodEnd,
        &totalDebit, &totalCredit, &closingBalance, &status,
        &signedByOur, &signedByTheir,
        &signedByOurName, &signedByTheirName,
        &signatureType,
        &ourSignatureHash, &theirSignatureHash,
        &signedAt,
    )
    
    if err != nil {
        log.Printf("❌ Ошибка получения акта: %v", err)
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }
    
    log.Printf("📊 Статус акта: signed_by_our=%v", signedByOur)
    
    // Парсим данные подписи - организация берется из signed_by_our_name
    organizationName := ""
    signerFullName := ""
    signerPosition := ""
    signerTime := ""
    
    if signedByOur && signedByOurName != "" {
        parts := strings.Split(signedByOurName, " | ")
        if len(parts) > 0 {
            organizationName = parts[0]
        }
        if len(parts) > 1 {
            signerFullName = parts[1]
        }
        if len(parts) > 2 {
            signerPosition = parts[2]
        }
        if len(parts) > 3 {
            signerTime = parts[3]
        }
    }
    
    if signerTime == "" && signedAt.Valid {
        signerTime = signedAt.Time.Format("02.01.2006 15:04:05")
    }
    
    var signatureStatus string
    var signatureBgColor string
    var signatureBlockHtml string
    
    if signedByOur {
        signatureStatus = "✅ ПОДПИСАНО"
        signatureBgColor = "#d1fae5"
        
        signatureBlockHtml = fmt.Sprintf(`
            <div class="signature-block">
                <h3>📝 ЭЛЕКТРОННАЯ ПОДПИСЬ</h3>
                <div class="signature-item">
                    <p><strong>✅ ПОДПИСАНО: %s</strong></p>
                    <p class="signature-name">%s</p>
                    <p class="signature-position">%s</p>
                    <p class="signature-details">Дата и время подписания: %s</p>
                    %s
                </div>
            </div>`, 
            organizationName, 
            signerFullName, 
            signerPosition,
            signerTime,
            getSignatureHashHTML(ourSignatureHash, true))
    } else {
        signatureStatus = "⏳ НЕ ПОДПИСАН"
        signatureBgColor = "#fee2e2"
        signatureBlockHtml = `
            <div class="signature-block">
                <h3>📝 ЭЛЕКТРОННАЯ ПОДПИСЬ</h3>
                <div class="signature-item">
                    <p><strong>⏳ АКТ НЕ ПОДПИСАН</strong></p>
                    <p class="signature-details">Ожидает подписания</p>
                </div>
            </div>`
    }
    
    html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Акт сверки №%s</title>
    <style>
        body { font-family: 'DejaVu Sans', 'Arial', sans-serif; padding: 40px; color: #333; line-height: 1.6; }
        .header { text-align: center; margin-bottom: 40px; border-bottom: 2px solid #667eea; padding-bottom: 20px; }
        .header h1 { color: #667eea; margin: 0; font-size: 28px; }
        .header p { color: #666; margin: 10px 0 0; }
        .signature-status { background: %s; padding: 15px; border-radius: 10px; text-align: center; margin-bottom: 30px; font-size: 18px; font-weight: bold; }
        .info-block { margin-bottom: 30px; padding: 20px; background: #f8f9fa; border-radius: 12px; }
        .info-row { padding: 10px 0; border-bottom: 1px solid #e9ecef; }
        .info-label { font-weight: bold; width: 180px; display: inline-block; color: #495057; }
        .totals { margin: 30px 0; padding: 25px; background: linear-gradient(135deg, #667eea15, #764ba215); border-radius: 15px; }
        .total-row { font-size: 18px; padding: 10px 0; }
        .signature-block { margin-top: 50px; padding-top: 30px; border-top: 2px solid #dee2e6; }
        .signature-block h3 { color: #495057; margin-bottom: 20px; }
        .signature-item { text-align: center; width: 80%%; margin: 0 auto; }
        .signature-item p { margin: 8px 0; }
        .signature-item hr { width: 80%%; margin: 15px auto; border: 1px solid #dee2e6; }
        .signature-name { font-weight: bold; color: #10b981; font-size: 16px; margin-top: 15px; }
        .signature-position { font-size: 14px; color: #6c757d; margin: 5px 0; }
        .signature-details { font-size: 12px; color: #6c757d; margin-top: 10px; }
        .signature-hash { font-size: 10px; color: #999; word-break: break-all; margin-top: 10px; font-family: monospace; }
        .electronic-seal { display: inline-block; background: #10b981; color: white; font-size: 10px; padding: 2px 8px; border-radius: 12px; margin-left: 10px; }
        .footer { margin-top: 50px; text-align: center; font-size: 11px; color: #6c757d; border-top: 1px solid #dee2e6; padding-top: 20px; }
        .amount { font-size: 20px; font-weight: bold; color: #667eea; }
    </style>
</head>
<body>
    <div class="header">
        <h1>АКТ СВЕРКИ ВЗАИМНЫХ РАСЧЕТОВ №%s</h1>
        <p>Дата формирования: %s</p>
        <p><span class="electronic-seal">🔒 ЭЛЕКТРОННАЯ ПОДПИСЬ</span></p>
    </div>
    
    <div class="signature-status" style="background: %s;">
        %s
    </div>
    
    <div class="info-block">
        <div class="info-row">
            <span class="info-label">🏢 Контрагент:</span>
            <span><strong>%s</strong> (ИНН: %s)</span>
        </div>
        <div class="info-row">
            <span class="info-label">📅 Период:</span>
            <span>%s — %s</span>
        </div>
        <div class="info-row">
            <span class="info-label">🔐 Тип подписи:</span>
            <span><strong>%s</strong></span>
        </div>
    </div>
    
    <div class="totals">
        <div class="total-row" style="font-size: 20px; font-weight: bold; margin-bottom: 15px;">
            💰 ИТОГИ ЗА ПЕРИОД:
        </div>
        <div class="total-row">
            📊 <strong>Дебет:</strong> <span class="amount">%.2f ₽</span>
        </div>
        <div class="total-row">
            📊 <strong>Кредит:</strong> <span class="amount">%.2f ₽</span>
        </div>
        <div class="total-row">
            ⚖️ <strong>Сальдо:</strong> <span style="font-size: 24px; font-weight: bold; color: #667eea;">%.2f ₽</span>
        </div>
    </div>
    
    %s
    
    <div class="footer">
        <p>Акт сверки подписан электронной подписью и имеет юридическую силу согласно Федеральному закону №63-ФЗ</p>
        <p>© %d SaaSPro FinCore. Все права защищены.</p>
    </div>
</body>
</html>`,
        actID[:8],
        signatureBgColor,
        actID[:8],
        time.Now().Format("2006-01-02 15:04:05"),
        signatureBgColor,
        signatureStatus,
        counterpartyName, counterpartyINN,
        periodStart.Format("2006-01-02"),
        periodEnd.Format("2006-01-02"),
        strings.ToUpper(signatureType),
        totalDebit, totalCredit, closingBalance,
        signatureBlockHtml,
        time.Now().Year(),
    )
    
    c.Header("Content-Type", "text/html; charset=utf-8")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=act_%s.html", actID[:8]))
    c.String(http.StatusOK, html)
}

func getSignatureHashHTML(hash string, signed bool) string {
    if signed && hash != "" {
        hashShort := hash
        if len(hash) > 20 {
            hashShort = hash[:20] + "..."
        }
        return fmt.Sprintf(`<div class="signature-hash">🔐 Хеш подписи: %s</div>`, hashShort)
    }
    return ""
}

func UpdateReconciliationAct(c *gin.Context) {
    actID := c.Param("id")
    tenantID := middleware.GetTenantIDFromContext(c)
    
    var req struct {
        CounterpartyName string  `json:"counterparty_name"`
        CounterpartyINN  string  `json:"counterparty_inn"`
        PeriodStart      string  `json:"period_start"`
        PeriodEnd        string  `json:"period_end"`
        TotalDebit       float64 `json:"total_debit"`
        TotalCredit      float64 `json:"total_credit"`
        ClosingBalance   float64 `json:"closing_balance"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        log.Printf("❌ Ошибка парсинга JSON: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    periodStart, err := time.Parse("2006-01-02", req.PeriodStart)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат даты начала"})
        return
    }
    
    periodEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат даты окончания"})
        return
    }
    
    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE reconciliation_acts 
        SET counterparty_name = $1, counterparty_inn = $2,
            period_start = $3, period_end = $4,
            total_debit = $5, total_credit = $6, closing_balance = $7,
            updated_at = NOW()
        WHERE id = $8 AND tenant_id = $9 AND status IN ('draft', 'generated', 'partially_signed')
    `, req.CounterpartyName, req.CounterpartyINN,
        periodStart, periodEnd,
        req.TotalDebit, req.TotalCredit, req.ClosingBalance,
        actID, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка обновления: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления"})
        return
    }
    
    rowsAffected := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Акт нельзя редактировать (возможно, уже подписан)"})
        return
    }
    
    log.Printf("✅ Акт %s обновлен", actID)
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

func DeleteReconciliationAct(c *gin.Context) {
    actID := c.Param("id")
    tenantID := middleware.GetTenantIDFromContext(c)
    
    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM reconciliation_acts 
        WHERE id = $1 AND tenant_id = $2 AND status IN ('draft', 'generated')
    `, actID, tenantID)
    
    if err != nil {
        log.Printf("❌ Ошибка удаления: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления"})
        return
    }
    
    rowsAffected := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден или не может быть удален"})
        return
    }
    
    log.Printf("✅ Акт %s удален", actID)
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// ========== ДОБАВЛЕННЫЕ ФУНКЦИИ ==========

// Получение истории изменений акта
func GetActHistory(c *gin.Context) {
    actID := c.Param("id")
    tenantID := middleware.GetTenantIDFromContext(c)
    
    log.Printf("📜 Получение истории для акта: %s", actID)
    
    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM reconciliation_acts WHERE id = $1 AND tenant_id = $2)
    `, actID, tenantID).Scan(&exists)
    
    if err != nil {
        log.Printf("❌ Ошибка проверки акта: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки акта"})
        return
    }
    
    if !exists {
        log.Printf("❌ Акт не найден: %s", actID)
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, action, user_name, user_email, old_data, new_data, created_at
        FROM reconciliation_act_logs
        WHERE act_id = $1::uuid
        ORDER BY created_at DESC
        LIMIT 50
    `, actID)
    
    if err != nil {
        log.Printf("❌ Ошибка получения истории: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения истории"})
        return
    }
    defer rows.Close()
    
    var history []gin.H
    for rows.Next() {
        var id int
        var action, userName, userEmail string
        var oldData, newData []byte
        var createdAt time.Time
        
        err := rows.Scan(&id, &action, &userName, &userEmail, &oldData, &newData, &createdAt)
        if err != nil {
            continue
        }
        
        var oldDataMap, newDataMap interface{}
        json.Unmarshal(oldData, &oldDataMap)
        json.Unmarshal(newData, &newDataMap)
        
        history = append(history, gin.H{
            "id":         id,
            "action":     action,
            "user_name":  userName,
            "user_email": userEmail,
            "old_data":   oldDataMap,
            "new_data":   newDataMap,
            "created_at": createdAt.Format("2006-01-02 15:04:05"),
        })
    }
    
    log.Printf("✅ Найдено записей истории: %d", len(history))
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    history,
    })
}

// Получение статистики для графика
func GetActStatistics(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    
    log.Printf("📊 Получение статистики для tenantID: %v", tenantID)
    
    var totalActs int
    var totalDebit, totalCredit, totalBalance float64
    var signedCount, draftCount int
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT 
            COALESCE(COUNT(*), 0) as total_acts,
            COALESCE(SUM(total_debit), 0) as total_debit,
            COALESCE(SUM(total_credit), 0) as total_credit,
            COALESCE(SUM(closing_balance), 0) as total_balance,
            COALESCE(SUM(CASE WHEN status = 'signed' THEN 1 ELSE 0 END), 0) as signed_count,
            COALESCE(SUM(CASE WHEN status IN ('draft', 'generated', 'partially_signed') THEN 1 ELSE 0 END), 0) as draft_count
        FROM reconciliation_acts
        WHERE tenant_id = $1
    `, tenantID).Scan(&totalActs, &totalDebit, &totalCredit, &totalBalance, &signedCount, &draftCount)
    
    if err != nil {
        log.Printf("❌ Ошибка получения статистики: %v", err)
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "data": gin.H{
                "total_acts":    0,
                "total_debit":   0,
                "total_credit":  0,
                "total_balance": 0,
                "signed_count":  0,
                "draft_count":   0,
                "months":        []string{},
                "debit_data":    []float64{},
                "credit_data":   []float64{},
                "balance_data":  []float64{},
            },
        })
        return
    }
    
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            DATE_TRUNC('month', created_at) as month,
            COUNT(*) as count,
            COALESCE(SUM(total_debit), 0) as debit,
            COALESCE(SUM(total_credit), 0) as credit,
            COALESCE(SUM(closing_balance), 0) as balance
        FROM reconciliation_acts
        WHERE tenant_id = $1
        GROUP BY DATE_TRUNC('month', created_at)
        ORDER BY month DESC
        LIMIT 12
    `, tenantID)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "success": true,
            "data": gin.H{
                "total_acts":    totalActs,
                "total_debit":   totalDebit,
                "total_credit":  totalCredit,
                "total_balance": totalBalance,
                "signed_count":  signedCount,
                "draft_count":   draftCount,
                "months":        []string{},
                "debit_data":    []float64{},
                "credit_data":   []float64{},
                "balance_data":  []float64{},
            },
        })
        return
    }
    defer rows.Close()
    
    var months []string
    var debitData []float64
    var creditData []float64
    var balanceData []float64
    
    for rows.Next() {
        var month time.Time
        var count int
        var debit, credit, balance float64
        
        rows.Scan(&month, &count, &debit, &credit, &balance)
        months = append([]string{month.Format("Jan 2006")}, months...)
        debitData = append([]float64{debit}, debitData...)
        creditData = append([]float64{credit}, creditData...)
        balanceData = append([]float64{balance}, balanceData...)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data": gin.H{
            "total_acts":     totalActs,
            "total_debit":    totalDebit,
            "total_credit":   totalCredit,
            "total_balance":  totalBalance,
            "signed_count":   signedCount,
            "draft_count":    draftCount,
            "months":         months,
            "debit_data":     debitData,
            "credit_data":    creditData,
            "balance_data":   balanceData,
        },
    })
}

// Массовое удаление актов
func BulkDeleteReconciliationActs(c *gin.Context) {
    tenantID := middleware.GetTenantIDFromContext(c)
    
    var req struct {
        ActIDs []string `json:"act_ids"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные"})
        return
    }
    
    if len(req.ActIDs) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Не выбраны акты для удаления"})
        return
    }
    
    placeholders := make([]string, len(req.ActIDs))
    args := make([]interface{}, len(req.ActIDs)+1)
    args[0] = tenantID
    for i, id := range req.ActIDs {
        placeholders[i] = fmt.Sprintf("$%d", i+2)
        args[i+1] = id
    }
    
    query := fmt.Sprintf(`
        SELECT id, status, counterparty_name FROM reconciliation_acts 
        WHERE tenant_id = $1 AND id IN (%s)
    `, strings.Join(placeholders, ","))
    
    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки актов"})
        return
    }
    defer rows.Close()
    
    var toDelete []string
    var toSkip []string
    
    for rows.Next() {
        var id string
        var status string
        var counterpartyName string
        rows.Scan(&id, &status, &counterpartyName)
        
        if status == "signed" {
            toSkip = append(toSkip, counterpartyName)
        } else {
            toDelete = append(toDelete, id)
        }
    }
    
    if len(toDelete) == 0 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Выбранные акты нельзя удалить (подписаны)"})
        return
    }
    
    deletePlaceholders := make([]string, len(toDelete))
    deleteArgs := make([]interface{}, len(toDelete)+1)
    deleteArgs[0] = tenantID
    for i, id := range toDelete {
        deletePlaceholders[i] = fmt.Sprintf("$%d", i+2)
        deleteArgs[i+1] = id
    }
    
    deleteQuery := fmt.Sprintf(`
        DELETE FROM reconciliation_acts 
        WHERE tenant_id = $1 AND id IN (%s)
    `, strings.Join(deletePlaceholders, ","))
    
    result, err := database.Pool.Exec(c.Request.Context(), deleteQuery, deleteArgs...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка массового удаления"})
        return
    }
    
    rowsAffected := result.RowsAffected()
    
    message := fmt.Sprintf("Удалено актов: %d", rowsAffected)
    if len(toSkip) > 0 {
        message += fmt.Sprintf("\nПропущено (подписаны): %d - %s", len(toSkip), strings.Join(toSkip, ", "))
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "message":       message,
        "deleted_count": rowsAffected,
        "skipped":       toSkip,
    })
}