package middleware

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	UserIDKey      = "userID"
	AuthCookieName = "session_token"
)

type UserData struct {
	ID       uuid.UUID `json:"id"`
	Email    string    `json:"email"`
	Nickname string    `json:"nickname"`
}

type AuthResponse struct {
	Success bool     `json:"success"`
	User    UserData `json:"user"` 
}


func AuthMiddleware(authServiceURL string) gin.HandlerFunc {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	return func(c *gin.Context) {
		log.Println("[AUTH-TRACE] Middleware started.")

		sessionToken, err := c.Cookie(AuthCookieName)
		if err != nil {
			log.Println("[AUTH-TRACE] FAILED: Could not get session cookie.")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization cookie not found"})
			return
		}
		if sessionToken == "" {
			log.Println("[AUTH-TRACE] FAILED: Session cookie is empty.")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization token is missing"})
			return
		}

		validationURL := fmt.Sprintf("%s/auth/me", authServiceURL)
		log.Printf("[AUTH-TRACE] Preparing to call auth service at: %s", validationURL)

		req, err := http.NewRequestWithContext(c.Request.Context(), "GET", validationURL, nil)
		if err != nil {
			log.Printf("[AUTH-TRACE] FAILED: Error creating auth request: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		req.AddCookie(&http.Cookie{
			Name:  AuthCookieName,
			Value: sessionToken,
		})

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[AUTH-TRACE] FAILED: Error contacting auth service: %v", err)
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication service is unavailable"})
			return
		}
		defer resp.Body.Close()

		log.Printf("[AUTH-TRACE] Auth service responded with status code: %d", resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[AUTH-TRACE] FAILED: Could not read response body: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to read auth response"})
			return
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("[AUTH-TRACE] FAILED: Auth service returned non-200 status. Body: %s", string(body))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired session"})
			return
		}

		log.Printf("[AUTH-TRACE] Auth service response body: %s", string(body))

		var authResp AuthResponse
		if err := json.Unmarshal(body, &authResp); err != nil {
			log.Printf("[AUTH-TRACE] FAILED: Error decoding auth response JSON: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error on auth response"})
			return
		}

		log.Printf("[AUTH-TRACE] SUCCESS: User authenticated. ID: %s", authResp.User.ID)
		c.Set(UserIDKey, authResp.User.ID)
		
		log.Println("[AUTH-TRACE] Middleware finished, calling next handler.")
		c.Next()
	}
}