package handlers

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "subscription-system/database"
)

// ==================== УНИВЕРСАЛЬНЫЙ СБОРЩИК ОТКЛИКОВ ====================

// Структуры для HH откликов
type HHNegotiation struct {
    ID           string `json:"id"`
    State        string `json:"state"`
    CreatedAt    string `json:"created_at"`
    Applicant    struct {
        ID         string `json:"id"`
        FirstName  string `json:"first_name"`
        LastName   string `json:"last_name"`
        MiddleName string `json:"middle_name"`
    } `json:"applicant"`
    Resume       struct {
        ID    string `json:"id"`
        Title string `json:"title"`
        URL   string `json:"url"`
    } `json:"resume"`
    Message      struct {
        Text string `json:"text"`
    } `json:"message"`
}

// Структуры для Avito откликов
type AvitoResponse struct {
    ID          string `json:"id"`
    FromUser    struct {
        ID    string `json:"id"`
        Name  string `json:"name"`
        Phone string `json:"phone"`
    } `json:"from_user"`
    Message     string `json:"message"`
    CreatedAt   string `json:"created_at"`
    IsRead      bool   `json:"is_read"`
}

// Вспомогательная функция для парсинга времени
func parseTime(timeStr string) time.Time {
    layouts := []string{
        time.RFC3339,
        "2006-01-02T15:04:05-07:00",
        "2006-01-02T15:04:05Z",
        "2006-01-02 15:04:05",
    }
    
    for _, layout := range layouts {
        if t, err := time.Parse(layout, timeStr); err == nil {
            return t
        }
    }
    return time.Now()
}

// Получение откликов с HH
func fetchHHResponses(accessToken, vacancyID, hhVacancyID string) ([]HHNegotiation, error) {
    req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.hh.ru/negotiations?vacancy_id=%s", hhVacancyID), nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)
    req.Header.Set("User-Agent", "Business Stack/1.0")
    
    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    
    var result struct {
        Items []HHNegotiation `json:"items"`
    }
    json.Unmarshal(body, &result)
    
    return result.Items, nil
}

// Получение откликов с Avito
func fetchAvitoResponses(accessToken, vacancyID string) ([]AvitoResponse, error) {
    req, _ := http.NewRequest("GET", "https://api.avito.ru/messenger/v1/threads", nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)
    
    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    
    var result struct {
        Threads []struct {
            ID        string `json:"id"`
            LastMessage struct {
                Text      string `json:"text"`
                CreatedAt string `json:"created_at"`
                AuthorID  string `json:"author_id"`
            } `json:"last_message"`
            Participants []struct {
                ID   string `json:"id"`
                Name string `json:"name"`
            } `json:"participants"`
        } `json:"threads"`
    }
    json.Unmarshal(body, &result)
    
    var responses []AvitoResponse
    for _, thread := range result.Threads {
        var candidateName string
        for _, p := range thread.Participants {
            if p.ID != "me" {
                candidateName = p.Name
            }
        }
        
        responses = append(responses, AvitoResponse{
            ID: thread.ID,
            FromUser: struct {
                ID    string `json:"id"`
                Name  string `json:"name"`
                Phone string `json:"phone"`
            }{Name: candidateName},
            Message:   thread.LastMessage.Text,
            CreatedAt: thread.LastMessage.CreatedAt,
            IsRead:    false,
        })
    }
    
    return responses, nil
}

// Сохранение откликов в БД
func saveResponsesToDB(ctx context.Context, vacancyID, platform string, responses interface{}, tenantID string) (int, error) {
    newCount := 0
    
    switch platform {
    case "hh":
        hhResponses, ok := responses.([]HHNegotiation)
        if !ok {
            return 0, nil
        }
        
        for _, resp := range hhResponses {
            var exists bool
            database.Pool.QueryRow(ctx, `
                SELECT EXISTS(SELECT 1 FROM hr_platform_responses 
                WHERE platform = $1 AND platform_response_id = $2 AND tenant_id = $3)
            `, platform, resp.ID, tenantID).Scan(&exists)
            
            if exists {
                continue
            }
            
            fullName := resp.Applicant.FirstName + " " + resp.Applicant.LastName
            if resp.Applicant.MiddleName != "" {
                fullName += " " + resp.Applicant.MiddleName
            }
            
            _, err := database.Pool.Exec(ctx, `
                INSERT INTO hr_platform_responses 
                (vacancy_id, platform, platform_response_id, candidate_name, cover_letter, 
                 status, responded_at, external_data, tenant_id)
                VALUES ($1, $2, $3, $4, $5, 'new', $6, $7, $8)
            `, vacancyID, platform, resp.ID, fullName, resp.Message.Text,
                parseTime(resp.CreatedAt), map[string]interface{}{
                    "resume_url": resp.Resume.URL,
                    "resume_title": resp.Resume.Title,
                    "state": resp.State,
                }, tenantID)
            
            if err == nil {
                newCount++
            }
        }
        
    case "avito":
        avitoResponses, ok := responses.([]AvitoResponse)
        if !ok {
            return 0, nil
        }
        
        for _, resp := range avitoResponses {
            var exists bool
            database.Pool.QueryRow(ctx, `
                SELECT EXISTS(SELECT 1 FROM hr_platform_responses 
                WHERE platform = $1 AND platform_response_id = $2 AND tenant_id = $3)
            `, platform, resp.ID, tenantID).Scan(&exists)
            
            if exists {
                continue
            }
            
            _, err := database.Pool.Exec(ctx, `
                INSERT INTO hr_platform_responses 
                (vacancy_id, platform, platform_response_id, candidate_name, candidate_phone,
                 cover_letter, status, responded_at, external_data, tenant_id)
                VALUES ($1, $2, $3, $4, $5, $6, 'new', $7, $8, $9)
            `, vacancyID, platform, resp.ID, resp.FromUser.Name, resp.FromUser.Phone,
                resp.Message, parseTime(resp.CreatedAt), map[string]interface{}{
                    "is_read": resp.IsRead,
                }, tenantID)
            
            if err == nil {
                newCount++
            }
        }
    }
    
    return newCount, nil
}

