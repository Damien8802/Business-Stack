package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"
)

// AISettingsPageHandler отображает страницу настроек AI
func AISettingsPageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "ai_settings.html", gin.H{
        "Title": "Настройки AI | Business Stack",
    })
}