package models

// ClientAccess holds the roles for one client entry under "resource_access".
type ClientAccess struct {
	Roles []string `json:"roles"`
}

// JWTClaims represents the structure of the JWT token claims
type JWTClaims struct {
	JTI            string      `json:"jti"`
	Exp            int64       `json:"exp"`
	NBF            int64       `json:"nbf"`
	IAT            int64       `json:"iat"`
	ISS            string      `json:"iss"`
	AUD            interface{} `json:"aud"` // Can be string or []string
	SUB            string      `json:"sub"`
	TYP            string      `json:"typ"`
	AZP            string      `json:"azp"`
	Nonce          string      `json:"nonce"`
	AuthTime       int64       `json:"auth_time"`
	SessionState   string      `json:"session_state"`
	ACR            string      `json:"acr"`
	AllowedOrigins []string    `json:"allowed-origins"`
	RealmAccess    struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	ResourceAccess    map[string]ClientAccess `json:"resource_access"`
	Scope             string                  `json:"scope"`
	Address           struct{}                `json:"address"`
	EmailVerified     bool                    `json:"email_verified"`
	Name              string                  `json:"name"`
	PreferredUsername string                  `json:"preferred_username"`
	GivenName         string                  `json:"given_name"`
	FamilyName        string                  `json:"family_name"`
	Email             string                  `json:"email"`
}

// GetAudiences returns the audience(s) as a slice of strings
func (c *JWTClaims) GetAudiences() []string {
	switch aud := c.AUD.(type) {
	case string:
		return []string{aud}
	case []string:
		return aud
	case []interface{}:
		// Handle case where JSON unmarshaling creates []interface{}
		result := make([]string, len(aud))
		for i, v := range aud {
			if str, ok := v.(string); ok {
				result[i] = str
			}
		}
		return result
	default:
		return []string{}
	}
}

// HasResourceRole reports whether any client under resource_access grants role.
func (c *JWTClaims) HasResourceRole(role string) bool {
	for _, access := range c.ResourceAccess {
		for _, r := range access.Roles {
			if r == role {
				return true
			}
		}
	}
	return false
}

// HasRole reports whether the token carries role as a realm role or on any
// resource_access client. The client id is environment-specific (e.g.
// "superapp.apps.rio.gov.br"), so all clients are scanned rather than a single
// hardcoded key, which previously dropped roles silently.
func (c *JWTClaims) HasRole(role string) bool {
	for _, r := range c.RealmAccess.Roles {
		if r == role {
			return true
		}
	}
	return c.HasResourceRole(role)
}
