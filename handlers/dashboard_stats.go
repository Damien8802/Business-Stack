package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"
)

// DashboardStatsHandler отображает страницу статистики
func DashboardStatsHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "dashboard_stats.html", gin.H{
        "Title": "Дашборд статистики | Business Stack",
    })
}