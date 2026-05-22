package handlers

import (
"net/http"
"subscription-system/models"

"github.com/gin-gonic/gin"
)

// AdminAIStatsHandler отображает страницу статистики AI-запросов
func AdminAIStatsHandler(c *gin.Context) {
period := c.DefaultQuery("period", "week")

stats, err := models.AdminGetAIStats(period)
if err != nil {
c.HTML(http.StatusInternalServerError, "admin_ai_stats.html", gin.H{
"Title": "Ошибка",
"Error": "Не удалось загрузить статистику",
})
return
}

c.HTML(http.StatusOK, "admin_ai_stats.html", gin.H{
"Title":      "Статистика AI-запросов - Business Stack",
"Version":    "3.0",
"Stats":      stats,
"Period":     period,
})
}
