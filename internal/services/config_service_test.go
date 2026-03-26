package services

import (
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/models"
)

func TestNewConfigService(t *testing.T) {
	service := NewConfigService()
	if service == nil {
		t.Error("NewConfigService() returned nil")
	}
}

func TestGetAvailableChannels(t *testing.T) {
	service := NewConfigService()
	response := service.GetAvailableChannels()

	if response == nil {
		t.Fatal("GetAvailableChannels() returned nil")
	}

	if len(response.Channels) != 3 {
		t.Errorf("GetAvailableChannels() returned %d channels, want 3", len(response.Channels))
	}

	// Verify WhatsApp channel
	found := false
	for _, channel := range response.Channels {
		if channel.Code == models.ChannelWhatsApp {
			found = true
			if channel.Name != "WhatsApp Bot" {
				t.Errorf("WhatsApp channel name = %v, want 'WhatsApp Bot'", channel.Name)
			}
			break
		}
	}
	if !found {
		t.Error("WhatsApp channel not found in response")
	}

	// Verify Web channel
	found = false
	for _, channel := range response.Channels {
		if channel.Code == models.ChannelWeb {
			found = true
			if channel.Name != "Web Application" {
				t.Errorf("Web channel name = %v, want 'Web Application'", channel.Name)
			}
			break
		}
	}
	if !found {
		t.Error("Web channel not found in response")
	}

	// Verify Mobile channel
	found = false
	for _, channel := range response.Channels {
		if channel.Code == models.ChannelMobile {
			found = true
			if channel.Name != "Mobile App" {
				t.Errorf("Mobile channel name = %v, want 'Mobile App'", channel.Name)
			}
			break
		}
	}
	if !found {
		t.Error("Mobile channel not found in response")
	}
}

func TestGetAvailableChannels_Consistency(t *testing.T) {
	service := NewConfigService()

	// Call multiple times to ensure consistency
	response1 := service.GetAvailableChannels()
	response2 := service.GetAvailableChannels()

	if len(response1.Channels) != len(response2.Channels) {
		t.Error("GetAvailableChannels() returns inconsistent results")
	}

	for i := range response1.Channels {
		if response1.Channels[i].Code != response2.Channels[i].Code {
			t.Errorf("Channel %d code differs between calls", i)
		}
		if response1.Channels[i].Name != response2.Channels[i].Name {
			t.Errorf("Channel %d name differs between calls", i)
		}
	}
}

func TestGetOptOutReasons(t *testing.T) {
	service := NewConfigService()
	response := service.GetOptOutReasons()

	if response == nil {
		t.Fatal("GetOptOutReasons() returned nil")
	}

	if len(response.Reasons) != 4 {
		t.Errorf("GetOptOutReasons() returned %d reasons, want 4", len(response.Reasons))
	}

	// Verify IrrelevantContent reason
	found := false
	for _, reason := range response.Reasons {
		if reason.Code == models.OptOutReasonIrrelevantContent {
			found = true
			if reason.Title != "Conteúdo irrelevante" {
				t.Errorf("IrrelevantContent title = %v, want 'Conteúdo irrelevante'", reason.Title)
			}
			if reason.Subtitle != "As mensagens não são úteis para mim." {
				t.Errorf("IrrelevantContent subtitle = %v, want 'As mensagens não são úteis para mim.'", reason.Subtitle)
			}
			break
		}
	}
	if !found {
		t.Error("IrrelevantContent reason not found in response")
	}

	// Verify NotFromRio reason
	found = false
	for _, reason := range response.Reasons {
		if reason.Code == models.OptOutReasonNotFromRio {
			found = true
			if reason.Title != "Não sou do Rio" {
				t.Errorf("NotFromRio title = %v, want 'Não sou do Rio'", reason.Title)
			}
			if reason.Subtitle != "Não moro na cidade do Rio de Janeiro" {
				t.Errorf("NotFromRio subtitle = %v, want 'Não moro na cidade do Rio de Janeiro'", reason.Subtitle)
			}
			break
		}
	}
	if !found {
		t.Error("NotFromRio reason not found in response")
	}

	// Verify IncorrectPerson reason
	found = false
	for _, reason := range response.Reasons {
		if reason.Code == models.OptOutReasonIncorrectPerson {
			found = true
			if reason.Title != "Mensagem era engano" {
				t.Errorf("IncorrectPerson title = %v, want 'Mensagem era engano'", reason.Title)
			}
			if reason.Subtitle != "Não sou a pessoa da mensagem" {
				t.Errorf("IncorrectPerson subtitle = %v, want 'Não sou a pessoa da mensagem'", reason.Subtitle)
			}
			break
		}
	}
	if !found {
		t.Error("IncorrectPerson reason not found in response")
	}

	// Verify TooManyMessages reason
	found = false
	for _, reason := range response.Reasons {
		if reason.Code == models.OptOutReasonTooManyMessages {
			found = true
			if reason.Title != "Quantidade de mensagens" {
				t.Errorf("TooManyMessages title = %v, want 'Quantidade de mensagens'", reason.Title)
			}
			if reason.Subtitle != "A Prefeitura está me enviando muitas mensagens" {
				t.Errorf("TooManyMessages subtitle = %v, want 'A Prefeitura está me enviando muitas mensagens'", reason.Subtitle)
			}
			break
		}
	}
	if !found {
		t.Error("TooManyMessages reason not found in response")
	}
}

func TestGetOptOutReasons_Consistency(t *testing.T) {
	service := NewConfigService()

	// Call multiple times to ensure consistency
	response1 := service.GetOptOutReasons()
	response2 := service.GetOptOutReasons()

	if len(response1.Reasons) != len(response2.Reasons) {
		t.Error("GetOptOutReasons() returns inconsistent results")
	}

	for i := range response1.Reasons {
		if response1.Reasons[i].Code != response2.Reasons[i].Code {
			t.Errorf("Reason %d code differs between calls", i)
		}
		if response1.Reasons[i].Title != response2.Reasons[i].Title {
			t.Errorf("Reason %d title differs between calls", i)
		}
		if response1.Reasons[i].Subtitle != response2.Reasons[i].Subtitle {
			t.Errorf("Reason %d subtitle differs between calls", i)
		}
	}
}

func TestGetOptOutReasons_AllHaveRequiredFields(t *testing.T) {
	service := NewConfigService()
	response := service.GetOptOutReasons()

	for i, reason := range response.Reasons {
		if reason.Code == "" {
			t.Errorf("Reason %d has empty code", i)
		}
		if reason.Title == "" {
			t.Errorf("Reason %d has empty title", i)
		}
		if reason.Subtitle == "" {
			t.Errorf("Reason %d has empty subtitle", i)
		}
	}
}

func TestGetAvailableChannels_AllHaveRequiredFields(t *testing.T) {
	service := NewConfigService()
	response := service.GetAvailableChannels()

	for i, channel := range response.Channels {
		if channel.Code == "" {
			t.Errorf("Channel %d has empty code", i)
		}
		if channel.Name == "" {
			t.Errorf("Channel %d has empty name", i)
		}
	}
}
