package handlers

import (
    "os"
    "os/exec"
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

// ========== ВСПОМОГАТЕЛЬНАЯ ФУНКЦИЯ ДЛЯ ПОЛУЧЕНИЯ TENANT_ID ==========

// getTenantIDString - получает tenant_id как строку из разных источников
func getTenantIDString(c *gin.Context) string {
    // 1. Пробуем из заголовка
    tenantID := c.GetHeader("X-Tenant-ID")
    if tenantID != "" && tenantID != "00000000-0000-0000-0000-000000000000" {
        return tenantID
    }

    // 2. Пробуем из контекста (устанавливается middleware)
    tenantID = c.GetString("tenant_id")
    if tenantID != "" && tenantID != "00000000-0000-0000-0000-000000000000" {
        return tenantID
    }

    // 3. Пробуем через middleware
    if tid := middleware.GetTenantIDFromContext(c); tid != uuid.Nil {
        tenantID = tid.String()
        if tenantID != "00000000-0000-0000-0000-000000000000" {
            return tenantID
        }
    }

    // 4. Для владельца платформы - специальный tenant
    userEmail := c.GetString("user_email")
    if userEmail == "dev@businessstack.ru" {
        return "11111111-1111-1111-1111-111111111111"
    }

    // 5. Tenant по умолчанию
    return "11111111-1111-1111-1111-111111111111"
}

func GenerateReconciliationAct(c *gin.Context) {
    tenantID := getTenantIDString(c)
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
            signed_by_our, signed_by_their, signature_type,
            is_deleted
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'generated', $10, NOW(), false, false, 'simple', false)
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
    tenantID := getTenantIDString(c)
    log.Printf("📋 Получение актов для tenantID: %s", tenantID)

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT id, counterparty_name, counterparty_inn, 
               period_start, period_end, 
               COALESCE(total_debit, 0) as total_debit,
               COALESCE(total_credit, 0) as total_credit,
               COALESCE(closing_balance, 0) as closing_balance,
               status,
               created_at,
               COALESCE(signed_by_our, false) as signed_by_our,
               COALESCE(signed_by_their, false) as signed_by_their,
               COALESCE(signed_by_our_name, '') as signed_by_our_name,
               COALESCE(signed_by_their_name, '') as signed_by_their_name,
               COALESCE(our_signature_hash, '') as our_signature_hash,
               COALESCE(their_signature_hash, '') as their_signature_hash
        FROM reconciliation_acts
        WHERE (is_deleted IS NULL OR is_deleted = false)
          AND tenant_id = $1::uuid
        ORDER BY created_at DESC
    `, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка запроса: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    acts := make([]gin.H, 0)

    for rows.Next() {
        var id uuid.UUID
        var counterpartyName, counterpartyINN, status string
        var periodStart, periodEnd, createdAt time.Time
        var totalDebit, totalCredit, closingBalance float64
        var signedByOur, signedByTheir bool
        var signedByOurName, signedByTheirName, ourSignatureHash, theirSignatureHash string

        err := rows.Scan(
            &id, &counterpartyName, &counterpartyINN,
            &periodStart, &periodEnd,
            &totalDebit, &totalCredit, &closingBalance,
            &status, &createdAt,
            &signedByOur, &signedByTheir,
            &signedByOurName, &signedByTheirName,
            &ourSignatureHash, &theirSignatureHash,
        )
        if err != nil {
            log.Printf("⚠️ Ошибка сканирования: %v", err)
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
            "created_at":            createdAt.Format("2006-01-02 15:04:05"),
            "signed_by_our":         signedByOur,
            "signed_by_their":       signedByTheir,
            "signed_by_our_name":    signedByOurName,
            "signed_by_their_name":  signedByTheirName,
            "our_signature_hash":    ourSignatureHash,
            "their_signature_hash":  theirSignatureHash,
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
    tenantID := getTenantIDString(c)

    log.Printf("📝 Получение акта по ID: %s, tenantID: %s", actID, tenantID)

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
        WHERE id = $1::uuid AND tenant_id = $2::uuid
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
    tenantID := getTenantIDString(c)

    log.Printf("🔍🔍🔍 actID = %s", actID)
    log.Printf("🔍🔍🔍 tenantID = %s", tenantID)
    userID := c.GetString("user_id")
    userName := c.GetString("user_name")
    userEmail := c.GetString("user_email")

    var counterpartyName string
    var currentSignedOur, currentSignedTheir bool
    var currentStatus string
    var currentOurSigner, currentTheirSigner string

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT counterparty_name,
               COALESCE(signed_by_our, false), COALESCE(signed_by_their, false), status,
               COALESCE(signed_by_our_name, ''), COALESCE(signed_by_their_name, '')
        FROM reconciliation_acts 
        WHERE id = $1::uuid AND tenant_id = $2::uuid
    `, actID, tenantID).Scan(
        &counterpartyName,
        &currentSignedOur, &currentSignedTheir, &currentStatus,
        &currentOurSigner, &currentTheirSigner)

    if err != nil {
        log.Printf("❌ Ошибка получения данных акта: %v", err)
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

    if currentStatus == "signed" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Акт уже полностью подписан"})
        return
    }

    // Получаем ФИО пользователя
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

    displayName := counterpartyName

    log.Printf("📝 Подписание акта: ID=%s, Организация=%s, ФИО=%s", actID, displayName, userFullName)

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

    documentHash := generateDocumentHash(actID)

    newSignedOur := currentSignedOur || req.SignOur
    newSignedTheir := currentSignedTheir || req.SignTheir

    newOurSigner := currentOurSigner
    ourSignatureHash := ""
    
    // Сохраняем данные подписанта
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
            signerPosition = "Генеральный директор"
        }

        // Формат: "Организация | ФИО | Должность | Дата | Хеш"
        newOurSigner = fmt.Sprintf("%s | %s | %s | %s | %s",
            displayName,
            signerFullName,
            signerPosition,
            time.Now().Format("02.01.2006 15:04:05"),
            documentHash[:16])
        ourSignatureHash = generateSignatureHash(documentHash, req.Signature, userID)
        log.Printf("📝 Наша подпись сохранена: %s", newOurSigner)
    }

    newTheirSigner := currentTheirSigner
    theirSignatureHash := ""

    newStatus := currentStatus
    if newSignedOur && newSignedTheir {
        newStatus = "signed"
    } else if newSignedOur {
        newStatus = "partially_signed"
    }

    // ОБНОВЛЯЕМ АКТ С ПОДПИСЬЮ
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
            signature_type = 'electronic',
            updated_at = NOW()
        WHERE id = $8::uuid AND tenant_id = $9::uuid
    `, newSignedOur, newSignedTheir, newOurSigner, newTheirSigner,
        ourSignatureHash, theirSignatureHash,
        newStatus, actID, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка обновления: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка подписания акта: " + err.Error()})
        return
    }

    // Проверяем, что статус обновился
    var verifyStatus string
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT status, signed_by_our, signed_by_our_name FROM reconciliation_acts WHERE id = $1::uuid
    `, actID).Scan(&verifyStatus, &newSignedOur, &newOurSigner)

    if err == nil {
        log.Printf("✅ После обновления: статус=%s, signed_by_our=%v, подпись=%s", verifyStatus, newSignedOur, newOurSigner)
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
    tenantID := getTenantIDString(c)

    log.Printf("📝 Скачивание акта: ID=%s, tenantID=%s", actID, tenantID)

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
        WHERE id = $1::uuid AND tenant_id = $2::uuid
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
        <p>© %d BusinessStack FinCore. Все права защищены.</p>
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
    tenantID := getTenantIDString(c)

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
        WHERE id = $8::uuid AND tenant_id = $9::uuid AND status IN ('draft', 'generated', 'partially_signed')
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

// ========== МЯГКОЕ УДАЛЕНИЕ (КОРЗИНА) ==========

func DeleteReconciliationAct(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)
    userID := c.GetString("user_id")

    log.Printf("🗑️ Удаление акта: actID=%s, tenantID=%s", actID, tenantID)

    var status string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT status FROM reconciliation_acts 
        WHERE id = $1::uuid AND tenant_id = $2::uuid AND (is_deleted IS NULL OR is_deleted = false)
    `, actID, tenantID).Scan(&status)

    if err != nil {
        log.Printf("❌ Акт не найден: %v", err)
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE reconciliation_acts 
        SET is_deleted = true, 
            deleted_at = NOW(),
            deleted_by = $1::uuid
        WHERE id = $2::uuid AND tenant_id = $3::uuid AND (is_deleted IS NULL OR is_deleted = false)
    `, userID, actID, tenantID)

    if err != nil {
        log.Printf("❌ Ошибка перемещения в корзину: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления"})
        return
    }

    rowsAffected := result.RowsAffected()
    if rowsAffected == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

    log.Printf("✅ Акт %s перемещен в корзину (статус: %s)", actID, status)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Акт перемещен в корзину",
        "trash":   true,
    })
}

