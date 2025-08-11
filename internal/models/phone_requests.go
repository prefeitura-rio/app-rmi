package models

// PhoneCitizenResponse represents the response for phone-based citizen lookup
type PhoneCitizenResponse struct {
	Found     bool   `json:"found"`
	CPF       string `json:"cpf,omitempty"`
	Name      string `json:"name,omitempty"`
	FirstName string `json:"first_name,omitempty"`
}

// ValidateRegistrationRequest represents the request for registration validation
type ValidateRegistrationRequest struct {
	Name      string `json:"name" binding:"required"`
	CPF       string `json:"cpf" binding:"required"`
	BirthDate string `json:"birth_date" binding:"required"`
	Channel   string `json:"channel" binding:"required"`
}

// ValidateRegistrationResponse represents the response for registration validation
type ValidateRegistrationResponse struct {
	Valid       bool   `json:"valid"`
	MatchedCPF  string `json:"matched_cpf"`
	MatchedName string `json:"matched_name"`
}

// OptInRequest represents the request for opt-in
type OptInRequest struct {
	CPF              string            `json:"cpf" binding:"required"`
	Channel          string            `json:"channel" binding:"required"`
	ValidationResult *ValidationResult `json:"validation_result,omitempty"`
}

// OptInResponse represents the response for opt-in
type OptInResponse struct {
	Status         string `json:"status"`
	PhoneMappingID string `json:"phone_mapping_id"`
}

// OptOutRequest represents the request for opt-out
type OptOutRequest struct {
	Channel string `json:"channel" binding:"required"`
	Reason  string `json:"reason" binding:"required"`
}

// OptOutResponse represents the response for opt-out
type OptOutResponse struct {
	Status string `json:"status"`
}

// RejectRegistrationRequest represents the request for rejecting a registration
type RejectRegistrationRequest struct {
	CPF     string `json:"cpf" binding:"required"`
	Channel string `json:"channel" binding:"required"`
	Reason  string `json:"reason" binding:"required"`
}

// RejectRegistrationResponse represents the response for rejecting a registration
type RejectRegistrationResponse struct {
	Status string `json:"status"`
}

// Channel represents a communication channel
type Channel struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// OptOutReason represents an opt-out reason
type OptOutReason struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
}

// ChannelsResponse represents the response for available channels
type ChannelsResponse struct {
	Channels []Channel `json:"channels"`
}

// OptOutReasonsResponse represents the response for available opt-out reasons
type OptOutReasonsResponse struct {
	Reasons []OptOutReason `json:"reasons"`
}
