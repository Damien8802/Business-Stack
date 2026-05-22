package handlers

import (
"net/http"
"strconv"
"subscription-system/models"

"github.com/gin-gonic/gin"
)

// AdminAPIKeysHandler отображает страницу со списком всех ключей
func AdminAPIKeysHandler(c *gin.Context) {
page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
search := c.Query("search")
if page < 1 {
page = 1
}
offset := (page - 1) * limit

keys, total, err := models.GetAllAPIKeys(search, limit, offset)
if err != nil {
c.HTML(http.StatusInternalServerError, "admin_api_keys.html", gin.H{
"Title": "Ошибка",
"Error": "Не удалось загрузить API-ключи",
})
return
}

totalPages := (int(total) + limit - 1) / limit

c.HTML(http.StatusOK, "admin_api_keys.html", gin.H{
"Title":      "Управление API-ключами - Business Stack",
"Version":    "3.0",
"Keys":       keys,
"Page":       page,
"Limit":      limit,
"Total":      total,
"TotalPages": totalPages,
"Search":     search,
})
}

// ---------- API для админки ----------
// AdminUpdateAPIKeyHandler – обновление лимита и статуса
func AdminUpdateAPIKeyHandler(c *gin.Context) {
keyID := c.Param("id")
var req struct {
QuotaLimit *int64 `json:"quota_limit"`
IsActive   *bool  `json:"is_active"`
}
if err := c.ShouldBindJSON(&req); err != nil {
c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
return
}
err := models.UpdateAPIKey(keyID, req.QuotaLimit, req.IsActive, nil)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось обновить ключ"})
return
}
c.JSON(http.StatusOK, gin.H{"message": "Ключ обновлён"})
}

// AdminDeleteAPIKeyHandler – удаление ключа
func AdminDeleteAPIKeyHandler(c *gin.Context) {
keyID := c.Param("id")
err := models.DeleteAPIKey(keyID)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось удалить ключ"})
return
}
c.JSON(http.StatusOK, gin.H{"message": "Ключ удалён"})
}
