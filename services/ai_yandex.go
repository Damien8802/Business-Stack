package services

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
    "time"

    "subscription-system/config"
)

type YandexAIService struct {
    cfg *config.Config
}

func NewYandexAIService(cfg *config.Config) *YandexAIService {
    return &YandexAIService{cfg: cfg}
}

// YandexGPTRequest структура запроса к YandexGPT
type YandexGPTRequest struct {
    ModelUri          string `json:"modelUri"`
    CompletionOptions struct {
        Stream      bool    `json:"stream"`
        Temperature float64 `json:"temperature"`
        MaxTokens   int     `json:"maxTokens"`
    } `json:"completionOptions"`
    Messages []YandexGPTMessage `json:"messages"`
}

type YandexGPTMessage struct {
    Role    string `json:"role"`
    Text    string `json:"text"`
}

// YandexGPTResponse структура ответа
type YandexGPTResponse struct {
    Result struct {
        Alternatives []struct {
            Message YandexGPTMessage `json:"message"`
        } `json:"alternatives"`
    } `json:"result"`
}

// Ask отправляет вопрос к YandexGPT и возвращает ответ
func (s *YandexAIService) Ask(ctx context.Context, prompt string) (string, error) {
    url := "https://llm.api.cloud.yandex.net/foundationModels/v1/completion"
    
    reqBody := YandexGPTRequest{
        ModelUri: fmt.Sprintf("gpt://%s/yandexgpt-lite", s.cfg.YandexFolderID),
        CompletionOptions: struct {
            Stream      bool    `json:"stream"`
            Temperature float64 `json:"temperature"`
            MaxTokens   int     `json:"maxTokens"`
        }{
            Stream:      false,
            Temperature: 0.6,
            MaxTokens:   2000,
        },
        Messages: []YandexGPTMessage{
            {Role: "system", Text: "Ты — AI-ассистент CRM. Отвечай кратко и по делу, используя предоставленные данные."},
            {Role: "user", Text: prompt},
        },
    }

    jsonData, err := json.Marshal(reqBody)
    if err != nil {
        return "", fmt.Errorf("failed to marshal request: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        return "", fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Api-Key "+s.cfg.YandexAPIKey)

    client := &http.Client{Timeout: 30 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("failed to send request: %w", err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("failed to read response: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("YandexGPT returned status %d: %s", resp.StatusCode, string(body))
    }

    var yandexResp YandexGPTResponse
    if err := json.Unmarshal(body, &yandexResp); err != nil {
        return "", fmt.Errorf("failed to unmarshal response: %w", err)
    }

    if len(yandexResp.Result.Alternatives) == 0 {
        return "", fmt.Errorf("no alternatives in response")
    }

    return yandexResp.Result.Alternatives[0].Message.Text, nil
}

// NewYandexServiceWithKeys создаёт сервис YandexGPT с указанными ключами
func NewYandexServiceWithKeys(apiKey, folderID string) *YandexAdapter {
    cfg := &config.Config{
        YandexAPIKey:   apiKey,
        YandexFolderID: folderID,
    }
    return NewYandexService(cfg)
}


