package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(r gin.IRoutes, h *Handler) {
	r.GET("/auth/github/login", h.Login)
	r.GET("/auth/github/callback", h.Callback)
	r.GET("/auth/me", h.Me)
	r.POST("/auth/logout", h.Logout)
}

func (h *Handler) Login(c *gin.Context) {
	url, err := h.service.LoginURL(c.Query("return_to"))
	if err != nil {
		if isConfigError(err) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"message": "github auth is not configured"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid return_to"})
		return
	}
	c.Redirect(http.StatusFound, url)
}

func (h *Handler) Callback(c *gin.Context) {
	_, token, returnTo, err := h.service.Callback(c.Request.Context(), c.Query("code"), c.Query("state"))
	if err != nil {
		if isConfigError(err) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"message": "github auth is not configured"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"message": "github oauth callback failed"})
		return
	}
	h.setCookie(c, token, h.service.CookieMaxAge())
	c.Redirect(http.StatusFound, h.service.RedirectURL(returnTo))
}

func (h *Handler) Me(c *gin.Context) {
	token, err := c.Cookie(h.service.CookieName())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}
	user, err := h.service.ParseUser(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

func (h *Handler) Logout(c *gin.Context) {
	h.setCookie(c, "", -1)
	c.Status(http.StatusNoContent)
}

func (h *Handler) setCookie(c *gin.Context, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		h.service.CookieName(),
		value,
		maxAge,
		"/",
		"",
		h.service.CookieSecure(),
		true,
	)
}

func isConfigError(err error) bool {
	return err == ErrMissingGitHubClientID ||
		err == ErrMissingGitHubClientSecret ||
		err == ErrMissingGitHubRedirectURL ||
		err == ErrMissingJWTSecret
}
