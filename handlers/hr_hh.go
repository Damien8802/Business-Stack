package handlers

import (
    "bytes"
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

// Структуры для работы с HH API
type HHToken struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
    ExpiresIn    int    `json:"expires_in"`
    TokenType    string `json:"token_type"`
}

type HHVacancyRequest struct {
    Title           string              `json:"title"`
    Description     string              `json:"description"`
    Employment      HHEmployment        `json:"employment"`
    Schedule        HHSchedule          `json:"schedule"`
    Experience      HHExperience        `json:"experience"`
    Area            HHArea              `json:"area"`
    Salary          *HHSalary           `json:"salary,omitempty"`
    Specializations []HHSpecialization  `json:"specializations"`
    Contacts        *HHContacts         `json:"contacts,omitempty"`
    BillingType     string              `json:"billing_type"`
}

type HHEmployment struct {
    ID string `json:"id"`
}

type HHSchedule struct {
    ID string `json:"id"`
}

type HHExperience struct {
    ID string `json:"id"`
}

type HHArea struct {
    ID string `json:"id"`
}

type HHSalary struct {
    From     float64 `json:"from"`
    To       float64 `json:"to"`
    Currency string  `json:"currency"`
    Gross    bool    `json:"gross"`
}

type HHSpecialization struct {
    ID           string `json:"id"`
    ProfareaID   string `json:"profarea_id"`
    SpecializID  string `json:"specializ_id"`
}

type HHContacts struct {
    Name   string `json:"name"`
    Email  string `json:"email"`
    Phone  string `json:"phone"`
}

// Конфигурация для HH API
const (
    HH_API_BASE = "https://api.hh.ru"
    HH_AUTH_URL = "https://hh.ru/oauth/token"
    HH_CLIENT_ID = "" // Заполните из .env
    HH_CLIENT_SECRET = "" // Заполните из .env
)

// Сохранение токена в БД
func saveHHToken(tenantID, accessToken, refreshToken string, expiresIn int) error {
    expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
    
    _, err := database.Pool.Exec(context.Background(), `
        INSERT INTO hr_hh_tokens (access_token, refresh_token, expires_at, tenant_id)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (tenant_id) DO UPDATE 
        SET access_token = EXCLUDED.access_token,
            refresh_token = EXCLUDED.refresh_token,
            expires_at = EXCLUDED.expires_at,
            updated_at = NOW()
    `, accessToken, refreshToken, expiresAt, tenantID)
    
    return err
}

// Получение токена доступа из БД
func getHHToken(tenantID string) (*HHToken, error) {
    var token HHToken
    var expiresAt time.Time
    
    err := database.Pool.QueryRow(context.Background(), `
        SELECT access_token, refresh_token, expires_at 
        FROM hr_hh_tokens WHERE tenant_id = $1
    `, tenantID).Scan(&token.AccessToken, &token.RefreshToken, &expiresAt)
    
    if err != nil {
        return nil, err
    }
    
    if time.Now().Add(5 * time.Minute).After(expiresAt) {
        return refreshHHToken(token.RefreshToken, tenantID)
    }
    
    return &token, nil
}

