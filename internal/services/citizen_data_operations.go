package services

import (
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/models"
)

// CitizenDataOperation implements DataOperation for citizen data
type CitizenDataOperation struct {
	CPF  string
	Data *models.Citizen
}

// GetKey returns the CPF as the key
func (op *CitizenDataOperation) GetKey() string {
	return op.CPF
}

// GetCollection returns the citizen collection name
func (op *CitizenDataOperation) GetCollection() string {
	return "citizens"
}

// GetData returns the citizen data
func (op *CitizenDataOperation) GetData() interface{} {
	return op.Data
}

// GetTTL returns the TTL for citizen data (24 hours)
func (op *CitizenDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *CitizenDataOperation) GetType() string {
	return "citizen"
}

// PhoneMappingDataOperation implements DataOperation for phone mapping data
type PhoneMappingDataOperation struct {
	PhoneNumber string
	Data        *models.PhoneCPFMapping
}

// GetKey returns the phone number as the key
func (op *PhoneMappingDataOperation) GetKey() string {
	return op.PhoneNumber
}

// GetCollection returns the phone mapping collection name
func (op *PhoneMappingDataOperation) GetCollection() string {
	return "phone_mappings"
}

// GetData returns the phone mapping data
func (op *PhoneMappingDataOperation) GetData() interface{} {
	return op.Data
}

// GetTTL returns the TTL for phone mapping data (24 hours)
func (op *PhoneMappingDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *PhoneMappingDataOperation) GetType() string {
	return "phone_mapping"
}

// UserConfigDataOperation implements DataOperation for user config data
type UserConfigDataOperation struct {
	UserID string
	Data   *models.UserConfig
}

// GetKey returns the user ID as the key
func (op *UserConfigDataOperation) GetKey() string {
	return op.UserID
}

// GetCollection returns the user config collection name
func (op *UserConfigDataOperation) GetCollection() string {
	return "user_configs"
}

// GetData returns the user config data
func (op *UserConfigDataOperation) GetData() interface{} {
	return op.Data
}

// GetTTL returns the TTL for user config data (24 hours)
func (op *UserConfigDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *UserConfigDataOperation) GetType() string {
	return "user_config"
}

// OptInHistoryDataOperation implements DataOperation for opt-in history data
type OptInHistoryDataOperation struct {
	ID   string
	Data *models.OptInHistory
}

// GetKey returns the ID as the key
func (op *OptInHistoryDataOperation) GetKey() string {
	return op.ID
}

// GetCollection returns the opt-in history collection name
func (op *OptInHistoryDataOperation) GetCollection() string {
	return "opt_in_histories"
}

// GetData returns the opt-in history data
func (op *OptInHistoryDataOperation) GetData() interface{} {
	return op.Data
}

// GetTTL returns the TTL for opt-in history data (24 hours)
func (op *OptInHistoryDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *OptInHistoryDataOperation) GetType() string {
	return "opt_in_history"
}

// BetaGroupDataOperation implements DataOperation for beta group data
type BetaGroupDataOperation struct {
	ID   string
	Data *models.BetaGroup
}

// GetKey returns the ID as the key
func (op *BetaGroupDataOperation) GetKey() string {
	return op.ID
}

// GetCollection returns the beta group collection name
func (op *BetaGroupDataOperation) GetCollection() string {
	return "beta_groups"
}

// GetData returns the beta group data
func (op *BetaGroupDataOperation) GetData() interface{} {
	return op.Data
}

// GetTTL returns the TTL for beta group data (24 hours)
func (op *BetaGroupDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *BetaGroupDataOperation) GetType() string {
	return "beta_group"
}

// PhoneVerificationDataOperation implements DataOperation for phone verification data
type PhoneVerificationDataOperation struct {
	PhoneNumber string
	Data        *models.PhoneVerification
}

// GetKey returns the phone number as the key
func (op *PhoneVerificationDataOperation) GetKey() string {
	return op.PhoneNumber
}

// GetCollection returns the phone verification collection name
func (op *PhoneVerificationDataOperation) GetCollection() string {
	return "phone_verifications"
}

// GetData returns the phone verification data
func (op *PhoneVerificationDataOperation) GetData() interface{} {
	return op.Data
}

// GetTTL returns the TTL for phone verification data (24 hours)
func (op *PhoneVerificationDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *PhoneVerificationDataOperation) GetType() string {
	return "phone_verification"
}

// MaintenanceRequestDataOperation implements DataOperation for maintenance request data
type MaintenanceRequestDataOperation struct {
	ID   string
	Data *models.MaintenanceRequest
}

