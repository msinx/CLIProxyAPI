package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usagekeeper/upstream/auth"
)

const sessionCookieName = "cpa_usage_keeper_session"

const maxFailedLoginAttempts = 5

const failedLoginWindow = 15 * time.Minute

type AuthConfig struct {
	Enabled       bool
	LoginPassword string
	SessionTTL    time.Duration
	BasePath      string
	Verifier      func(clientIP string, localClient bool, provided string) (bool, int, string)
}

type authHandler struct {
	config   AuthConfig
	sessions *auth.SessionManager
	now      func() time.Time

	mu             sync.Mutex
	failedAttempts map[string]failedLoginAttempt
}

type failedLoginAttempt struct {
	count     int
	updatedAt time.Time
}

type loginRequest struct {
	Password string `json:"password"`
}

type sessionResponse struct {
	Authenticated bool `json:"authenticated"`
}

func NewAuthHandler(config AuthConfig, sessions *auth.SessionManager) *authHandler {
	return &authHandler{config: config, sessions: sessions, now: time.Now, failedAttempts: make(map[string]failedLoginAttempt)}
}

func (h *authHandler) registerRoutes(router gin.IRoutes) {
	router.GET("/session", h.getSession)
	router.POST("/login", h.login)
	router.POST("/logout", h.logout)
}

func (h *authHandler) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h == nil || !h.config.Enabled {
			c.Next()
			return
		}
		if h.sessions == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		token, err := c.Cookie(sessionCookieName)
		if err != nil || !h.sessions.Validate(token) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		c.Next()
	}
}

func (h *authHandler) getSession(c *gin.Context) {
	if h == nil || !h.config.Enabled {
		c.JSON(http.StatusOK, sessionResponse{Authenticated: true})
		return
	}
	if h.sessions == nil {
		c.JSON(http.StatusOK, sessionResponse{Authenticated: false})
		return
	}

	token, err := c.Cookie(sessionCookieName)
	if err != nil {
		c.JSON(http.StatusOK, sessionResponse{Authenticated: false})
		return
	}

	c.JSON(http.StatusOK, sessionResponse{Authenticated: h.sessions.Validate(token)})
}

func (h *authHandler) login(c *gin.Context) {
	if h == nil || !h.config.Enabled {
		c.Status(http.StatusNoContent)
		return
	}
	if h.sessions == nil {
		writeInternalError(c, "session manager is not configured", nil)
		return
	}

	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	clientKey := loginClientKey(c)
	verification := h.verifyPassword(c, request.Password)
	if h.tooManyFailedAttempts(clientKey) && !verification.matched {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many failed login attempts"})
		return
	}

	if !verification.matched {
		h.recordFailedAttempt(clientKey)
		c.JSON(verification.statusCode, gin.H{"error": verification.message})
		return
	}
	h.clearFailedAttempts(clientKey)

	token, expiresAt, err := h.sessions.Create()
	if err != nil {
		writeInternalError(c, "create auth session failed", err)
		return
	}

	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	cookiePath := h.config.BasePath
	if cookiePath == "" {
		cookiePath = "/"
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     cookiePath,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})
	c.Status(http.StatusNoContent)
}

type passwordVerification struct {
	matched    bool
	statusCode int
	message    string
}

func (h *authHandler) verifyPassword(c *gin.Context, provided string) passwordVerification {
	if h == nil {
		return passwordVerification{statusCode: http.StatusUnauthorized, message: "invalid password"}
	}
	if h.config.Verifier == nil {
		if subtle.ConstantTimeCompare([]byte(provided), []byte(h.config.LoginPassword)) == 1 {
			return passwordVerification{matched: true}
		}
		return passwordVerification{statusCode: http.StatusUnauthorized, message: "invalid password"}
	}
	clientIP := loginClientKey(c)
	allowed, statusCode, message := h.config.Verifier(clientIP, isLoopbackHost(clientIP), provided)
	if allowed {
		return passwordVerification{matched: true}
	}
	if statusCode == 0 {
		statusCode = http.StatusUnauthorized
	}
	if message == "" {
		message = "invalid management key"
	}
	return passwordVerification{statusCode: statusCode, message: message}
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *authHandler) logout(c *gin.Context) {
	if h == nil || !h.config.Enabled {
		c.Status(http.StatusNoContent)
		return
	}
	if h.sessions != nil {
		if token, err := c.Cookie(sessionCookieName); err == nil {
			h.sessions.Delete(token)
		}
	}
	clearSessionCookie(c, h.config.BasePath)
	c.Status(http.StatusNoContent)
}

func (h *authHandler) tooManyFailedAttempts(key string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupFailedAttemptLocked(key)
	return h.failedAttempts[key].count >= maxFailedLoginAttempts
}

func (h *authHandler) recordFailedAttempt(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupFailedAttemptLocked(key)
	attempt := h.failedAttempts[key]
	attempt.count++
	attempt.updatedAt = h.now()
	h.failedAttempts[key] = attempt
}

func (h *authHandler) clearFailedAttempts(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.failedAttempts, key)
}

func (h *authHandler) cleanupFailedAttemptLocked(key string) {
	attempt, ok := h.failedAttempts[key]
	if !ok {
		return
	}
	if h.now().Sub(attempt.updatedAt) > failedLoginWindow {
		delete(h.failedAttempts, key)
	}
}

func loginClientKey(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return c.ClientIP()
}

func clearSessionCookie(c *gin.Context, basePath string) {
	cookiePath := basePath
	if cookiePath == "" {
		cookiePath = "/"
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     cookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}
