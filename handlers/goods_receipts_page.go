package handlers

import (
    "net/http"
    
    "github.com/gin-gonic/gin"
)

// GoodsReceiptsPageHandler - страница приемки товаров
func GoodsReceiptsPageHandler(c *gin.Context) {
    c.HTML(http.StatusOK, "goods_receipts.html", gin.H{
        "title": "Приемка товаров | Business Stack",
    })
}