// GetKey returns the ID as the key
func (op *MaintenanceRequestDataOperation) GetKey() string {
	return op.ID
}

// GetCollection returns the maintenance request collection name
func (op *MaintenanceRequestDataOperation) GetCollection() string {
	return "maintenance_requests"
}

// GetData returns the maintenance request data
func (op *MaintenanceRequestDataOperation) GetData() interface{} {
	return op.Data
}

// GetTTL returns the TTL for maintenance request data (24 hours)
func (op *MaintenanceRequestDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *MaintenanceRequestDataOperation) GetType() string {
	return "maintenance_request"
}

// SelfDeclaredAddressDataOperation implements DataOperation for self-declared address data
type SelfDeclaredAddressDataOperation struct {
	CPF       string
	Endereco  *models.Endereco
	UpdatedAt time.Time
}

// GetKey returns the CPF as the key
func (op *SelfDeclaredAddressDataOperation) GetKey() string {
	return op.CPF
}

// GetCollection returns the self-declared collection name
func (op *SelfDeclaredAddressDataOperation) GetCollection() string {
	return "self_declared"
}

// GetData returns the self-declared address data
func (op *SelfDeclaredAddressDataOperation) GetData() interface{} {
	return map[string]interface{}{
		"cpf":        op.CPF,
		"endereco":   op.Endereco,
		"updated_at": op.UpdatedAt,
	}
}

// GetTTL returns the TTL for self-declared address data (24 hours)
func (op *SelfDeclaredAddressDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *SelfDeclaredAddressDataOperation) GetType() string {
	return "self_declared_address"
}

// SelfDeclaredEmailDataOperation implements DataOperation for self-declared email data
type SelfDeclaredEmailDataOperation struct {
	CPF       string
	Email     *models.Email
	UpdatedAt time.Time
}

// GetKey returns the CPF as the key
func (op *SelfDeclaredEmailDataOperation) GetKey() string {
	return op.CPF
}

// GetCollection returns the self-declared collection name
func (op *SelfDeclaredEmailDataOperation) GetCollection() string {
	return "self_declared"
}

// GetData returns the self-declared email data
func (op *SelfDeclaredEmailDataOperation) GetData() interface{} {
	return map[string]interface{}{
		"cpf":        op.CPF,
		"email":      op.Email,
		"updated_at": op.UpdatedAt,
	}
}

// GetTTL returns the TTL for self-declared email data (24 hours)
func (op *SelfDeclaredEmailDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *SelfDeclaredEmailDataOperation) GetType() string {
	return "self_declared_email"
}

// SelfDeclaredPhoneDataOperation implements DataOperation for self-declared phone data
type SelfDeclaredPhoneDataOperation struct {
	CPF       string
	Telefone  *models.Telefone
	UpdatedAt time.Time
}

// GetKey returns the CPF as the key
func (op *SelfDeclaredPhoneDataOperation) GetKey() string {
	return op.CPF
}

// GetCollection returns the self-declared collection name
func (op *SelfDeclaredPhoneDataOperation) GetCollection() string {
	return "self_declared"
}

// GetData returns the self-declared phone data
func (op *SelfDeclaredPhoneDataOperation) GetData() interface{} {
	return map[string]interface{}{
		"cpf":        op.CPF,
		"telefone":   op.Telefone,
		"updated_at": op.UpdatedAt,
	}
}

// GetTTL returns the TTL for self-declared phone data (24 hours)
func (op *SelfDeclaredPhoneDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *SelfDeclaredPhoneDataOperation) GetType() string {
	return "self_declared_phone"
}

// SelfDeclaredRacaDataOperation implements DataOperation for self-declared ethnicity data
type SelfDeclaredRacaDataOperation struct {
	CPF       string
	Raca      string
	UpdatedAt time.Time
}

// GetKey returns the CPF as the key
func (op *SelfDeclaredRacaDataOperation) GetKey() string {
	return op.CPF
}

// GetCollection returns the self-declared collection name
func (op *SelfDeclaredRacaDataOperation) GetCollection() string {
	return "self_declared"
}

// GetData returns the self-declared ethnicity data
func (op *SelfDeclaredRacaDataOperation) GetData() interface{} {
	return map[string]interface{}{
		"cpf":        op.CPF,
		"raca":       op.Raca,
		"updated_at": op.UpdatedAt,
	}
}

// GetTTL returns the TTL for self-declared ethnicity data (24 hours)
func (op *SelfDeclaredRacaDataOperation) GetTTL() time.Duration {
	return 24 * time.Hour
}

// GetType returns the operation type
func (op *SelfDeclaredRacaDataOperation) GetType() string {
	return "self_declared_raca"
}
