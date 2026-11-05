package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildAddressString tests the buildAddressString helper function
func TestBuildAddressString(t *testing.T) {
	tests := []struct {
		name     string
		endereco *models.EnderecoPrincipal
		expected string
	}{
		{
			name: "complete address with all fields",
			endereco: &models.EnderecoPrincipal{
				Logradouro:  strPtr("Rua das Flores"),
				Numero:      strPtr("123"),
				Complemento: strPtr("Apto 401"),
				Bairro:      strPtr("Centro"),
				Municipio:   strPtr("Rio de Janeiro"),
				Estado:      strPtr("RJ"),
			},
			expected: "Rua das Flores, 123, Apto 401, Centro, Rio de Janeiro, RJ",
		},
		{
			name: "address without complement",
			endereco: &models.EnderecoPrincipal{
				Logradouro: strPtr("Avenida Atlântica"),
				Numero:     strPtr("500"),
				Bairro:     strPtr("Copacabana"),
				Municipio:  strPtr("Rio de Janeiro"),
				Estado:     strPtr("RJ"),
			},
			expected: "Avenida Atlântica, 500, Copacabana, Rio de Janeiro, RJ",
		},
		{
			name: "address without number",
			endereco: &models.EnderecoPrincipal{
				Logradouro: strPtr("Rua Sem Número"),
				Bairro:     strPtr("Ipanema"),
				Municipio:  strPtr("Rio de Janeiro"),
				Estado:     strPtr("RJ"),
			},
			expected: "Rua Sem Número, Ipanema, Rio de Janeiro, RJ",
		},
		{
			name: "address with empty number",
			endereco: &models.EnderecoPrincipal{
				Logradouro: strPtr("Rua Vazia"),
				Numero:     strPtr(""),
				Bairro:     strPtr("Botafogo"),
				Municipio:  strPtr("Rio de Janeiro"),
				Estado:     strPtr("RJ"),
			},
			expected: "Rua Vazia, Botafogo, Rio de Janeiro, RJ",
		},
		{
			name: "address with default city (no municipio)",
			endereco: &models.EnderecoPrincipal{
				Logradouro: strPtr("Rua Teste"),
				Numero:     strPtr("42"),
				Bairro:     strPtr("Teste"),
				Estado:     strPtr("RJ"),
			},
			expected: "Rua Teste, 42, Teste, Rio de Janeiro, RJ",
		},
		{
			name: "address with default state (no estado)",
			endereco: &models.EnderecoPrincipal{
				Logradouro: strPtr("Rua Exemplo"),
				Numero:     strPtr("99"),
				Bairro:     strPtr("Exemplo"),
				Municipio:  strPtr("Rio de Janeiro"),
			},
			expected: "Rua Exemplo, 99, Exemplo, Rio de Janeiro, RJ",
		},
		{
			name: "address with default city and state",
			endereco: &models.EnderecoPrincipal{
				Logradouro: strPtr("Rua Mínima"),
				Bairro:     strPtr("Mínimo"),
			},
			expected: "Rua Mínima, Mínimo, Rio de Janeiro, RJ",
		},
		{
			name: "minimal address (just logradouro)",
			endereco: &models.EnderecoPrincipal{
				Logradouro: strPtr("Rua Só"),
			},
			expected: "Rua Só, Rio de Janeiro, RJ",
		},
		{
			name:     "nil address",
			endereco: &models.EnderecoPrincipal{},
			expected: "",
		},
		{
			name: "address with nil logradouro",
			endereco: &models.EnderecoPrincipal{
				Bairro:    strPtr("Centro"),
				Municipio: strPtr("Rio de Janeiro"),
				Estado:    strPtr("RJ"),
			},
			expected: "",
		},
		{
			name: "address with empty logradouro",
			endereco: &models.EnderecoPrincipal{
				Logradouro: strPtr(""),
				Bairro:     strPtr("Centro"),
			},
			expected: "",
		},
		{
			name: "strips parenthetical neighborhood annotation",
			endereco: &models.EnderecoPrincipal{
				Logradouro: strPtr("Estrada dos Bandeirantes"),
				Numero:     strPtr("100"),
				Bairro:     strPtr("Freguesia (Jacarepaguá)"),
				Municipio:  strPtr("Rio de Janeiro"),
				Estado:     strPtr("RJ"),
			},
			expected: "Estrada dos Bandeirantes, 100, Freguesia, Rio de Janeiro, RJ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAddressString(tt.endereco)
			assert.Equal(t, tt.expected, result, "Address string should match expected value")
		})
	}
}