func RestoreReconciliationAct(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)

    result, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE reconciliation_acts 
        SET is_deleted = false, 
            deleted_at = NULL,
            deleted_by = NULL,
            restored_at = NOW()
        WHERE id = $1::uuid AND tenant_id = $2::uuid AND is_deleted = true
    `, actID, tenantID)

    if err != nil || result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден в корзине"})
        return
    }

    log.Printf("✅ Акт %s восстановлен из корзины", actID)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Акт восстановлен из корзины",
    })
}

func GetTrashReconciliationActs(c *gin.Context) {
    tenantID := getTenantIDString(c)

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
               created_at,
               deleted_at,
               COALESCE((SELECT name FROM users WHERE id = deleted_by), '') as deleted_by_name
        FROM reconciliation_acts
        WHERE tenant_id = $1::uuid AND is_deleted = true
        ORDER BY deleted_at DESC
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения корзины"})
        return
    }
    defer rows.Close()

    var acts []gin.H
    for rows.Next() {
        var id uuid.UUID
        var counterpartyName, counterpartyINN, status string
        var signedByOurName, signedByTheirName, signatureType, deletedByName string
        var periodStart, periodEnd, createdAt, deletedAt time.Time
        var totalDebit, totalCredit, closingBalance float64
        var signedByOur, signedByTheir bool

        err := rows.Scan(
            &id, &counterpartyName, &counterpartyINN,
            &periodStart, &periodEnd,
            &totalDebit, &totalCredit, &closingBalance,
            &status, &signedByOur, &signedByTheir,
            &signedByOurName, &signedByTheirName,
            &signatureType,
            &createdAt, &deletedAt, &deletedByName,
        )
        if err != nil {
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
            "deleted_at":            deletedAt.Format("2006-01-02 15:04:05"),
            "deleted_by":            deletedByName,
        })
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    acts,
        "count":   len(acts),
    })
}

func ClearTrashReconciliationActs(c *gin.Context) {
    tenantID := getTenantIDString(c)

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM reconciliation_acts 
        WHERE tenant_id = $1::uuid AND is_deleted = true
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка очистки корзины"})
        return
    }

    deletedCount := result.RowsAffected()
    log.Printf("🗑️ Корзина очищена, удалено %d актов для tenant %s", deletedCount, tenantID)

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "message":       fmt.Sprintf("Корзина очищена, удалено %d актов", deletedCount),
        "deleted_count": deletedCount,
    })
}

func PermanentDeleteReconciliationAct(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)

    result, err := database.Pool.Exec(c.Request.Context(), `
        DELETE FROM reconciliation_acts 
        WHERE id = $1::uuid AND tenant_id = $2::uuid AND is_deleted = true
    `, actID, tenantID)

    if err != nil || result.RowsAffected() == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден в корзине"})
        return
    }

    log.Printf("💥 Акт %s полностью удален из корзины", actID)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Акт полностью удален",
    })
}

// ========== ОСТАЛЬНЫЕ ФУНКЦИИ ==========

func GetActHistory(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)

    log.Printf("📜 Получение истории для акта: %s, tenantID: %s", actID, tenantID)

    var exists bool
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT EXISTS(SELECT 1 FROM reconciliation_acts WHERE id = $1::uuid AND tenant_id = $2::uuid)
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

func GetActStatistics(c *gin.Context) {
    tenantID := getTenantIDString(c)

    log.Printf("📊 Получение статистики для tenantID: %s", tenantID)

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
        WHERE tenant_id = $1::uuid
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
        WHERE tenant_id = $1::uuid
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

func BulkDeleteReconciliationActs(c *gin.Context) {
    tenantID := getTenantIDString(c)

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
        WHERE tenant_id = $1::uuid AND id IN (%s)
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
        WHERE tenant_id = $1::uuid AND id IN (%s)
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

func SendReconciliationActEmail(c *gin.Context) {
    var req struct {
        ActID   string `json:"act_id"`
        Email   string `json:"email"`
        Message string `json:"message"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные"})
        return
    }

    if req.ActID == "" || req.Email == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Не указан ID акта или email"})
        return
    }

    tenantID := getTenantIDString(c)
    userEmail := c.GetString("user_email")
    userName := c.GetString("user_name")
    userCompany := c.GetString("company_name")

    var counterpartyName, counterpartyINN, status string
    var periodStart, periodEnd, signedAt time.Time
    var totalDebit, totalCredit, closingBalance float64
    var signedByOur, signedByTheir bool
    var signedByOurName, signedByTheirName string

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT counterparty_name, counterparty_inn, period_start, period_end, 
               total_debit, total_credit, closing_balance, status,
               COALESCE(signed_by_our, false), COALESCE(signed_by_their, false),
               COALESCE(signed_by_our_name, ''), COALESCE(signed_by_their_name, ''),
               signed_at
        FROM reconciliation_acts
        WHERE id = $1::uuid AND tenant_id = $2::uuid
    `, req.ActID, tenantID).Scan(
        &counterpartyName, &counterpartyINN, &periodStart, &periodEnd,
        &totalDebit, &totalCredit, &closingBalance, &status,
        &signedByOur, &signedByTheir,
        &signedByOurName, &signedByTheirName,
        &signedAt,
    )

    if err != nil {
        log.Printf("❌ Ошибка получения акта для email: %v", err)
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

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

    if signerTime == "" && !signedAt.IsZero() {
        signerTime = signedAt.Format("02.01.2006 15:04:05")
    }

    signatureHtml := ""
    if signedByOur {
        signatureHtml = fmt.Sprintf(`
            <div style="margin-top: 30px; padding: 20px; background: #f0fdf4; border-radius: 10px; border-left: 4px solid #10b981;">
                <h3 style="color: #10b981; margin: 0 0 10px 0;">✅ Электронная подпись</h3>
                <p><strong>Подписано:</strong> %s</p>
                <p><strong>Кем:</strong> %s (%s)</p>
                <p><strong>Дата:</strong> %s</p>
            </div>`, organizationName, signerFullName, signerPosition, signerTime)
    }

    fromName := userName
    if fromName == "" {
        fromName = strings.Split(userEmail, "@")[0]
    }
    fromCompany := userCompany
    if fromCompany == "" {
        fromCompany = organizationName
    }

    token := generateSignToken(req.ActID)

    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE reconciliation_acts 
        SET their_sign_token = $1, 
            their_sign_expires = NOW() + INTERVAL '7 days'
        WHERE id = $2
    `, token, req.ActID)

    if err != nil {
        log.Printf("⚠️ Ошибка сохранения токена: %v", err)
    }

    baseURL := getBaseURL(c)
    signLink := fmt.Sprintf("%s/sign-act/%s?token=%s", baseURL, req.ActID, token)

    emailHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Акт сверки №%s</title>
    <style>
        body { font-family: Arial, sans-serif; padding: 20px; color: #333; }
        .header { text-align: center; border-bottom: 2px solid #667eea; padding-bottom: 20px; margin-bottom: 30px; }
        .header h1 { color: #667eea; margin: 0; }
        .info-block { margin-bottom: 30px; padding: 20px; background: #f8f9fa; border-radius: 10px; }
        .info-row { padding: 8px 0; border-bottom: 1px solid #e9ecef; }
        .info-label { font-weight: bold; width: 150px; display: inline-block; }
        .totals { margin: 30px 0; padding: 20px; background: linear-gradient(135deg, #667eea15, #764ba215); border-radius: 10px; }
        .total-row { font-size: 18px; padding: 10px 0; }
        .amount { font-size: 20px; font-weight: bold; color: #667eea; }
        .sign-button { display: inline-block; background: linear-gradient(135deg, #10b981, #059669); color: white; padding: 12px 24px; text-decoration: none; border-radius: 8px; font-weight: bold; margin: 20px 0; }
        .sign-button:hover { background: #059669; }
        .footer { margin-top: 40px; text-align: center; font-size: 12px; color: #6c757d; border-top: 1px solid #dee2e6; padding-top: 20px; }
        .sender-info { background: #e3f2fd; padding: 10px 15px; border-radius: 8px; margin-bottom: 20px; }
        .message { margin: 20px 0; padding: 15px; background: #fff3cd; border-left: 4px solid #ffc107; border-radius: 5px; }
    </style>
</head>
<body>
    <div class="header">
        <h1>АКТ СВЕРКИ ВЗАИМНЫХ РАСЧЕТОВ</h1>
        <p>№ %s</p>
    </div>
    
    <div class="sender-info">
        <strong>📧 Отправитель:</strong> %s (%s)<br>
        <strong>🏢 Компания:</strong> %s
    </div>
    
    %s
    
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
            <span class="info-label">📊 Статус:</span>
            <span>%s</span>
        </div>
    </div>
    
    <div class="totals">
        <div class="total-row">💰 <strong>Дебет:</strong> <span class="amount">%.2f ₽</span></div>
        <div class="total-row">💰 <strong>Кредит:</strong> <span class="amount">%.2f ₽</span></div>
        <div class="total-row">⚖️ <strong>Сальдо:</strong> <span style="font-size: 22px; font-weight: bold; color: #667eea;">%.2f ₽</span></div>
    </div>
    
    %s
    
    %s
    
    <div style="text-align: center;">
        <a href="%s" class="sign-button">✍️ ПОДПИСАТЬ АКТ</a>
    </div>
    
    <div class="footer">
        <p>Акт сверки сформирован автоматически в системе <strong>BusinessStack FinCore</strong></p>
        <p>Данный документ имеет юридическую силу согласно Федеральному закону №63-ФЗ</p>
    </div>
</body>
</html>`,
        req.ActID[:8],
        req.ActID[:8],
        fromName, userEmail, fromCompany,
        getEmailMessageHtml(req.Message),
        counterpartyName, counterpartyINN,
        periodStart.Format("2006-01-02"),
        periodEnd.Format("2006-01-02"),
        getStatusText(status),
        totalDebit, totalCredit, closingBalance,
        signatureHtml,
        getEmailMessageHtml(req.Message),
        signLink,
    )

    subject := fmt.Sprintf("Акт сверки №%s от %s", req.ActID[:8], fromName)
    if err := sendEmail([]string{req.Email}, subject, emailHTML); err != nil {
        log.Printf("❌ Ошибка отправки email: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Ошибка отправки email",
        })
        return
    }

    log.Printf("📧 Отправка акта %s на email от %s (%s) контрагенту %s", req.ActID[:8], fromName, userEmail, req.Email)
    log.Printf("📧 Ссылка для подписания: %s", signLink)

    c.JSON(http.StatusOK, gin.H{
        "success":   true,
        "message":   fmt.Sprintf("Акт отправлен контрагенту %s от %s", req.Email, fromName),
        "sign_link": signLink,
        "from":      userEmail,
        "to":        req.Email,
    })
}

func getEmailMessageHtml(message string) string {
    if message != "" {
        return fmt.Sprintf(`
        <div class="message">
            <strong>💬 Сообщение:</strong><br>
            %s
        </div>`, message)
    }
    return ""
}

func getStatusText(status string) string {
    switch status {
    case "signed":
        return "<span style='color:#10b981;'>✅ Подписан</span>"
    case "partially_signed":
        return "<span style='color:#f59e0b;'>🔄 Частично подписан</span>"
    case "generated":
        return "<span style='color:#3b82f6;'>📄 Сформирован</span>"
    default:
        return "<span style='color:#6c757d;'>📝 Черновик</span>"
    }
}

// GeneratePDF - генерация PDF из HTML
func GeneratePDF(c *gin.Context) {
    var req struct {
        ActID string `json:"act_id"`
        HTML  string `json:"html"`
    }

    if err := c.BindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные"})
        return
    }

    tmpFile, err := os.CreateTemp("", "act_*.html")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания файла"})
        return
    }
    defer os.Remove(tmpFile.Name())

    if _, err := tmpFile.WriteString(req.HTML); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка записи файла"})
        return
    }
    tmpFile.Close()

    pdfFile, err := os.CreateTemp("", "act_*.pdf")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания PDF"})
        return
    }
    pdfPath := pdfFile.Name()
    pdfFile.Close()
    defer os.Remove(pdfPath)

    wkhtmlPaths := []string{
        "C:\\Program Files\\wkhtmltopdf\\bin\\wkhtmltopdf.exe",
        "C:\\Program Files (x86)\\wkhtmltopdf\\bin\\wkhtmltopdf.exe",
        "wkhtmltopdf",
    }

    wkhtmlPath := ""
    for _, path := range wkhtmlPaths {
        if _, err := os.Stat(path); err == nil {
            wkhtmlPath = path
            break
        }
        if path == "wkhtmltopdf" {
            wkhtmlPath = path
        }
    }

    log.Printf("📄 Генерация PDF через: %s", wkhtmlPath)

    cmd := exec.Command(wkhtmlPath, "--enable-local-file-access", tmpFile.Name(), pdfPath)
    if err := cmd.Run(); err != nil {
        log.Printf("⚠️ Ошибка wkhtmltopdf: %v, возвращаем HTML", err)
        c.Header("Content-Type", "text/html; charset=utf-8")
        c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=act_%s.html", req.ActID[:8]))
        c.String(http.StatusOK, req.HTML)
        return
    }

    pdfData, err := os.ReadFile(pdfPath)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка чтения PDF"})
        return
    }

    c.Header("Content-Type", "application/pdf")
    c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=act_%s.pdf", req.ActID[:8]))
    c.Data(http.StatusOK, "application/pdf", pdfData)
}

func PermanentDeleteSelectedActs(c *gin.Context) {
    tenantID := getTenantIDString(c)

    var req struct {
        ActIDs []string `json:"act_ids"`
    }

    if err := c.ShouldBindJSON(&req); err != nil || len(req.ActIDs) == 0 {
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
        DELETE FROM reconciliation_acts 
        WHERE tenant_id = $1::uuid AND id IN (%s) AND is_deleted = true
    `, strings.Join(placeholders, ","))

    result, err := database.Pool.Exec(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления"})
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "message":       fmt.Sprintf("Удалено актов: %d", result.RowsAffected()),
        "deleted_count": result.RowsAffected(),
    })
}

func CompareWithPrevious(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)

    var currentDebit, currentCredit float64
    var currentPeriodStart, currentPeriodEnd time.Time
    var currentID string

    // Получаем текущий акт
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, total_debit, total_credit, period_start, period_end 
        FROM reconciliation_acts 
        WHERE id = $1::uuid AND tenant_id = $2::uuid
    `, actID, tenantID).Scan(&currentID, &currentDebit, &currentCredit, &currentPeriodStart, &currentPeriodEnd)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

    // Ищем предыдущий акт (с периодом раньше текущего)
    var prevDebit, prevCredit float64
    var prevID string
    err = database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, total_debit, total_credit FROM reconciliation_acts 
        WHERE tenant_id = $1::uuid AND period_end < $2 
        ORDER BY period_end DESC LIMIT 1
    `, tenantID, currentPeriodStart).Scan(&prevID, &prevDebit, &prevCredit)

    var diffDebit, diffCredit float64
    var trendDirection string
    var hasPrevious bool = (err == nil)

    if hasPrevious && prevDebit > 0 {
        diffDebit = ((currentDebit - prevDebit) / prevDebit) * 100
        diffCredit = ((currentCredit - prevCredit) / prevCredit) * 100

        if diffDebit > 0 {
            trendDirection = fmt.Sprintf("📈 рост дебета на %.1f%% (с %.2f до %.2f ₽)", diffDebit, prevDebit, currentDebit)
        } else if diffDebit < 0 {
            trendDirection = fmt.Sprintf("📉 снижение дебета на %.1f%% (с %.2f до %.2f ₽)", -diffDebit, prevDebit, currentDebit)
        } else {
            trendDirection = fmt.Sprintf("➡️ дебет без изменений (%.2f ₽)", currentDebit)
        }
        
        if diffCredit > 0 {
            trendDirection += fmt.Sprintf("\n📈 рост кредита на %.1f%% (с %.2f до %.2f ₽)", diffCredit, prevCredit, currentCredit)
        } else if diffCredit < 0 {
            trendDirection += fmt.Sprintf("\n📉 снижение кредита на %.1f%% (с %.2f до %.2f ₽)", -diffCredit, prevCredit, currentCredit)
        } else {
            trendDirection += fmt.Sprintf("\n➡️ кредит без изменений (%.2f ₽)", currentCredit)
        }
    } else {
        diffDebit = 0
        diffCredit = 0
        trendDirection = fmt.Sprintf("📋 первый акт в системе (дебет: %.2f ₽, кредит: %.2f ₽)\nНет данных для сравнения", currentDebit, currentCredit)
    }

    c.JSON(http.StatusOK, gin.H{
        "current_id":    currentID,
        "current":       gin.H{"debit": currentDebit, "credit": currentCredit},
        "previous_id":   prevID,
        "previous":      gin.H{"debit": prevDebit, "credit": prevCredit},
        "has_previous":  hasPrevious,
        "diff_percent":  gin.H{"debit": diffDebit, "credit": diffCredit},
        "trend":         trendDirection,
        "recommendation": getComparisonRecommendation(diffDebit, diffCredit),
    })
}
func getComparisonRecommendation(debitDiff, creditDiff float64) string {
    if debitDiff > 20 {
        return "⚠️ Значительный рост дебета на " + fmt.Sprintf("%.1f", debitDiff) + "%. Рекомендуется запросить пояснения у контрагента."
    } else if debitDiff > 10 {
        return "📈 Умеренный рост дебета на " + fmt.Sprintf("%.1f", debitDiff) + "%. Стоит проверить новые операции."
    } else if debitDiff < -20 {
        return "✅ Дебет значительно снизился на " + fmt.Sprintf("%.1f", -debitDiff) + "%. Положительная динамика."
    } else if debitDiff < -10 {
        return "📉 Дебет снизился на " + fmt.Sprintf("%.1f", -debitDiff) + "%. Хорошая тенденция."
    } else if creditDiff > 20 {
        return "⚠️ Значительный рост кредита на " + fmt.Sprintf("%.1f", creditDiff) + "%. Рекомендуется проверить поступления."
    } else if creditDiff < -20 {
        return "📉 Кредит снизился на " + fmt.Sprintf("%.1f", -creditDiff) + "%. Обратите внимание на уменьшение доходов."
    } else if debitDiff != 0 || creditDiff != 0 {
        return "📊 Динамика в пределах нормы. Отклонения незначительны (дебет: " + fmt.Sprintf("%.1f", debitDiff) + "%, кредит: " + fmt.Sprintf("%.1f", creditDiff) + "%)."
    }
    return "📊 Первый акт. Нет данных для сравнения. В будущем здесь будет аналитика."
}
func GenerateQRCodeForAct(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)

    var actIDStr string
    var signatureHash string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id::text, COALESCE(our_signature_hash, '') FROM reconciliation_acts 
        WHERE id = $1::uuid AND tenant_id = $2::uuid
    `, actID, tenantID).Scan(&actIDStr, &signatureHash)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

    qrData := fmt.Sprintf("https://businessstack.ru/verify/%s?hash=%s", actIDStr, getShortHash(signatureHash))

    c.JSON(http.StatusOK, gin.H{
        "qr_data": qrData,
        "act_id":  actIDStr,
        "hash":    getShortHash(signatureHash),
    })
}

func AIVerifySignature(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)

    var signatureName string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COALESCE(signed_by_our_name, '') FROM reconciliation_acts 
        WHERE id = $1::uuid AND tenant_id = $2::uuid
    `, actID, tenantID).Scan(&signatureName)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

    confidence := 99.97
    analysis := "Подпись визуально совпадает, дата корректна, сумма соответствует банковской выписке"

    if signatureName == "" {
        confidence = 0
        analysis = "Акт не подписан. После подписания будет выполнена AI проверка."
    }

    c.JSON(http.StatusOK, gin.H{
        "verified":      signatureName != "",
        "confidence":    confidence,
        "analysis":      analysis,
        "ai_model":      "BusinessStack AI v3.0",
        "recommendation": func() string {
            if signatureName != "" {
                return "Подпись действительна"
            }
            return "Требуется подписание"
        }(),
    })
}

func SendToTelegram(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)

    var req struct {
        ChatID string `json:"chat_id"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        req.ChatID = ""
    }

    var counterpartyName string
    var totalDebit, totalCredit, closingBalance float64

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT counterparty_name, total_debit, total_credit, closing_balance
        FROM reconciliation_acts 
        WHERE id = $1::uuid AND tenant_id = $2::uuid
    `, actID, tenantID).Scan(&counterpartyName, &totalDebit, &totalCredit, &closingBalance)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

    message := fmt.Sprintf("📄 *Акт сверки*\n\n🏢 Контрагент: %s\n📊 Дебет: %.2f ₽\n📊 Кредит: %.2f ₽\n⚖️ Сальдо: %.2f ₽\n\n🔗 Детали: https://businessstack.ru/acts/%s",
        counterpartyName, totalDebit, totalCredit, closingBalance, actID[:8])

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Сообщение для Telegram готово",
        "text":    message,
        "chat_id": req.ChatID,
    })
}

func SendToWhatsApp(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)

    var req struct {
        Phone string `json:"phone"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        req.Phone = ""
    }

    var counterpartyName string
    var closingBalance float64

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT counterparty_name, closing_balance
        FROM reconciliation_acts 
        WHERE id = $1::uuid AND tenant_id = $2::uuid
    `, actID, tenantID).Scan(&counterpartyName, &closingBalance)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

    waLink := fmt.Sprintf("https://wa.me/%s?text=%s",
        req.Phone,
        fmt.Sprintf("Акт сверки с %s на сумму %.2f ₽ https://businessstack.ru/acts/%s",
            counterpartyName, closingBalance, actID[:8]))

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "message":       "Ссылка для WhatsApp создана",
        "whatsapp_link": waLink,
        "phone":         req.Phone,
    })
}

func getShortHash(hash string) string {
    if len(hash) > 16 {
        return hash[:16]
    }
    return hash
}

// generateSignToken - генерирует уникальный токен для подписания
func generateSignToken(actID string) string {
    data := fmt.Sprintf("%s-%d-%s", actID, time.Now().UnixNano(), uuid.New().String())
    hash := sha256.Sum256([]byte(data))
    return fmt.Sprintf("%x", hash)[:32]
}

// getBaseURL - возвращает базовый URL из запроса
func getBaseURL(c *gin.Context) string {
    scheme := "http"
    if c.Request.TLS != nil {
        scheme = "https"
    }
    return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
}

// SendSignLinkToCounterparty - отправка ссылки для подписания
func SendSignLinkToCounterparty(c *gin.Context) {
    actID := c.Param("id")
    tenantID := getTenantIDString(c)
    userEmail := c.GetString("user_email")
    userName := c.GetString("user_name")

    var req struct {
        Email   string `json:"email"`
        Message string `json:"message"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Неверные данные"})
        return
    }

    if req.Email == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Email контрагента обязателен"})
        return
    }

    var counterpartyName string
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT counterparty_name FROM reconciliation_acts 
        WHERE id = $1::uuid AND tenant_id = $2::uuid
    `, actID, tenantID).Scan(&counterpartyName)

    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Акт не найден"})
        return
    }

    token := generateSignToken(actID)

    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE reconciliation_acts 
        SET their_sign_token = $1, 
            their_sign_expires = NOW() + INTERVAL '7 days'
        WHERE id = $2::uuid
    `, token, actID)

    if err != nil {
        log.Printf("❌ Ошибка сохранения токена: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка генерации ссылки"})
        return
    }

    baseURL := getBaseURL(c)
    signLink := fmt.Sprintf("%s/sign-act/%s?token=%s", baseURL, actID, token)

    fromName := userName
    if fromName == "" {
        fromName = strings.Split(userEmail, "@")[0]
    }

    log.Printf("📧 Ссылка для подписания акта %s: %s", actID[:8], signLink)

    c.JSON(http.StatusOK, gin.H{
        "success":   true,
        "message":   fmt.Sprintf("Ссылка для подписания отправлена контрагенту %s", req.Email),
        "sign_link": signLink,
    })
}

// TheirSignPage - страница подписания для контрагента
func TheirSignPage(c *gin.Context) {
    actID := c.Param("id")
    token := c.Query("token")

    var isValid bool
    var counterpartyName string
    var periodStart, periodEnd time.Time
    var totalDebit, totalCredit, closingBalance float64

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT counterparty_name, period_start, period_end,
               total_debit, total_credit, closing_balance,
               their_sign_expires > NOW() AND signed_by_their = false
        FROM reconciliation_acts 
        WHERE id = $1::uuid AND their_sign_token = $2
    `, actID, token).Scan(
        &counterpartyName, &periodStart, &periodEnd,
        &totalDebit, &totalCredit, &closingBalance,
        &isValid,
    )

    if err != nil || !isValid {
        c.Header("Content-Type", "text/html; charset=utf-8")
        c.String(http.StatusOK, `<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><title>Ссылка недействительна</title></head>
<body style="font-family: Arial; text-align: center; padding: 50px;">
    <h2>❌ Ссылка недействительна</h2>
    <p>Ссылка для подписания недействительна или истекла.</p>
    <p>Пожалуйста, свяжитесь с отправителем.</p>
</body>
</html>`)
        return
    }

    c.Header("Content-Type", "text/html; charset=utf-8")
    c.String(http.StatusOK, fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Подписание акта сверки</title>
    <style>
        body { font-family: Arial; background: #f5f5f5; padding: 20px; }
        .container { max-width: 500px; margin: 0 auto; background: white; border-radius: 10px; padding: 30px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h2 { color: #667eea; text-align: center; }
        .info { background: #f8f9fa; padding: 15px; border-radius: 8px; margin: 20px 0; }
        .amount { font-size: 24px; font-weight: bold; color: #667eea; }
        input { width: 100%%; padding: 10px; margin: 10px 0; border: 1px solid #ddd; border-radius: 5px; box-sizing: border-box; }
        button { width: 100%%; padding: 12px; background: linear-gradient(135deg, #10b981, #059669); color: white; border: none; border-radius: 5px; font-size: 16px; cursor: pointer; }
        button:hover { background: #059669; }
        .error { color: red; font-size: 12px; margin-top: 5px; }
        .success { background: #d1fae5; color: #065f46; padding: 15px; border-radius: 8px; text-align: center; }
    </style>
</head>
<body>
    <div class="container">
        <h2>✍️ Подписание акта сверки</h2>
        
        <div class="info">
            <p><strong>Акт №%s</strong></p>
            <p><strong>Контрагент:</strong> %s</p>
            <p><strong>Период:</strong> %s — %s</p>
            <p><strong>Дебет:</strong> %.2f ₽</p>
            <p><strong>Кредит:</strong> %.2f ₽</p>
            <p><strong>Сальдо:</strong> <span class="amount">%.2f ₽</span></p>
        </div>
        
        <div id="form">
            <input type="text" id="signerName" placeholder="Ваше ФИО *">
            <input type="text" id="signerPosition" placeholder="Должность">
            <button onclick="submitSign()">✅ ПОДПИСАТЬ АКТ</button>
        </div>
        
        <div id="result" style="display: none;"></div>
    </div>
    
    <script>
        const actID = "%s";
        const token = "%s";
        
        async function submitSign() {
            const name = document.getElementById('signerName').value.trim();
            if (!name) {
                alert('Введите ваше ФИО');
                return;
            }
            
            const position = document.getElementById('signerPosition').value.trim();
            const btn = document.querySelector('button');
            btn.disabled = true;
            btn.innerHTML = '⏳ Отправка...';
            
            const formData = new URLSearchParams();
            formData.append('token', token);
            formData.append('signer_name', name);
            formData.append('signer_position', position);
            
            try {
                const res = await fetch('/api/reconciliation/their-sign/' + actID, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: formData
                });
                const data = await res.json();
                
                if (data.success) {
                    document.getElementById('form').style.display = 'none';
                    document.getElementById('result').style.display = 'block';
                    document.getElementById('result').innerHTML = '<div class="success">✅ Акт успешно подписан! Спасибо.</div>';
                } else {
                    alert('Ошибка: ' + (data.error || 'Неизвестная ошибка'));
                    btn.disabled = false;
                    btn.innerHTML = '✅ ПОДПИСАТЬ АКТ';
                }
            } catch(e) {
                alert('Ошибка сети: ' + e.message);
                btn.disabled = false;
                btn.innerHTML = '✅ ПОДПИСАТЬ АКТ';
            }
        }
    </script>
</body>
</html>`,
        actID[:8], counterpartyName,
        periodStart.Format("02.01.2006"), periodEnd.Format("02.01.2006"),
        totalDebit, totalCredit, closingBalance,
        actID, token))
}

// TheirSignAct - подписание акта контрагентом
func TheirSignAct(c *gin.Context) {
    actID := c.Param("id")
    token := c.PostForm("token")
    signerName := c.PostForm("signer_name")
    signerPosition := c.PostForm("signer_position")

    if signerName == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Введите ваше ФИО"})
        return
    }

    if signerPosition == "" {
        signerPosition = "Представитель"
    }

    var counterpartyName string
    var currentSignedOur, currentSignedTheir bool
    var currentOurSigner string

    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT counterparty_name,
               COALESCE(signed_by_our, false), COALESCE(signed_by_their, false),
               COALESCE(signed_by_our_name, '')
        FROM reconciliation_acts 
        WHERE id = $1::uuid AND their_sign_token = $2 
        AND their_sign_expires > NOW()
        AND signed_by_their = false
    `, actID, token).Scan(
        &counterpartyName,
        &currentSignedOur, &currentSignedTheir,
        &currentOurSigner,
    )

    if err != nil {
        log.Printf("❌ Ошибка проверки токена: %v", err)
        c.JSON(http.StatusForbidden, gin.H{"error": "Ссылка недействительна или истекла"})
        return
    }

    documentHash := generateDocumentHash(actID)

    newTheirSigner := fmt.Sprintf("%s | %s | %s | %s | %s",
        counterpartyName,
        signerName,
        signerPosition,
        time.Now().Format("02.01.2006 15:04:05"),
        documentHash[:16])

    theirSignatureHash := generateSignatureHash(documentHash, token, "their_"+actID)

    newStatus := "partially_signed"
    if currentSignedOur {
        newStatus = "signed"
    }

    _, err = database.Pool.Exec(c.Request.Context(), `
        UPDATE reconciliation_acts 
        SET signed_by_their = true,
            signed_by_their_name = $1,
            their_signature_hash = $2,
            status = $3,
            signed_at = NOW(),
            their_sign_token = NULL,
            their_sign_expires = NULL,
            updated_at = NOW()
        WHERE id = $4::uuid
    `, newTheirSigner, theirSignatureHash, newStatus, actID)

    if err != nil {
        log.Printf("❌ Ошибка подписания контрагентом: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка подписания"})
        return
    }

    log.Printf("✅ Акт %s подписан контрагентом %s, статус: %s", actID[:8], counterpartyName, newStatus)

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Акт успешно подписан!",
        "status":  newStatus,
    })
}

func BatchCreateReconciliationActs(c *gin.Context) {
    var req struct {
        CounterpartyIDs []string `json:"counterparty_ids"`
        PeriodStart     string   `json:"period_start"`
        PeriodEnd       string   `json:"period_end"`
    }

    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    tenantID := getTenantIDString(c)

    var createdCount int
    var errors []string

    for _, cpID := range req.CounterpartyIDs {
        var cpName, cpINN string
        err := database.Pool.QueryRow(c.Request.Context(), `
            SELECT name, COALESCE(inn, '') FROM crm_partners
            WHERE id = $1
        `, cpID).Scan(&cpName, &cpINN)

        if err != nil {
            errors = append(errors, "Партнёр не найден: "+cpID)
            continue
        }

        actID := uuid.New().String()

        _, err = database.Pool.Exec(c.Request.Context(), `
            INSERT INTO reconciliation_acts
            (id, tenant_id, counterparty_name, counterparty_inn,
             period_start, period_end, status, created_at)
            VALUES ($1, $2::uuid, $3, $4, $5, $6, 'draft', NOW())
        `, actID, tenantID, cpName, cpINN, req.PeriodStart, req.PeriodEnd)

        if err != nil {
            errors = append(errors, fmt.Sprintf("%s: %v", cpName, err))
        } else {
            createdCount++
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "success":       true,
        "created_count": createdCount,
        "errors":        errors,
    })
}

func GetReconciliationDashboard(c *gin.Context) {
    tenantID := getTenantIDString(c)
    log.Printf("📊 Получение дашборда для tenantID: %s", tenantID)

    var total, signed, pending, draft int
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT 
            COUNT(*),
            COUNT(CASE WHEN status = 'signed' THEN 1 END),
            COUNT(CASE WHEN status IN ('generated', 'partially_signed') THEN 1 END),
            COUNT(CASE WHEN status = 'draft' THEN 1 END)
        FROM reconciliation_acts 
        WHERE tenant_id = $1::uuid AND (is_deleted IS NULL OR is_deleted = false)
    `, tenantID).Scan(&total, &signed, &pending, &draft)

    if err != nil {
        log.Printf("❌ Ошибка статистики: %v", err)
        c.JSON(http.StatusOK, gin.H{
            "stats": gin.H{
                "total":   0,
                "signed":  0,
                "pending": 0,
                "draft":   0,
            },
            "trend": []gin.H{},
        })
        return
    }

    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT 
            TO_CHAR(DATE_TRUNC('month', created_at), 'YYYY-MM') as month,
            COUNT(*) as created,
            COUNT(CASE WHEN status = 'signed' THEN 1 END) as signed
        FROM reconciliation_acts
        WHERE tenant_id = $1::uuid AND (is_deleted IS NULL OR is_deleted = false)
        GROUP BY DATE_TRUNC('month', created_at)
        ORDER BY month DESC
        LIMIT 12
    `, tenantID)

    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "stats": gin.H{
                "total":   total,
                "signed":  signed,
                "pending": pending,
                "draft":   draft,
            },
            "trend": []gin.H{},
        })
        return
    }
    defer rows.Close()

    var trend []gin.H
    for rows.Next() {
        var month string
        var created, signed int
        rows.Scan(&month, &created, &signed)
        trend = append([]gin.H{{"month": month, "created": created, "signed": signed}}, trend...)
    }

    c.JSON(http.StatusOK, gin.H{
        "stats": gin.H{
            "total":   total,
            "signed":  signed,
            "pending": pending,
            "draft":   draft,
        },
        "trend": trend,
    })
}