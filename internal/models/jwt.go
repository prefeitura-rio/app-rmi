package models

// JWTClaims represents the structure of the JWT token claims
type JWTClaims struct {
	JTI            string   `json:"jti"`
	Exp            int64    `json:"exp"`
	NBF            int64    `json:"nbf"`
	IAT            int64    `json:"iat"`
	ISS            string   `json:"iss"`
	AUD            []string `json:"aud"`
	SUB            string   `json:"sub"`
	TYP            string   `json:"typ"`
	AZP            string   `json:"azp"`
	Nonce          string   `json:"nonce"`
	AuthTime       int64    `json:"auth_time"`
	SessionState   string   `json:"session_state"`
	ACR            string   `json:"acr"`
	AllowedOrigins []string `json:"allowed-origins"`
	RealmAccess    struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	ResourceAccess struct {
		Broker struct {
			Roles []string `json:"roles"`
		} `json:"broker"`
		Account struct {
			Roles []string `json:"roles"`
		} `json:"account"`
	} `json:"resource_access"`
	Scope             string   `json:"scope"`
	Address           struct{} `json:"address"`
	EmailVerified     bool     `json:"email_verified"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	GivenName         string   `json:"given_name"`
	FamilyName        string   `json:"family_name"`
	Email             string   `json:"email"`
}
