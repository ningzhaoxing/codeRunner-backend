package handler

import (
	"context"
	"net/http"
	"strings"

	"codeRunner-siwu/internal/agent"

	"github.com/gin-gonic/gin"
)

type cancelRequest struct {
	SessionID string `json:"session_id"`
}

func CancelHandler(svc *agent.AgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req cancelRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
			return
		}
		if strings.TrimSpace(req.SessionID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "session_id is required"})
			return
		}

		cancelFn, ok := svc.Cancels.LoadAndDelete(req.SessionID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"message": "no active run for this session"})
			return
		}

		cancelFn.(context.CancelFunc)()
		c.JSON(http.StatusOK, gin.H{"message": "cancelled"})
	}
}
