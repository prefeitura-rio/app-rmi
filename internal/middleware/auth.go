package middleware

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"go.uber.org/zap"
)

// AuthMiddleware extracts and validates JWT claims from the request
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		// Check if it's a Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}

		// Get the token
		token := parts[1]

		// Extract claims from the token
		// Note: The token is already validated by Istio, we just need to extract the claims
		claims, err := extractClaims(token)
		if err != nil {
			observability.Logger().Error("failed to extract claims from token", zap.Error(err))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		// Store claims in context for later use
		c.Set("claims", claims)
		c.Next()
	}
}

// extractClaims extracts the claims from the JWT token
// Note: This is a simplified version since Istio handles validation
func extractClaims(token string) (*models.JWTClaims, error) {
	// Split the token into parts
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode the claims part (second part)
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode claims: %w", err)
	}

	// Parse the claims
	var claims models.JWTClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	return &claims, nil
}

// RequireAdmin checks if the user has admin privileges
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("claims")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Claims not found"})
			c.Abort()
			return
		}

		jwtClaims, ok := claims.(*models.JWTClaims)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims type"})
			c.Abort()
			return
		}

		// Check if user has admin role
		isAdmin := false
		for _, role := range jwtClaims.RealmAccess.Roles {
			if role == config.AppConfig.AdminGroup {
				isAdmin = true
				break
			}
		}

		if !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin privileges required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireOwnCPF checks if the user is accessing their own data
func RequireOwnCPF() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("claims")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Claims not found"})
			c.Abort()
			return
		}

		jwtClaims, ok := claims.(*models.JWTClaims)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims type"})
			c.Abort()
			return
		}

		// Get the CPF from the URL
		requestedCPF := c.Param("cpf")
		userCPF := jwtClaims.PreferredUsername

		// Check if user is admin
		isAdmin := false
		for _, role := range jwtClaims.RealmAccess.Roles {
			if role == config.AppConfig.AdminGroup {
				isAdmin = true
				break
			}
		}

		// Allow if user is admin or accessing their own data
		if !isAdmin && requestedCPF != userCPF {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// ExtractCPFFromToken extracts CPF from JWT token in Gin context
func ExtractCPFFromToken(c *gin.Context) (string, error) {
	claims, exists := c.Get("claims")
	if !exists {
		return "", fmt.Errorf("claims not found")
	}

	jwtClaims, ok := claims.(*models.JWTClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims type")
	}

	return jwtClaims.PreferredUsername, nil
}

// IsAdmin checks if the user has admin privileges
func IsAdmin(c *gin.Context) (bool, error) {
	claims, exists := c.Get("claims")
	if !exists {
		return false, fmt.Errorf("claims not found")
	}

	jwtClaims, ok := claims.(*models.JWTClaims)
	if !ok {
		return false, fmt.Errorf("invalid claims type")
	}

	// Check if user has admin role
	for _, role := range jwtClaims.RealmAccess.Roles {
		if role == config.AppConfig.AdminGroup {
			return true, nil
		}
	}

	return false, nil
}

// ErrAccessDenied is returned when access is denied
var ErrAccessDenied = fmt.Errorf("access denied") 