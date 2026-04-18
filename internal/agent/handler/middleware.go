package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func AgentAPIKeyMiddleware(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expected == "" || c.GetHeader("X-Agent-API-Key") != expected {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}
