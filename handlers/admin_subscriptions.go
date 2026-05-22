package handlers

import (
"net/http"
"strconv"
"subscription-system/models"

"github.com/gin-gonic/gin"
)

// AdminSubscriptionsHandler отображает страницу со списком подписок
func AdminSubscriptionsHandler(c *gin.Context) {
page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
search := c.Query("search")
status := c.Query("status")
if page < 1 {
page = 1
}
offset := (page - 1) * limit

subs, total, err := models.GetAdminSubscriptions(search, status, limit, offset)
if err != nil {
c.HTML(http.StatusInternalServerError, "admin_subscriptions.html", gin.H{
"Title": "Ошибка",
"Error": "Не удалось загрузить подписки",
})
return
}

stats, _ := models.GetSubscriptionStats()
totalPages := (int(total) + limit - 1) / limit

c.HTML(http.StatusOK, "admin_subscriptions.html", gin.H{
"Title":      "Управление подписками - Business Stack",
"Version":    "3.0",
"Subs":       subs,
"Page":       page,
"Limit":      limit,
"Total":      total,
"TotalPages": totalPages,
"Search":     search,
"Status":     status,
"Stats":      stats,
})
}

// ---------- API для управления подписками ----------
// AdminCancelSubscriptionHandler – отмена подписки
func AdminCancelSubscriptionHandler(c *gin.Context) {
subID := c.Param("id")
var req struct {
Immediate bool `json:"immediate"`
}
if err := c.ShouldBindJSON(&req); err != nil {
req.Immediate = false // по умолчанию – отмена в конце периода
}
err := models.AdminCancelSubscription(subID, req.Immediate)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось отменить подписку"})
return
}
c.JSON(http.StatusOK, gin.H{"message": "Подписка отменена"})
}

// AdminReactivateSubscriptionHandler – повторная активация (отмена отмены)
func AdminReactivateSubscriptionHandler(c *gin.Context) {
subID := c.Param("id")
err := models.AdminReactivateSubscription(subID)
if err != nil {
c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось активировать подписку"})
return
}
c.JSON(http.StatusOK, gin.H{"message": "Подписка реактивирована"})
}
