package handler

import (
	"net/http"

	"codeRunner-siwu/internal/agent"

	"github.com/gin-gonic/gin"
)

// ListSessionsHandler returns metadata for all active sessions.
// GET /agent/sessions
func ListSessionsHandler(svc *agent.AgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessions := svc.SessionStore.ListSessions()
		c.JSON(http.StatusOK, gin.H{"sessions": sessions})
	}
}

// GetSessionMessagesHandler returns all messages for a given session.
// GET /agent/sessions/:id/messages
func GetSessionMessagesHandler(svc *agent.AgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("id")
		if _, ok := svc.SessionStore.GetMeta(sessionID); !ok {
			c.JSON(http.StatusNotFound, gin.H{"message": "session not found or expired"})
			return
		}
		msgs, err := svc.SessionStore.GetMessages(sessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to load messages"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"session_id": sessionID, "messages": msgs})
	}
}
