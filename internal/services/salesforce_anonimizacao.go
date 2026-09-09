package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
)

const (
	salesforceAnonimizacaoOwnerKeyPrefix = "salesforce:anonimizacao:"
	salesforceAnonimizacaoOwnerTTL       = 90 * 24 * time.Hour
)

func salesforceAnonimizacaoOwnerKey(numero string) string {
	return salesforceAnonimizacaoOwnerKeyPrefix + strings.TrimSpace(numero)
}

// StoreSalesforceAnonimizacaoOwner binds a Salesforce anonymization request number to the
// calling citizen's CPF. GET polling uses this mapping when upstream omits CPF.
func StoreSalesforceAnonimizacaoOwner(ctx context.Context, rdb *redisclient.Client, numero, cpf string) error {
	numero = strings.TrimSpace(numero)
	cpf = utils.NormalizeCPF(cpf)
	if numero == "" || cpf == "" {
		return fmt.Errorf("numeroSolicitacao and cpf are required")
	}
	if rdb == nil {
		return fmt.Errorf("redis is required to persist anonimizacao ownership")
	}
	return rdb.Set(ctx, salesforceAnonimizacaoOwnerKey(numero), cpf, salesforceAnonimizacaoOwnerTTL).Err()
}

// LookupSalesforceAnonimizacaoOwner returns the CPF that created numeroSolicitacao, if stored.
func LookupSalesforceAnonimizacaoOwner(ctx context.Context, rdb *redisclient.Client, numero string) (string, bool) {
	if rdb == nil {
		return "", false
	}
	numero = strings.TrimSpace(numero)
	if numero == "" {
		return "", false
	}
	val, err := rdb.Get(ctx, salesforceAnonimizacaoOwnerKey(numero)).Result()
	if err != nil {
		return "", false
	}
	cpf := utils.NormalizeCPF(val)
	return cpf, cpf != ""
}