// Обновление токена
func refreshHHToken(refreshToken, tenantID string) (*HHToken, error) {
    data := map[string]string{
        "grant_type":    "refresh_token",
        "refresh_token": refreshToken,
        "client_id":     HH_CLIENT_ID,
        "client_secret": HH_CLIENT_SECRET,
    }
    
    jsonData, _ := json.Marshal(data)
    
    resp, err := http.Post(HH_AUTH_URL, "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var token HHToken
    body, _ := io.ReadAll(resp.Body)
    json.Unmarshal(body, &token)
    
    _, err = database.Pool.Exec(context.Background(), `
        UPDATE hr_hh_tokens 
        SET access_token = $1, refresh_token = $2, expires_at = NOW() + INTERVAL '1 hour', updated_at = NOW()
        WHERE tenant_id = $3
    `, token.AccessToken, token.RefreshToken, tenantID)
    
    if err != nil {
        return nil, err
    }
    
    return &token, nil
}

// Авторизация в HH с паролем
func authenticateHHWithPassword(email, password string) (*HHToken, error) {
    data := map[string]string{
        "grant_type":    "password",
        "username":      email,
        "password":      password,
        "client_id":     HH_CLIENT_ID,
        "client_secret": HH_CLIENT_SECRET,
    }
    
    jsonData, _ := json.Marshal(data)
    
    resp, err := http.Post(HH_AUTH_URL, "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("ошибка авторизации: %s", string(body))
    }
    
    var token HHToken
    json.Unmarshal(body, &token)
    
    return &token, nil
}

// Получение справочников HH
func GetHHReferences(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    token, err := getHHToken(tenantID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось получить токен"})
        return
    }
    
    references := make(map[string]interface{})
    
    // Типы занятости
    employment, _ := getHHReference(token.AccessToken, "/employments")
    references["employments"] = employment
    
    // Графики работы
    schedules, _ := getHHReference(token.AccessToken, "/schedules")
    references["schedules"] = schedules
    
    // Опыт работы
    experience, _ := getHHReference(token.AccessToken, "/experience")
    references["experiences"] = experience
    
    // Регионы
    areas, _ := getHHReference(token.AccessToken, "/areas")
    references["areas"] = areas
    
    c.JSON(http.StatusOK, gin.H{"references": references})
}

// Вспомогательная функция для получения справочников
func getHHReference(accessToken, endpoint string) ([]map[string]interface{}, error) {
    req, _ := http.NewRequest("GET", HH_API_BASE+endpoint, nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)
    req.Header.Set("User-Agent", "Business Stack/1.0")
    
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    var result []map[string]interface{}
    json.Unmarshal(body, &result)
    
    return result, nil
}

// Публикация вакансии в HH от имени клиента
func PublishVacancyToHHClient(c *gin.Context) {
    var req struct {
        VacancyID string `json:"vacancy_id" binding:"required"`
        Email     string `json:"email" binding:"required"`
        Password  string `json:"password" binding:"required"`
        Remember  bool   `json:"remember"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    tenantID := c.GetString("tenant_id")
    if tenantID == "" {
        tenantID = "11111111-1111-1111-1111-111111111111"
    }
    
    var vacancy struct {
        ID          string
        Title       string
        Description string
        Department  string
        SalaryMin   float64
        SalaryMax   float64
        Location    string
        IsRemote    bool
        Requirements string
    }
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT id, title, description, department, salary_min, salary_max, location, is_remote, requirements
        FROM hr_vacancies WHERE id = $1 AND tenant_id = $2
    `, req.VacancyID, tenantID).Scan(
        &vacancy.ID, &vacancy.Title, &vacancy.Description, &vacancy.Department,
        &vacancy.SalaryMin, &vacancy.SalaryMax, &vacancy.Location, &vacancy.IsRemote, &vacancy.Requirements,
    )
    
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Вакансия не найдена"})
        return
    }
    
    token, err := authenticateHHWithPassword(req.Email, req.Password)
    if err != nil {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Ошибка авторизации в HeadHunter: " + err.Error()})
        return
    }
    
    if req.Remember {
        saveHHToken(tenantID, token.AccessToken, token.RefreshToken, token.ExpiresIn)
    }
    
    hhVacancy := HHVacancyRequest{
        Title:       vacancy.Title,
        Description: vacancy.Description + "\n\n" + vacancy.Requirements,
        Employment:  HHEmployment{ID: "full"},
        Schedule:    HHSchedule{ID: "fullDay"},
        Experience:  HHExperience{ID: "between1And3"},
        Area:        HHArea{ID: "113"},
        BillingType: "standard",
    }
    
    if vacancy.SalaryMin > 0 || vacancy.SalaryMax > 0 {
        hhVacancy.Salary = &HHSalary{
            From:     vacancy.SalaryMin,
            To:       vacancy.SalaryMax,
            Currency: "RUR",
            Gross:    true,
        }
    }
    
    jsonData, _ := json.Marshal(hhVacancy)
    
    reqHH, _ := http.NewRequest("POST", HH_API_BASE+"/vacancies", bytes.NewBuffer(jsonData))
    reqHH.Header.Set("Authorization", "Bearer "+token.AccessToken)
    reqHH.Header.Set("Content-Type", "application/json")
    reqHH.Header.Set("User-Agent", "Business Stack/1.0")
    
    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(reqHH)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка связи с HH: " + err.Error()})
        return
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    
    if resp.StatusCode != 201 {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error":  "Ошибка публикации в HH",
            "status": resp.StatusCode,
            "body":   string(body),
        })
        return
    }
    
    var hhResp struct {
        ID string `json:"id"`
    }
    json.Unmarshal(body, &hhResp)
    
    _, err = database.Pool.Exec(c.Request.Context(), `
        INSERT INTO hr_hh_vacancies (vacancy_id, hh_vacancy_id, status, published_at, tenant_id)
        VALUES ($1, $2, 'published', NOW(), $3)
        ON CONFLICT (vacancy_id) DO UPDATE 
        SET hh_vacancy_id = $2, status = 'published', published_at = NOW(), updated_at = NOW()
    `, vacancy.ID, hhResp.ID, tenantID)
    
    if err != nil {
        log.Printf("Ошибка сохранения связи: %v", err)
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Вакансия успешно опубликована в HeadHunter",
        "hh_url":  fmt.Sprintf("https://hh.ru/vacancy/%s", hhResp.ID),
    })
}

// Получение статуса публикации
func GetVacancyPublishStatus(c *gin.Context) {
    vacancyID := c.Param("id")
    tenantID := c.GetString("tenant_id")
    
    var hhVacancyID string
    var status string
    var publishedAt time.Time
    
    err := database.Pool.QueryRow(c.Request.Context(), `
        SELECT hh_vacancy_id, status, published_at
        FROM hr_hh_vacancies WHERE vacancy_id = $1 AND tenant_id = $2
    `, vacancyID, tenantID).Scan(&hhVacancyID, &status, &publishedAt)
    
    if err != nil {
        c.JSON(http.StatusOK, gin.H{
            "published": false,
            "status":    "not_published",
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "published":     status == "published",
        "status":        status,
        "hh_vacancy_id": hhVacancyID,
        "published_at":  publishedAt,
        "hh_url":        fmt.Sprintf("https://hh.ru/vacancy/%s", hhVacancyID),
    })
}