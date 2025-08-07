package services

import "github.com/prefeitura-rio/app-rmi/internal/models"

// ConfigService provides configuration data for the application
type ConfigService struct{}

// NewConfigService creates a new ConfigService
func NewConfigService() *ConfigService {
	return &ConfigService{}
}

// GetAvailableChannels returns the list of available communication channels
func (s *ConfigService) GetAvailableChannels() *models.ChannelsResponse {
	return &models.ChannelsResponse{
		Channels: []models.Channel{
			{
				Code: models.ChannelWhatsApp,
				Name: "WhatsApp Bot",
			},
			{
				Code: models.ChannelWeb,
				Name: "Web Application",
			},
			{
				Code: models.ChannelMobile,
				Name: "Mobile App",
			},
		},
	}
}

// GetOptOutReasons returns the list of available opt-out reasons
func (s *ConfigService) GetOptOutReasons() *models.OptOutReasonsResponse {
	return &models.OptOutReasonsResponse{
		Reasons: []models.OptOutReason{
			{
				Code:     models.OptOutReasonIrrelevantContent,
				Title:    "Conteúdo irrelevante",
				Subtitle: "As mensagens não são úteis para mim.",
			},
			{
				Code:     models.OptOutReasonNotFromRio,
				Title:    "Não sou do Rio",
				Subtitle: "Não moro na cidade do Rio de Janeiro",
			},
			{
				Code:     models.OptOutReasonIncorrectPerson,
				Title:    "Mensagem era engano",
				Subtitle: "Não sou a pessoa da mensagem",
			},
			{
				Code:     models.OptOutReasonTooManyMessages,
				Title:    "Quantidade de mensagens",
				Subtitle: "A Prefeitura está me enviando muitas mensagens",
			},
		},
	}
} 