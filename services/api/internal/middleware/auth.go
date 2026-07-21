package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"plantbrain-api/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthMiddleware extracts session token, verifies it, and sets auth context variables.
func AuthMiddleware(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string

		// 1. Try Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				token = parts[1]
			}
		}

		// 2. Try Cookie. BetterAuth names it "better-auth.session_token" (underscore);
		// the hyphenated form is kept as a fallback for older/custom clients.
		if token == "" {
			for _, name := range []string{"better-auth.session_token", "better-auth.session-token"} {
				if cookie, err := c.Cookie(name); err == nil && cookie != "" {
					token = cookie
					break
				}
			}
		}

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: missing session token"})
			c.Abort()
			return
		}

		ctx := c.Request.Context()
		var userIDStr string
		var orgIDStr string
		var plantIDStr *string
		var roleStr *string

		// Try JWT first
		if len(strings.Split(token, ".")) == 3 {
			jwtSecret := os.Getenv("JWT_SECRET")
			if jwtSecret != "" {
				claims, err := parseJWTHS256(token, jwtSecret)
				if err == nil {
					if id, ok := claims["sub"].(string); ok {
						userIDStr = id
					} else if id, ok := claims["userId"].(string); ok {
						userIDStr = id
					}

					if org, ok := claims["orgId"].(string); ok {
						orgIDStr = org
					}
					if plant, ok := claims["plantId"].(string); ok {
						plantIDStr = &plant
					}
					if role, ok := claims["role"].(string); ok {
						roleStr = &role
					}
				} else {
					log.Printf("JWT verification failed: %v", err)
				}
			}
		}

		// Fallback to session token in identity.sessions
		if userIDStr == "" {
			// BetterAuth signs the session cookie as "<token>.<hmac-signature>";
			// the stored session row holds only the token, so use the part before
			// the first dot. Plain bearer tokens (no dot) pass through unchanged.
			sessionToken := token
			if idx := strings.IndexByte(sessionToken, '.'); idx >= 0 {
				sessionToken = sessionToken[:idx]
			}

			query := `
				SELECT
					s.user_id::text,
					u.organization_id::text,
					m.plant_id::text,
					r.name as role
				FROM identity.sessions s
				JOIN identity.users u ON s.user_id = u.id
				LEFT JOIN identity.memberships m ON u.id = m.user_id
				LEFT JOIN identity.roles r ON m.role_id = r.id
				WHERE s.token = $1 AND s.expires_at > NOW()
				LIMIT 1
			`

			err := dbPool.QueryRow(ctx, query, sessionToken).Scan(&userIDStr, &orgIDStr, &plantIDStr, &roleStr)
			if err != nil {
				log.Printf("Auth check failed: %v", err)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: invalid or expired session"})
				c.Abort()
				return
			}
		}

		// Set variables in context
		c.Set("userID", userIDStr)
		c.Set("orgID", orgIDStr)
		if plantIDStr != nil {
			c.Set("plantID", *plantIDStr)
		} else {
			c.Set("plantID", "")
		}
		if roleStr != nil {
			c.Set("role", *roleStr)
		} else {
			c.Set("role", "")
		}

		c.Next()
	}
}

func parseJWTHS256(tokenString, secret string) (map[string]interface{}, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	headerPayload := parts[0] + "." + parts[1]

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		signature, err = base64.URLEncoding.DecodeString(parts[2])
		if err != nil {
			return nil, err
		}
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(headerPayload))
	expectedSignature := mac.Sum(nil)

	if !hmac.Equal(signature, expectedSignature) {
		return nil, fmt.Errorf("invalid signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, err
		}
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}

	return claims, nil
}

// RequireRoles enforces coarse-grained RBAC after AuthMiddleware has populated the request context.
func RequireRoles(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if !auth.RoleAllowed(role, allowedRoles...) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":        "forbidden: role is not allowed for this action",
				"requiredRole": allowedRoles,
				"currentRole":  role,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
