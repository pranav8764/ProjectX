package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HandleGetMe returns information about the currently authenticated user session.
func HandleGetMe() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"userId":  c.GetString("userID"),
			"orgId":   c.GetString("orgID"),
			"plantId": c.GetString("plantID"),
			"role":    c.GetString("role"),
		})
	}
}
