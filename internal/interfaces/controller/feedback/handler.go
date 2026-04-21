package feedback

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	domainfeedback "codeRunner-siwu/internal/domain/feedback"
)

type SubmitCmd struct {
	IP      string
	Type    string
	Content string
	Contact string
}

type FeedbackService interface {
	Submit(ctx context.Context, cmd SubmitCmd) error
}

type feedbackRequest struct {
	Type    string `json:"type"    binding:"required"`
	Content string `json:"content" binding:"required"`
	Contact string `json:"contact"`
}

type feedbackResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func HandleFeedback(svc FeedbackService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req feedbackRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, feedbackResponse{false, "请求格式错误"})
			return
		}

		ip := extractIP(c)
		cmd := SubmitCmd{
			IP:      ip,
			Type:    req.Type,
			Content: req.Content,
			Contact: req.Contact,
		}

		if err := svc.Submit(c.Request.Context(), cmd); err != nil {
			switch {
			case errors.Is(err, domainfeedback.ErrRateLimited):
				c.JSON(http.StatusTooManyRequests, feedbackResponse{false, err.Error()})
			case errors.Is(err, domainfeedback.ErrInvalidType),
				errors.Is(err, domainfeedback.ErrInvalidContent),
				errors.Is(err, domainfeedback.ErrInvalidContact):
				c.JSON(http.StatusBadRequest, feedbackResponse{false, err.Error()})
			default:
				c.JSON(http.StatusInternalServerError, feedbackResponse{false, domainfeedback.ErrMailSend.Error()})
			}
			return
		}

		c.JSON(http.StatusOK, feedbackResponse{true, "感谢反馈"})
	}
}

// extractIP 从请求头中提取 IP：优先信任 Nginx 注入的 X-Real-IP。
func extractIP(c *gin.Context) string {
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	if fwd := c.GetHeader("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return c.RemoteIP()
}
