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
		observability.Logger().Debug("attempting to extract claims from JWT token", zap.String("token_prefix", token[:min(20, len(token))]+"..."))
		claims, err := extractClaims(token)
		if err != nil {
			observability.Logger().Error("failed to extract claims from token", zap.Error(err), zap.String("token_length", fmt.Sprintf("%d", len(token))))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		observability.Logger().Debug("successfully extracted claims from JWT token", zap.String("user_sub", claims.SUB))

		// Store claims in context for later use
		c.Set("claims", claims)
		c.Next()
	}
}

// extractClaims extracts the claims from the JWT token
// Note: This is a simplified version since Istio handles validation
func extractClaims(token string) (*models.JWTClaims, error) {
	logger := observability.Logger()

	// Split the token into parts
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		logger.Debug("invalid JWT token format", zap.Int("parts_count", len(parts)))
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode the claims part (second part) with proper padding handling
	claimsPart := parts[1]
	logger.Debug("extracting JWT claims", zap.String("claims_part_length", fmt.Sprintf("%d", len(claimsPart))))

	// Add padding if needed
	switch len(claimsPart) % 4 {
	case 2:
		claimsPart += "=="
	case 3:
		claimsPart += "="
	}

	// Try RawURLEncoding first, then fallback to standard encoding
	var claimsBytes []byte
	var err error

	claimsBytes, err = base64.RawURLEncoding.DecodeString(claimsPart)
	if err != nil {
		logger.Debug("RawURLEncoding failed, trying StdEncoding", zap.Error(err))
		// Fallback to standard base64 decoding
		claimsBytes, err = base64.StdEncoding.DecodeString(claimsPart)
		if err != nil {
			logger.Error("failed to decode JWT claims with both encodings", zap.Error(err), zap.String("claims_part", claimsPart[:min(50, len(claimsPart))]))
			return nil, fmt.Errorf("failed to decode claims: %w", err)
		}
	}

	logger.Debug("successfully decoded JWT claims", zap.Int("claims_bytes_length", len(claimsBytes)))

	// Parse the claims
	var claims models.JWTClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		logger.Error("failed to parse JWT claims JSON", zap.Error(err), zap.String("claims_json", string(claimsBytes[:min(200, len(claimsBytes))])))
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	logger.Debug("successfully parsed JWT claims", zap.String("sub", claims.SUB), zap.String("iss", claims.ISS))
	return &claims, nil
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

		// Check if user has admin role in RealmAccess or ResourceAccess.Superapp
		if !isAdminClaims(jwtClaims) {
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

		// Allow if user is admin or accessing their own data
		if !isAdminClaims(jwtClaims) && requestedCPF != userCPF {
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

// isAdminClaims reports whether the JWT claims carry the configured admin group
// in either RealmAccess.Roles or ResourceAccess.Superapp.Roles.
func isAdminClaims(jwtClaims *models.JWTClaims) bool {
	for _, role := range jwtClaims.RealmAccess.Roles {
		if role == config.AppConfig.AdminGroup {
			return true
		}
	}
	for _, role := range jwtClaims.ResourceAccess.Superapp.Roles {
		if role == config.AppConfig.AdminGroup {
			return true
		}
	}
	return false
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

	return isAdminClaims(jwtClaims), nil
}

// RequireServiceAccount ensures the caller is either:
//   - a Keycloak service account from one of the allowed client IDs, OR
//   - a human admin in the configured admin group (admins can do anything)
//
// Usage: RequireServiceAccount("superapp", "app-eai-agent")
func RequireServiceAccount(clientIDs ...string) gin.HandlerFunc {
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

		// Admins bypass everything (checked in both RealmAccess and ResourceAccess.Superapp)
		if isAdminClaims(jwtClaims) {
			c.Next()
			return
		}

		// Must be a service account from one of the expected clients
		isServiceAccount := strings.HasPrefix(jwtClaims.PreferredUsername, "service-account-")
		isAllowedClient := false
		for _, id := range clientIDs {
			if jwtClaims.AZP == id {
				isAllowedClient = true
				break
			}
		}

		if !isServiceAccount || !isAllowedClient {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireOwnCPFOrServiceAccount passes if the caller is:
//   - a human admin in the configured admin group, OR
//   - a Keycloak service account from one of the allowed client IDs, OR
//   - a regular user whose preferred_username matches the :cpf URL param
//
// Usage: RequireOwnCPFOrServiceAccount("superapp", "app-eai-agent")
func RequireOwnCPFOrServiceAccount(clientIDs ...string) gin.HandlerFunc {
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

		// Admins bypass everything (checked in both RealmAccess and ResourceAccess.Superapp)
		if isAdminClaims(jwtClaims) {
			c.Next()
			return
		}

		// Service accounts from allowed clients bypass CPF check
		if strings.HasPrefix(jwtClaims.PreferredUsername, "service-account-") {
			for _, id := range clientIDs {
				if jwtClaims.AZP == id {
					c.Next()
					return
				}
			}
			// SA from a disallowed client — reject immediately
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}

		// Regular users must be accessing their own CPF
		requestedCPF := c.Param("cpf")
		if jwtClaims.PreferredUsername == requestedCPF {
			c.Next()
			return
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		c.Abort()
	}
}

// ErrAccessDenied is returned when access is denied
var ErrAccessDenied = fmt.Errorf("access denied")
