package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"
)

func SupportPageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "support.html", gin.H{
        "Title": "Поддержка - Business Stack",
    })
}