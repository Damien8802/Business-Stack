package handlers

import (
"net/http"
"subscription-system/models"

"github.com/gin-gonic/gin"
)

// ProfileAIUsageHandler отображает страницу использования AI для текущего пользователя
func ProfileAIUsageHandler(c *gin.Context) {
userID, exists := c.Get("userID")
if !exists {
c.Redirect(http.StatusFound, "/login")
return
}

totalTokens, totalRequests, err := models.GetUserAITotals(userID.(string))
if err != nil {
totalTokens = 0
totalRequests = 0
}

byModel, err := models.GetUserAIUsageByModel(userID.(string))
if err != nil {
byModel = []struct {
Model    string `json:"model"`
Requests int64  `json:"requests"`
Tokens   int64  `json:"tokens"`
}{}
}

// Получаем активные ключи и их квоты
keys, err := models.GetAPIKeysByUser(userID.(string))
if err != nil {
keys = []models.APIKey{}
}

c.HTML(http.StatusOK, "profile_ai_usage.html", gin.H{
"Title":         "Мои AI-запросы - Business Stack",
"Version":       "3.0",
"TotalTokens":   totalTokens,
"TotalRequests": totalRequests,
"ByModel":       byModel,
"Keys":          keys,
})
}
