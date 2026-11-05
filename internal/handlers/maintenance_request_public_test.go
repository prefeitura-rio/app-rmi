package handlers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

const (
	protocoloTest      = "RIO-87654321-0"
	protocoloOwnerCPF  = "11144477735"
	protocoloOtherCPF  = "52998224725"
	protocoloTestKid   = "protocolo-test-kid"
	protocoloOtherProt = "RIO-87654321-1"
)

type protocoloTestIssuer struct {
	issuer string
	key    *rsa.PrivateKey
}

func newProtocoloTestIssuer(t *testing.T) *protocoloTestIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kid": protocoloTestKid,
				"kty": "RSA",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	}))
	t.Cleanup(srv.Close)
	return &protocoloTestIssuer{issuer: srv.URL + "/realms/test", key: key}
}

func (i *protocoloTestIssuer) token(t *testing.T, key *rsa.PrivateKey, preferredUsername string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":                i.issuer,
		"preferred_username": preferredUsername,
		"exp":                time.Now().Add(5 * time.Minute).Unix(),
	})
	tok.Header["kid"] = protocoloTestKid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func seedProtocoloDocuments(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	coll := config.MongoDB.Collection(config.AppConfig.MaintenanceRequestCollection)
	cleanup := func() {
		_, _ = coll.DeleteMany(ctx, bson.M{"protocolo": bson.M{"$in": bson.A{protocoloTest, protocoloOtherProt}}})
		_ = config.Redis.Del(ctx, "maintenance_request:protocolo:"+protocoloTest).Err()
	}
	cleanup()
	t.Cleanup(cleanup)

	_, err := coll.InsertMany(ctx, []interface{}{
		bson.M{
			"_id": "protocolo-test-1", "cpf": protocoloOwnerCPF, "id": "9000001", "protocolo": protocoloTest,
			"categoria": "Serviço", "tipo": "Iluminação Pública", "status": "Aberto",
			"data_inicio": "2025-01-10T10:00:00", "descricao": "descricao do titular",
			"endereco": "Rua Sintetica, 1 - Centro", "bairro": "Centro", "logradouro": "Rua Sintetica, 1",
			"loaded_at": time.Date(2025, 11, 3, 3, 0, 0, 0, time.UTC),
			"andamentos": bson.A{
				bson.M{"data": "2025-01-10T10:05:00", "tipo": "Abertura", "descricao": "andamento publico", "fl_publico": int32(1)},
				bson.M{"data": "2025-01-11T10:05:00", "tipo": "Interno", "descricao": "andamento interno", "fl_publico": int32(0)},
			},
		},
		bson.M{
			"_id": "protocolo-test-2", "cpf": protocoloOwnerCPF, "id": "9000002", "protocolo": protocoloTest,
			"categoria": "Serviço", "tipo": "Conservação", "status": "Fechado",
			"data_inicio": "2025-01-09T10:00:00", "descricao": "segunda descricao", "bairro": "Centro",
			"logradouro": "Rua Sintetica, 1",
		},
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func protocoloRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v1/maintenance-request/:protocolo", GetMaintenanceRequestByProtocolo)
	return r
}

func getProtocolo(t *testing.T, r *gin.Engine, protocolo, authHeader string) (int, models.PublicMaintenanceRequestResponse, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/maintenance-request/"+protocolo, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body models.PublicMaintenanceRequestResponse
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
	}
	return w.Code, body, w.Body.String()
}

func assertPublicView(t *testing.T, raw string, body models.PublicMaintenanceRequestResponse) {
	t.Helper()
	if body.Protocolo != protocoloTest || len(body.Chamados) != 2 {
		t.Fatalf("body = %s, want the two chamados of %s", raw, protocoloTest)
	}
	for _, leaked := range []string{protocoloOwnerCPF, "descricao do titular", "segunda descricao", "Rua Sintetica", "andamento interno", "logradouro", "endereco", "\"cpf\""} {
		if strings.Contains(raw, leaked) {
			t.Fatalf("public response leaks %q: %s", leaked, raw)
		}
	}
	first := body.Chamados[0]
	if first.IDChamado != "9000001" || first.Bairro != "Centro" || first.LoadedAt != "2025-11-03T03:00:00Z" || first.Titular {
		t.Fatalf("first chamado = %+v", first)
	}
	if len(first.Andamentos) != 1 || first.Andamentos[0].Descricao != "andamento publico" {
		t.Fatalf("andamentos = %+v, want only the public one", first.Andamentos)
	}
}

func TestGetMaintenanceRequestByProtocolo(t *testing.T) {
	seedProtocoloDocuments(t)
	issuer := newProtocoloTestIssuer(t)
	SetMaintenanceRequestJWTVerifier(middleware.NewJWTVerifier(issuer.issuer, nil))
	t.Cleanup(func() { SetMaintenanceRequestJWTVerifier(nil) })
	r := protocoloRouter()

	t.Run("anonymous gets public view", func(t *testing.T) {
		code, body, raw := getProtocolo(t, r, protocoloTest, "")
		if code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", code, raw)
		}
		assertPublicView(t, raw, body)
	})

	t.Run("lowercase protocolo is normalized", func(t *testing.T) {
		code, body, raw := getProtocolo(t, r, strings.ToLower(protocoloTest), "")
		if code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", code, raw)
		}
		assertPublicView(t, raw, body)
	})

	t.Run("verified JWT of another person gets public view", func(t *testing.T) {
		code, body, raw := getProtocolo(t, r, protocoloTest, "Bearer "+issuer.token(t, issuer.key, protocoloOtherCPF))
		if code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", code, raw)
		}
		assertPublicView(t, raw, body)
	})

	t.Run("forged JWT with the owner CPF gets public view", func(t *testing.T) {
		forgedKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		code, body, raw := getProtocolo(t, r, protocoloTest, "Bearer "+issuer.token(t, forgedKey, protocoloOwnerCPF))
		if code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", code, raw)
		}
		assertPublicView(t, raw, body)
	})

	t.Run("unsigned JWT with the owner CPF gets public view", func(t *testing.T) {
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"` + issuer.issuer + `","preferred_username":"` + protocoloOwnerCPF + `"}`))
		code, body, raw := getProtocolo(t, r, protocoloTest, "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9."+payload+".fake-signature")
		if code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", code, raw)
		}
		assertPublicView(t, raw, body)
	})

	t.Run("verified owner gets street and description", func(t *testing.T) {
		code, body, raw := getProtocolo(t, r, protocoloTest, "Bearer "+issuer.token(t, issuer.key, protocoloOwnerCPF))
		if code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", code, raw)
		}
		if strings.Contains(raw, protocoloOwnerCPF) || strings.Contains(raw, "andamento interno") {
			t.Fatalf("owner response leaks CPF or internal andamento: %s", raw)
		}
		first := body.Chamados[0]
		if !first.Titular || first.Logradouro != "Rua Sintetica, 1" || first.Descricao != "descricao do titular" {
			t.Fatalf("owner chamado = %+v", first)
		}
	})

	t.Run("owner without verifier configured gets public view", func(t *testing.T) {
		token := issuer.token(t, issuer.key, protocoloOwnerCPF)
		SetMaintenanceRequestJWTVerifier(nil)
		defer SetMaintenanceRequestJWTVerifier(middleware.NewJWTVerifier(issuer.issuer, nil))
		code, body, raw := getProtocolo(t, r, protocoloTest, "Bearer "+token)
		if code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", code, raw)
		}
		assertPublicView(t, raw, body)
	})

	t.Run("unknown protocolo is 404", func(t *testing.T) {
		if code, _, raw := getProtocolo(t, r, protocoloOtherProt, ""); code != http.StatusNotFound {
			t.Fatalf("status = %d, body = %s, want 404", code, raw)
		}
	})

	t.Run("invalid protocolo is 400", func(t *testing.T) {
		for _, p := range []string{"12345678", "RIO-1234-5", "RIO-12345678-99", "RIO-1234567A-9", "%7B%22%24ne%22%3A1%7D"} {
			if code, _, raw := getProtocolo(t, r, p, ""); code != http.StatusBadRequest {
				t.Fatalf("protocolo %q: status = %d, body = %s, want 400", p, code, raw)
			}
		}
	})
}

func TestNormalizeProtocoloSGRC(t *testing.T) {
	cases := map[string]struct {
		want  string
		valid bool
	}{
		"RIO-12345678-9":   {"RIO-12345678-9", true},
		" rio-12345678-9 ": {"RIO-12345678-9", true},
		"RIO-1234567-9":    {"RIO-1234567-9", false},
		"RIO-12345678-":    {"RIO-12345678-", false},
		"RIO-12345678-9\n": {"RIO-12345678-9", true},
		"RIO-12345678-9 x": {"RIO-12345678-9 X", false},
		"":                 {"", false},
		"RIO-１２３４５６７８-9":   {"RIO-１２３４５６７８-9", false},
	}
	for in, tc := range cases {
		got, ok := NormalizeProtocoloSGRC(in)
		if got != tc.want || ok != tc.valid {
			t.Errorf("NormalizeProtocoloSGRC(%q) = (%q, %v), want (%q, %v)", in, got, ok, tc.want, tc.valid)
		}
	}
}
