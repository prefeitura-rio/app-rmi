package services

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"sync"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.uber.org/zap"
)

const (
	// MobilidadeInviteEmailQueue is the Redis sync queue name for conductor invite emails.
	MobilidadeInviteEmailQueue = "mobilidade_invite_email"
)

// ErrEmailDeliverySkipped is returned when outbound email is intentionally not delivered
// (e.g. Data Relay unset). Callers must not treat this as a successful send.
var ErrEmailDeliverySkipped = errors.New("email delivery skipped: outbound provider not configured")

// MobilidadeInviteEmailPayload is the job data enqueued when a conductor is invited.
type MobilidadeInviteEmailPayload struct {
	ConductorID   string `json:"conductor_id"`
	NotifyEmail   string `json:"notify_email"`
	VehicleID     string `json:"vehicle_id"`
	OwnerName     string `json:"owner_name"`
	ConductorName string `json:"conductor_name"`
	DisplayName   string `json:"display_name"`
	BrandLabel    string `json:"brand_label"`
	ModelLabel    string `json:"model_label"`
	ConductorCPF  string `json:"conductor_cpf"`

	// DeliveryOutcome is set after the provider call succeeds (sent) or is skipped, so retries
	// can persist email_status without re-sending. Values: "sent" | "skipped".
	DeliveryOutcome string `json:"delivery_outcome,omitempty"`
}

// EmailMessage is a transactional email to be delivered by an EmailSender.
type EmailMessage struct {
	To         string
	Subject    string
	Body       string
	IsHTMLBody bool
}

// EmailSender delivers transactional emails (Data Relay mailman / logging / tests).
type EmailSender interface {
	Send(ctx context.Context, msg EmailMessage) error
}

// LoggingEmailSender logs the message and reports delivery as skipped (no provider).
type LoggingEmailSender struct {
	logger *logging.SafeLogger
}

// NewLoggingEmailSender creates a LoggingEmailSender.
func NewLoggingEmailSender(logger *logging.SafeLogger) *LoggingEmailSender {
	return &LoggingEmailSender{logger: logger}
}

// Send logs the invite email and returns ErrEmailDeliverySkipped (not a successful delivery).
func (s *LoggingEmailSender) Send(ctx context.Context, msg EmailMessage) error {
	if s.logger != nil {
		s.logger.Info("mobilidade invite email (logging sender; not delivered)",
			zap.String("to", utils.MaskEmail(msg.To)),
			zap.String("subject", msg.Subject),
			zap.Int("body_len", len(msg.Body)))
	}
	return ErrEmailDeliverySkipped
}

// DataRelayEmailSender sends mail via Data Relay POST /data/mailman.
type DataRelayEmailSender struct {
	client *clients.DataRelayClient
	logger *logging.SafeLogger
}

// NewDataRelayEmailSender wraps a configured Data Relay client as an EmailSender.
func NewDataRelayEmailSender(client *clients.DataRelayClient, logger *logging.SafeLogger) *DataRelayEmailSender {
	return &DataRelayEmailSender{client: client, logger: logger}
}

// Send delivers the message through Data Relay mailman.
func (s *DataRelayEmailSender) Send(ctx context.Context, msg EmailMessage) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("data relay email sender not configured")
	}
	to := strings.TrimSpace(msg.To)
	if to == "" {
		return fmt.Errorf("email recipient is required")
	}

	err := s.client.SendEmail(ctx, &clients.EmailRequest{
		ToAddresses: []string{to},
		Subject:     msg.Subject,
		Body:        msg.Body,
		IsHTMLBody:  msg.IsHTMLBody,
	})
	if err != nil {
		return err
	}
	if s.logger != nil {
		s.logger.Info("mobilidade invite email sent via data relay",
			zap.String("to", utils.MaskEmail(to)),
			zap.String("subject", msg.Subject))
	}
	return nil
}

// RecordingEmailSender stores sent messages in memory (for tests).
type RecordingEmailSender struct {
	mu       sync.Mutex
	Messages []EmailMessage
	Err      error
}

// Send records the message or returns Err if set.
func (s *RecordingEmailSender) Send(ctx context.Context, msg EmailMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	s.Messages = append(s.Messages, msg)
	return nil
}

// SentCount returns how many messages were recorded.
func (s *RecordingEmailSender) SentCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Messages)
}

