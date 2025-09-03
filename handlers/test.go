package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// TestHandler handles the /test endpoint
func TestHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Hello from api.sjaplus.top",
	})
}
