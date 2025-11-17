package middleware

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.uber.org/zap"
)

// AuditMiddleware logs all PUT/POST/DELETE requests automatically
// This ensures comprehensive audit trail for all write operations
func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method

		// Only audit write operations
		if method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
			c.Next()
			return
		}

		// Skip health checks and metrics endpoints
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/v1/health") || strings.HasPrefix(path, "/v1/metrics") || strings.HasPrefix(path, "/metrics") {
			c.Next()
			return
		}

		// Extract CPF from various sources
		cpf := extractCPFFromRequest(c)

		// Read request body (we need to preserve it for the handler)
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			// Restore the body for the handler
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Get audit context
		auditCtx := utils.GetAuditContextFromGin(c, cpf)

		// Determine action based on HTTP method
		action := mapHTTPMethodToAction(method)

		// Build metadata
		metadata := map[string]string{
			"endpoint":   path,
			"method":     method,
			"ip_address": c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
		}

		// Add query parameters to metadata
		if len(c.Request.URL.RawQuery) > 0 {
			metadata["query_params"] = c.Request.URL.RawQuery
		}

		// Sanitize and add body to metadata if present
		if len(bodyBytes) > 0 {
			bodyStr := string(bodyBytes)
			if len(bodyStr) > 1000 {
				bodyStr = bodyStr[:1000] + "... (truncated)"
			}
			metadata["request_body"] = bodyStr
		}

		// Extract resource type from path
		resource := extractResourceFromPath(path)

		// Extract resource ID from path or body
		resourceID := extractResourceID(c, path)

		// Process the request
		c.Next()

		// Only log if request was successful (2xx status)
		status := c.Writer.Status()
		if status >= 200 && status < 300 {
			// Add response status to metadata
			metadata["response_status"] = string(rune(status))

			// Log audit event asynchronously
			if err := utils.LogAuditEvent(c.Request.Context(), auditCtx, action, resource, resourceID, nil, nil, metadata); err != nil {
				observability.Logger().Warn("failed to log audit event",
					zap.Error(err),
					zap.String("endpoint", path),
					zap.String("method", method),
				)
			}
		}
	}
}

// extractCPFFromRequest tries to extract CPF from various sources
func extractCPFFromRequest(c *gin.Context) string {
	// Try to get from URL path parameter
	if cpf := c.Param("cpf"); cpf != "" {
		return cpf
	}

	// Try to get from authenticated user claims
	if claims, exists := c.Get("claims"); exists {
		if claimsMap, ok := claims.(map[string]interface{}); ok {
			if cpf, ok := claimsMap["cpf"].(string); ok && cpf != "" {
				return cpf
			}
		}
	}

	return ""
}

// mapHTTPMethodToAction maps HTTP methods to audit actions
func mapHTTPMethodToAction(method string) string {
	switch method {
	case "POST":
		return utils.AuditActionCreate
	case "PUT", "PATCH":
		return utils.AuditActionUpdate
	case "DELETE":
		return utils.AuditActionDelete
	default:
		return utils.AuditActionUpdate
	}
}

// extractResourceFromPath extracts the resource type from the request path
func extractResourceFromPath(path string) string {
	// Remove /v1/ prefix
	path = strings.TrimPrefix(path, "/v1/")

	// Split by / and extract the main resource
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		resource := parts[0]

		// Map common endpoints to resource types
		switch {
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/address"):
			return utils.AuditResourceAddress
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/phone"):
			return utils.AuditResourcePhone
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/email"):
			return utils.AuditResourceEmail
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/ethnicity"):
			return utils.AuditResourceEthnicity
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/exhibition-name"):
			return utils.AuditResourceExhibitionName
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/avatar"):
			return utils.AuditResourceAvatar
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/pets"):
			return utils.AuditResourcePet
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/optin"):
			return utils.AuditResourceUserConfig
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/firstlogin"):
			return utils.AuditResourceUserConfig
		case strings.HasPrefix(path, "citizen/") && strings.Contains(path, "/notification-preferences"):
			return "notification_preferences"
		case strings.HasPrefix(path, "phone/") && strings.Contains(path, "/opt-in"):
			return utils.AuditResourcePhoneMapping
		case strings.HasPrefix(path, "phone/") && strings.Contains(path, "/opt-out"):
			return utils.AuditResourcePhoneMapping
		case strings.HasPrefix(path, "phone/") && strings.Contains(path, "/quarantine"):
			return utils.AuditResourcePhoneQuarantine
		case strings.HasPrefix(path, "phone/") && strings.Contains(path, "/bind"):
			return utils.AuditResourcePhoneMapping
		case strings.HasPrefix(path, "phone/") && strings.Contains(path, "/validate-registration"):
			return utils.AuditResourcePhoneVerification
		case strings.HasPrefix(path, "phone/") && strings.Contains(path, "/reject-registration"):
			return utils.AuditResourcePhoneMapping
		case strings.HasPrefix(path, "phone/") && strings.Contains(path, "/notification-preferences"):
			return "notification_preferences"
		case strings.HasPrefix(path, "memory/"):
			return utils.AuditResourceMemory
		case strings.HasPrefix(path, "avatars"):
			return utils.AuditResourceAvatar
		case strings.HasPrefix(path, "admin/beta/groups"):
			return utils.AuditResourceBetaGroup
		case strings.HasPrefix(path, "admin/beta/whitelist"):
			return utils.AuditResourceBetaWhitelist
		case strings.HasPrefix(path, "admin/notification-categories") || strings.HasPrefix(path, "notification-categories"):
			return utils.AuditResourceNotificationCategory
		default:
			return resource
		}
	}

	return "unknown"
}

// extractResourceID extracts the resource identifier from path or context
func extractResourceID(c *gin.Context, path string) string {
	// Try common ID parameters
	if id := c.Param("id"); id != "" {
		return id
	}
	if id := c.Param("cpf"); id != "" {
		return id
	}
	if id := c.Param("phone_number"); id != "" {
		return id
	}
	if id := c.Param("group_id"); id != "" {
		return id
	}
	if id := c.Param("category_id"); id != "" {
		return id
	}
	if id := c.Param("pet_id"); id != "" {
		return id
	}
	if id := c.Param("memory_name"); id != "" {
		return id
	}

	// Extract from path segments
	parts := strings.Split(strings.TrimPrefix(path, "/v1/"), "/")
	if len(parts) > 1 {
		return parts[1]
	}

	return ""
}