// BuildMobilidadeInviteEmail builds subject/body for a conductor invitation (CadMicro).
func BuildMobilidadeInviteEmail(payload MobilidadeInviteEmailPayload, deepLinkBase string) EmailMessage {
	owner := strings.TrimSpace(payload.OwnerName)
	if owner == "" {
		owner = "Alguém"
	}
	guest := strings.TrimSpace(payload.ConductorName)
	vehicle := strings.TrimSpace(payload.DisplayName)
	if vehicle == "" {
		vehicle = "um veículo"
	}
	brandModel := joinBrandModel(payload.BrandLabel, payload.ModelLabel)

	base := strings.TrimRight(strings.TrimSpace(deepLinkBase), "/")
	if base == "" {
		base = DefaultMobilidadeInviteDeepLinkBase()
	}
	cta := base + "/carteira?mobilidade=true"

	subject := fmt.Sprintf("%s convidou você para ser condutor no CadMicro", owner)

	greeting := "Olá,"
	if guest != "" {
		greeting = fmt.Sprintf("Olá %s,", html.EscapeString(guest))
	}

	vehicleLine := html.EscapeString(vehicle)
	if brandModel != "" {
		vehicleLine = fmt.Sprintf("%s, %s", html.EscapeString(vehicle), html.EscapeString(brandModel))
	}

	body := fmt.Sprintf(
		"%s<br><br>"+
			"Você recebeu um convite de <strong>%s</strong> para ser um condutor autorizado na plataforma CadMicro, "+
			"o cadastro para equipamentos de micromobilidade da Prefeitura do Rio.<br><br>"+
			"Veículo: <strong>%s</strong><br><br>"+
			"Ao aceitar, você poderá utilizar o equipamento em ciclovias, ciclorrotas e ruas permitidas, "+
			"respeitando os limites de velocidade e as restrições em calçadas.<br><br>"+
			"Acesse sua carteira em <a href=\"%s\">Pref Rio</a> para aceitar ou recusar o convite.",
		greeting,
		html.EscapeString(owner),
		vehicleLine,
		html.EscapeString(cta),
	)

	return EmailMessage{
		To:         payload.NotifyEmail,
		Subject:    subject,
		Body:       body,
		IsHTMLBody: true,
	}
}

func joinBrandModel(brand, model string) string {
	brand = strings.TrimSpace(brand)
	model = strings.TrimSpace(model)
	switch {
	case brand != "" && model != "":
		return brand + " " + model
	case brand != "":
		return brand
	default:
		return model
	}
}

// ProcessMobilidadeInviteEmail validates the payload and sends the invite email.
func ProcessMobilidadeInviteEmail(ctx context.Context, sender EmailSender, payload MobilidadeInviteEmailPayload, deepLinkBase string) error {
	if sender == nil {
		return fmt.Errorf("email sender is nil")
	}
	if strings.TrimSpace(payload.NotifyEmail) == "" {
		return fmt.Errorf("notify_email is required")
	}
	if strings.TrimSpace(payload.ConductorID) == "" {
		return fmt.Errorf("conductor_id is required")
	}

	msg := BuildMobilidadeInviteEmail(payload, deepLinkBase)
	if err := sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("send invite email: %w", err)
	}
	return nil
}

// DefaultMobilidadeInviteDeepLinkBase returns the carteira deep-link base.
// Production → https://pref.rio; any other environment → https://staging.app.dados.rio.
func DefaultMobilidadeInviteDeepLinkBase() string {
	if config.AppConfig != nil && strings.EqualFold(config.AppConfig.Environment, "production") {
		return "https://pref.rio"
	}
	return "https://staging.app.dados.rio"
}

// DefaultEmailSender is an optional process-wide override (tests may set a RecordingEmailSender).
var DefaultEmailSender EmailSender

// ResolveDefaultEmailSender picks Data Relay when configured, otherwise logging-only.
// DefaultEmailSender (tests) always wins when set.
func ResolveDefaultEmailSender(logger *logging.SafeLogger) EmailSender {
	if DefaultEmailSender != nil {
		return DefaultEmailSender
	}
	if config.AppConfig != nil {
		base := config.AppConfig.DataRelayBaseURL
		key := config.AppConfig.DataRelayAPIKey
		if base != "" && key != "" {
			timeout := config.AppConfig.DataRelayTimeout
			if timeout <= 0 {
				timeout = 30 * time.Second
			}
			client := clients.NewDataRelayClient(base, key, timeout)
			if logger != nil {
				logger.Info("email sender: data relay mailman enabled",
					zap.String("base_url", base))
			}
			return NewDataRelayEmailSender(client, logger)
		}
	}
	if logger != nil {
		logger.Info("email sender: logging only (DATA_RELAY_BASE_URL / DATA_RELAY_API_KEY not set)")
	}
	return NewLoggingEmailSender(logger)
}
