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
	"github.com/prefeitura-rio/app-rmi/internal/utils"
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

		// Store claims and raw bearer for handlers / DataManager → Salesforce push
		c.Set("claims", claims)
		c.Set("bearer_token", token)
		c.Request = c.Request.WithContext(utils.ContextWithBearerToken(c.Request.Context(), token))
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

	var claimsBytes []byte
	var err error

	// Standard JWT encoding is base64.RawURLEncoding (unpadded URL-safe base64 per RFC 7519).
	claimsBytes, err = base64.RawURLEncoding.DecodeString(claimsPart)
	if err != nil {
		logger.Debug("RawURLEncoding failed, trying URLEncoding with padding", zap.Error(err))
		padded := claimsPart
		switch len(padded) % 4 {
		case 2:
			padded += "=="
		case 3:
			padded += "="
		}
		claimsBytes, err = base64.URLEncoding.DecodeString(padded)
		if err != nil {
			logger.Debug("URLEncoding failed, trying StdEncoding", zap.Error(err))
			claimsBytes, err = base64.StdEncoding.DecodeString(padded)
			if err != nil {
				claimsBytes, err = base64.RawStdEncoding.DecodeString(claimsPart)
				if err != nil {
					logger.Error("failed to decode JWT claims with any encoding", zap.Error(err), zap.String("claims_part", claimsPart[:min(50, len(claimsPart))]))
					return nil, fmt.Errorf("failed to decode claims: %w", err)
				}
			}
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

// HasAdminPrivileges checks if the JWT claims grant administrative access to RMI.
// A caller is considered admin if they carry the configured AdminGroup (e.g. "heimdall-admin"),
// the standard "rmi-admin" role, or if their authorized party (azp) is in TrustedServiceClients.
func HasAdminPrivileges(jwtClaims *models.JWTClaims) bool {
	if jwtClaims == nil {
		return false
	}
	if jwtClaims.HasRole("rmi-admin") {
		return true
	}
	if config.AppConfig != nil {
		if config.AppConfig.AdminGroup != "" && jwtClaims.HasRole(config.AppConfig.AdminGroup) {
			return true
		}
		azp := strings.TrimSpace(jwtClaims.AZP)
		if azp != "" {
			for _, clientID := range config.AppConfig.TrustedServiceClients {
				if azp == strings.TrimSpace(clientID) {
					return true
				}
			}
		}
	}
	return false
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

		if !HasAdminPrivileges(jwtClaims) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin privileges required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireSalesforceWebhookClient allows only JWTs whose azp is in SalesforceWebhookClients.
// The Salesforce integration obtains that JWT via Keycloak (OAuth); RMI rejects other clients.
func RequireSalesforceWebhookClient() gin.HandlerFunc {
	return func(c *gin.Context) {
		if config.AppConfig == nil || len(config.AppConfig.SalesforceWebhookClients) == 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "salesforce webhook clients not configured"})
			c.Abort()
			return
		}

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

		azp := strings.TrimSpace(jwtClaims.AZP)
		for _, allowed := range config.AppConfig.SalesforceWebhookClients {
			if azp != "" && azp == strings.TrimSpace(allowed) {
				c.Next()
				return
			}
		}

		observability.Logger().Warn("salesforce webhook client rejected",
			zap.String("azp", azp),
			zap.String("remote_addr", c.ClientIP()),
			zap.String("path", c.Request.URL.Path))
		c.JSON(http.StatusForbidden, gin.H{"error": "salesforce webhook client not allowed"})
		c.Abort()
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

		isAdmin := HasAdminPrivileges(jwtClaims)

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

	return HasAdminPrivileges(jwtClaims), nil
}

// ErrAccessDenied is returned when access is denied
var ErrAccessDenied = fmt.Errorf("access denied")
