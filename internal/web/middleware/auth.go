package middleware

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"service-controller-notebookum/internal/web/problem"

	"github.com/gin-gonic/gin"
)

// parseUserIDFromJWT decodes the JWT payload (no signature verification)
// and returns the user_id claim as a string.
func parseUserIDFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		UserID interface{} `json:"user_id"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}
	switch v := claims.UserID.(type) {
	case float64:
		return strconv.Itoa(int(v))
	case string:
		return v
	}
	return ""
}

func ExtractUserID(c *gin.Context) string {
	if userID := c.GetHeader("X-User-ID"); userID != "" {
		return userID
	}

	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		return parseUserIDFromJWT(token)
	}

	return ""
}

func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := ExtractUserID(c)
		if userID == "" {
			problem.Write(c, 401, "Unauthorized", "Missing or invalid authentication", CorrelationID(c))
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Next()
	}
}
