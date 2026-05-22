package handlers

import (
"net/http"
"subscription-system/models"

"github.com/gin-gonic/gin"
)

// AdminDashboardHandler отображает главную страницу админ-панели со статистикой
func AdminDashboardHandler(c *gin.Context) {
stats, err := models.GetAdminStats()
if err != nil {
c.HTML(http.StatusOK, "admin_dashboard.html", gin.H{
"Title":   "Админ-панель - Business Stack",
"Version": "3.0",
"Stats":   &models.AdminStats{},
"Error":   "Не удалось загрузить статистику",
})
return
}
c.HTML(http.StatusOK, "admin_dashboard.html", gin.H{
"Title":   "Админ-панель - Business Stack",
"Version": "3.0",
"Stats":   stats,
})
}
