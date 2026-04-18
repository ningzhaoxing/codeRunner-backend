package server

import (
	"net/http"
	"time"

	"codeRunner-siwu/api/proto"
	serverService "codeRunner-siwu/internal/application/service/server"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ExecuteRequest struct {
	Language  string `json:"language" binding:"required"`
	CodeBlock string `json:"code_block" binding:"required"`
}

type ExecuteResponse struct {
	ID       string `json:"id"`
	Result   string `json:"result"`
	Err      string `json:"err,omitempty"`
	Language string `json:"language"`
}

func ExecuteHandler(svc serverService.ServerService, timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ExecuteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request: " + err.Error()})
			return
		}

		if len(req.CodeBlock) > maxCodeBlockSize {
			c.JSON(http.StatusBadRequest, gin.H{"message": "code_block exceeds 64KB limit"})
			return
		}

		requestID := uuid.NewString()

		resp, err := svc.ExecuteSync(c.Request.Context(), &proto.ExecuteRequest{
			Id:        requestID,
			Language:  req.Language,
			CodeBlock: req.CodeBlock,
		}, timeout)

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "execution failed: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, ExecuteResponse{
			ID:       requestID,
			Result:   resp.Result,
			Err:      resp.Err,
			Language: req.Language,
		})
	}
}
