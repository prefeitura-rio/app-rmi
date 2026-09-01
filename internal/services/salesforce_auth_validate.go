package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
)

// AuthValidateResult is returned by the login Salesforce sync orchestrator.
type AuthValidateResult struct {
	Action  string                   `json:"action"` // matched | updated | created
	Cidadao *clients.SalesforceCidadao `json:"cidadao"`
}

// SyncSalesforceOnLogin orchestrates active sync using Keycloak JWT claims:
// GET citizen → match / silent PATCH email / POST create on 404.
func SyncSalesforceOnLogin(ctx context.Context, sf SalesforceCidadaoAPI, claims *models.JWTClaims) (*AuthValidateResult, error) {
	if sf == nil || !sf.Configured() {
		return nil, fmt.Errorf("salesforce client not configured")
	}
	if claims == nil {
		return nil, fmt.Errorf("jwt claims are required")
	}

	cpf := utils.NormalizeCPF(claims.PreferredUsername)
	if !utils.ValidateCPF(cpf) {
		return nil, fmt.Errorf("invalid CPF in preferred_username")
	}

	contaOrigem := SalesforceContaOrigem

	nome := strings.TrimSpace(claims.Name)
	email := strings.TrimSpace(claims.Email)

	cidadao, err := sf.GetCidadao(ctx, cpf)
	if err == nil {
		if emailsMatch(cidadao.Email, email) || email == "" {
			return &AuthValidateResult{Action: "matched", Cidadao: cidadao}, nil
		}
		// Silent update when email diverged. PatchCidadao re-GETs when SF returns
		// only camposAtualizados, so the returned DTO is the post-update Person Account.
		patch := clients.NewSalesforcePatch()
		patch.PutIfNonempty("email", email)
		patch.Put("contaOrigem", contaOrigem)
		updated, patchErr := sf.PatchCidadao(ctx, cpf, patch)
		if patchErr != nil {
			return nil, fmt.Errorf("salesforce silent email update failed: %w", patchErr)
		}
		if updated == nil {
			cidadao.Email = email
			return &AuthValidateResult{Action: "updated", Cidadao: cidadao}, nil
		}
		return &AuthValidateResult{Action: "updated", Cidadao: updated}, nil
	}

	var apiErr *clients.SalesforceAPIError
	if !errors.As(err, &apiErr) || !apiErr.IsNotFound() {
		return nil, fmt.Errorf("salesforce get cidadao failed: %w", err)
	}

	created, createErr := sf.CreateOrUpdateCidadao(ctx, &clients.SalesforceCidadaoCreateRequest{
		CPF:         cpf,
		Nome:        nome,
		Email:       email,
		ContaOrigem: contaOrigem,
		Idioma:      []string{"Portugues_Brasil"},
	})
	if createErr != nil {
		return nil, fmt.Errorf("salesforce create cidadao failed: %w", createErr)
	}

	// Prefer returning the full DTO after create.
	cidadao, getErr := sf.GetCidadao(ctx, cpf)
	if getErr != nil {
		cidadao = &clients.SalesforceCidadao{
			AccountID:              created.AccountID,
			CPF:                    cpf,
			Nome:                   nome,
			Email:                  email,
			CanalOrigem:            contaOrigem,
			CanalUltimaModificacao: contaOrigem,
		}
	}
	return &AuthValidateResult{Action: "created", Cidadao: cidadao}, nil
}

func emailsMatch(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
