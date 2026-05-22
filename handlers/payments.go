package handlers

import (
    
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// ==================== ПЛАТЕЖНЫЕ СТРАНИЦЫ ====================
func BankCardPaymentHandler(c *gin.Context) {
    // // tenantID := middleware.GetTenantIDFromContext(c)
	c.HTML(http.StatusOK, "bank_card_payment.html", gin.H{
		"Title":   "Оплата картой - Business Stack",
		"Version": "3.0",
		"Time":    time.Now().Format("2006-01-02 15:04:05"),
	})
}

func PaymentSuccessHandler(c *gin.Context) {
    // // tenantID := middleware.GetTenantIDFromContext(c)
	c.HTML(http.StatusOK, "payment-success.html", gin.H{
		"Title":   "Успешная оплата - Business Stack",
		"Version": "3.0",
		"Time":    time.Now().Format("2006-01-02 15:04:05"),
	})
}

func USDTPaymentHandler(c *gin.Context) {
    // // tenantID := middleware.GetTenantIDFromContext(c)
	c.HTML(http.StatusOK, "usdt-payment.html", gin.H{
		"Title":   "Оплата USDT - Business Stack",
		"Version": "3.0",
		"Time":    time.Now().Format("2006-01-02 15:04:05"),
	})
}

func RUBPaymentHandler(c *gin.Context) {
    // // tenantID := middleware.GetTenantIDFromContext(c)
	c.HTML(http.StatusOK, "rub-payment.html", gin.H{
		"Title":   "Оплата RUB - Business Stack",
		"Version": "3.0",
		"Time":    time.Now().Format("2006-01-02 15:04:05"),
	})
}