// Синхронизация откликов со всех платформ
func SyncAllPlatformResponses(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    // Получаем все активные сессии платформ
    rows, err := database.Pool.Query(c.Request.Context(), `
        SELECT platform, access_token, external_user_id
        FROM hr_platform_sessions 
        WHERE tenant_id = $1 AND is_active = true
    `, tenantID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var sessions []struct {
        Platform    string
        AccessToken string
        ExternalID  string
    }
    
    for rows.Next() {
        var s struct {
            Platform    string
            AccessToken string
            ExternalID  string
        }
        rows.Scan(&s.Platform, &s.AccessToken, &s.ExternalID)
        sessions = append(sessions, s)
    }
    
    // Получаем все вакансии, опубликованные на платформах
    vacancyRows, err := database.Pool.Query(c.Request.Context(), `
        SELECT v.id, v.title, 
               COALESCE(hh.hh_vacancy_id, '') as hh_id,
               COALESCE(avito.platform_vacancy_id, '') as avito_id,
               COALESCE(rabota.platform_vacancy_id, '') as rabota_id
        FROM hr_vacancies v
        LEFT JOIN hr_hh_vacancies hh ON v.id = hh.vacancy_id
        LEFT JOIN hr_platform_vacancies avito ON v.id = avito.vacancy_id AND avito.platform = 'avito'
        LEFT JOIN hr_platform_vacancies rabota ON v.id = rabota.vacancy_id AND rabota.platform = 'rabota'
        WHERE v.tenant_id = $1 AND v.status = 'open'
    `, tenantID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer vacancyRows.Close()
    
    var vacancies []struct {
        ID       string
        Title    string
        HhID     string
        AvitoID  string
        RabotaID string
    }
    
    for vacancyRows.Next() {
        var v struct {
            ID       string
            Title    string
            HhID     string
            AvitoID  string
            RabotaID string
        }
        vacancyRows.Scan(&v.ID, &v.Title, &v.HhID, &v.AvitoID, &v.RabotaID)
        vacancies = append(vacancies, v)
    }
    
    totalNew := 0
    
    // Синхронизируем каждую платформу
    for _, session := range sessions {
        for _, vacancy := range vacancies {
            var platformVacancyID string
            switch session.Platform {
            case "hh":
                platformVacancyID = vacancy.HhID
            case "avito":
                platformVacancyID = vacancy.AvitoID
            case "rabota":
                platformVacancyID = vacancy.RabotaID
            }
            
            if platformVacancyID == "" {
                continue
            }
            
            var responses interface{}
            var err error
            
            switch session.Platform {
            case "hh":
                responses, err = fetchHHResponses(session.AccessToken, vacancy.ID, platformVacancyID)
            case "avito":
                responses, err = fetchAvitoResponses(session.AccessToken, vacancy.ID)
            }
            
            if err != nil {
                log.Printf("❌ Ошибка синхронизации %s: %v", session.Platform, err)
                continue
            }
            
            newCount, _ := saveResponsesToDB(c.Request.Context(), vacancy.ID, session.Platform, responses, tenantID)
            totalNew += newCount
        }
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": fmt.Sprintf("Синхронизация завершена. Новых откликов: %d", totalNew),
        "new_responses": totalNew,
    })
}

// Получение всех откликов
func GetPlatformResponses(c *gin.Context) {
    vacancyID := c.Query("vacancy_id")
    platform := c.Query("platform")
    status := c.Query("status")
    tenantID := c.GetString("tenant_id")
    
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    query := `
        SELECT id, vacancy_id, platform, platform_response_id, candidate_name, 
               candidate_email, candidate_phone, cover_letter, expected_salary,
               status, viewed, responded_at, created_at, external_data
        FROM hr_platform_responses 
        WHERE tenant_id = $1
    `
    args := []interface{}{tenantID}
    argIdx := 2
    
    if vacancyID != "" {
        query += fmt.Sprintf(" AND vacancy_id = $%d", argIdx)
        args = append(args, vacancyID)
        argIdx++
    }
    if platform != "" {
        query += fmt.Sprintf(" AND platform = $%d", argIdx)
        args = append(args, platform)
        argIdx++
    }
    if status != "" {
        query += fmt.Sprintf(" AND status = $%d", argIdx)
        args = append(args, status)
        argIdx++
    }
    
    query += " ORDER BY created_at DESC"
    
    rows, err := database.Pool.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()
    
    var responses []gin.H
    for rows.Next() {
        var id, vacancyIDStr, platformStr, platformRespID, candidateName, candidateEmail, candidatePhone, coverLetter, statusStr string
        var expectedSalary *float64
        var viewed bool
        var respondedAt, createdAt time.Time
        var externalData json.RawMessage
        
        rows.Scan(&id, &vacancyIDStr, &platformStr, &platformRespID, &candidateName,
            &candidateEmail, &candidatePhone, &coverLetter, &expectedSalary,
            &statusStr, &viewed, &respondedAt, &createdAt, &externalData)
        
        response := gin.H{
            "id":                 id,
            "vacancy_id":         vacancyIDStr,
            "platform":           platformStr,
            "candidate_name":     candidateName,
            "candidate_email":    candidateEmail,
            "candidate_phone":    candidatePhone,
            "cover_letter":       coverLetter,
            "status":             statusStr,
            "viewed":             viewed,
            "responded_at":       respondedAt,
            "created_at":         createdAt,
        }
        
        if expectedSalary != nil {
            response["expected_salary"] = *expectedSalary
        }
        
        var extData map[string]interface{}
        if err := json.Unmarshal(externalData, &extData); err == nil {
            response["external_data"] = extData
        }
        
        responses = append(responses, response)
    }
    
    c.JSON(http.StatusOK, gin.H{"responses": responses})
}

// Получение количества новых откликов
func GetNewResponsesCount(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    var count int
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT COUNT(*) FROM hr_platform_responses 
        WHERE tenant_id = $1 AND status = 'new' AND viewed = false
    `, tenantID).Scan(&count)
    
    if err != nil {
        log.Printf("Ошибка получения count: %v", err)
        c.JSON(http.StatusOK, gin.H{"new_count": 0})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"new_count": count})
}

// Получение одного отклика по ID
func GetResponseByID(c *gin.Context) {
    responseID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    var id, vacancyID, platform, candidateName, candidateEmail, candidatePhone, coverLetter, status string
    var viewed bool
    var respondedAt, createdAt time.Time
    var externalData []byte
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, vacancy_id, platform, candidate_name, candidate_email, candidate_phone,
               cover_letter, status, viewed, responded_at, created_at, COALESCE(external_data::text, '{}')
        FROM hr_platform_responses 
        WHERE id = $1 AND tenant_id = $2
    `, responseID, tenantID).Scan(
        &id, &vacancyID, &platform, &candidateName, &candidateEmail, &candidatePhone,
        &coverLetter, &status, &viewed, &respondedAt, &createdAt, &externalData,
    )
    
    if err != nil {
        log.Printf("Ошибка получения отклика: %v", err)
        c.JSON(http.StatusNotFound, gin.H{"error": "Отклик не найден"})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"response": gin.H{
        "id":              id,
        "vacancy_id":      vacancyID,
        "platform":        platform,
        "candidate_name":  candidateName,
        "candidate_email": candidateEmail,
        "candidate_phone": candidatePhone,
        "cover_letter":    coverLetter,
        "status":          status,
        "viewed":          viewed,
        "responded_at":    respondedAt,
        "created_at":      createdAt,
        "external_data":   externalData,
    }})
}

// Обновление статуса отклика
func UpdateResponseStatus(c *gin.Context) {
    responseID := c.Param("id")
    var req struct {
        Status string `json:"status" binding:"required"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE hr_platform_responses 
        SET status = $1, updated_at = NOW()
        WHERE id = $2 AND tenant_id = $3
    `, req.Status, responseID, tenantID)
    
    if err != nil {
        log.Printf("Ошибка обновления статуса: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}

// Отметить отклик как просмотренный
func MarkResponseViewed(c *gin.Context) {
    responseID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    _, err := database.Pool.Exec(c.Request.Context(), `
        UPDATE hr_platform_responses 
        SET viewed = true, updated_at = NOW()
        WHERE id = $1 AND tenant_id = $2
    `, responseID, tenantID)
    
    if err != nil {
        log.Printf("Ошибка отметки просмотра: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(http.StatusOK, gin.H{"success": true})
}