// strPtr is a helper to create string pointers for test data
func strPtr(s string) *string {
	return &s
}

// TestQueueCFLookupJob tests the queueCFLookupJob function
func TestQueueCFLookupJob(t *testing.T) {
	ctx := context.Background()
	cpf := "12345678901"
	address := "Rua Teste, 123, Centro, Rio de Janeiro, RJ"

	queueKey := "sync:queue:cf_lookup"
	config.Redis.Del(ctx, queueKey)
	config.Redis.Del(ctx, "cf_lookup:pending:"+cpf)

	t.Run("successfully queue CF lookup job", func(t *testing.T) {
		config.Redis.Del(ctx, queueKey)
		config.Redis.Del(ctx, "cf_lookup:pending:"+cpf)
		queueCFLookupJob(ctx, cpf, address)

		time.Sleep(10 * time.Millisecond)

		length, err := config.Redis.LLen(ctx, queueKey).Result()
		require.NoError(t, err, "Should be able to check queue length")
		assert.Equal(t, int64(1), length, "Queue should have 1 job")

		jobJSON, err := config.Redis.RPop(ctx, queueKey).Result()
		require.NoError(t, err, "Should be able to pop job from queue")

		var job services.SyncJob
		err = json.Unmarshal([]byte(jobJSON), &job)
		require.NoError(t, err, "Job should be valid JSON")

		assert.Equal(t, "cf_lookup", job.Type, "Job type should be cf_lookup")
		assert.Equal(t, cpf, job.Key, "Job key should be CPF")
		assert.Equal(t, "cf_lookup", job.Collection, "Job collection should be cf_lookup")
		assert.Equal(t, 0, job.RetryCount, "RetryCount should be 0")
		assert.Equal(t, 3, job.MaxRetries, "MaxRetries should be 3")

		jobData, ok := job.Data.(map[string]interface{})
		require.True(t, ok, "Job data should be a map")
		assert.Equal(t, cpf, jobData["cpf"], "Job data should contain CPF")
		assert.Equal(t, address, jobData["address"], "Job data should contain address")
	})

	t.Run("queue multiple jobs", func(t *testing.T) {
		cpfs := []string{"11111111111", "22222222222", "33333333333"}
		config.Redis.Del(ctx, queueKey)
		for _, c := range cpfs {
			config.Redis.Del(ctx, "cf_lookup:pending:"+c)
		}

		queueCFLookupJob(ctx, cpfs[0], "Address 1")
		queueCFLookupJob(ctx, cpfs[1], "Address 2")
		queueCFLookupJob(ctx, cpfs[2], "Address 3")

		time.Sleep(10 * time.Millisecond)

		length, err := config.Redis.LLen(ctx, queueKey).Result()
		require.NoError(t, err, "Should be able to check queue length")
		assert.Equal(t, int64(3), length, "Queue should have 3 jobs")
	})

	t.Run("skips duplicate job for same CPF while pending", func(t *testing.T) {
		dupCPF := "44444444444"
		config.Redis.Del(ctx, queueKey)
		config.Redis.Del(ctx, "cf_lookup:pending:"+dupCPF)

		queueCFLookupJob(ctx, dupCPF, "Address A")
		queueCFLookupJob(ctx, dupCPF, "Address B")

		time.Sleep(10 * time.Millisecond)

		length, err := config.Redis.LLen(ctx, queueKey).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), length, "Only one job should be queued while pending")
	})
}
