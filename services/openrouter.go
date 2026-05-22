package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"subscription-system/config"
)

// OpenRouterService - сервис для работы с OpenRouter API
type OpenRouterService struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// NewOpenRouterService - создает новый экземпляр OpenRouterService
func NewOpenRouterService(cfg *config.Config) *OpenRouterService {
	return &OpenRouterService{
		APIKey:  cfg.OpenRouterAPIKey,
		BaseURL: "https://openrouter.ai/api/v1",
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Ask - отправляет запрос к OpenRouter API
func (s *OpenRouterService) Ask(prompt string, model string, temperature float64) (string, error) {
	if s.APIKey == "" {
		return "API ключ OpenRouter не настроен", nil
	}

	if model == "" {
		model = "openrouter/auto"
	}
	if temperature == 0 {
		temperature = 0.7
	}

	requestBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": temperature,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга запроса: %v", err)
	}

	req, err := http.NewRequest("POST", s.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("HTTP-Referer", "https://Business Stack.local")
	req.Header.Set("X-Title", "Business Stack CRM")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ошибка выполнения запроса: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenRouter API вернул ошибку %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("ошибка парсинга ответа: %v", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("пустой ответ от OpenRouter")
	}

	return response.Choices[0].Message.Content, nil
}