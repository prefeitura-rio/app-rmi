package middleware

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type jwksTestServer struct {
	server   *httptest.Server
	issuer   string
	key      *rsa.PrivateKey
	kid      string
	status   atomic.Int32
	requests atomic.Int32
}

func newJWKSTestServer(t *testing.T) *jwksTestServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	s := &jwksTestServer{key: key, kid: "test-kid"}
	s.status.Store(http.StatusOK)
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		if r.URL.Path != "/realms/test/protocol/openid-connect/certs" {
			http.NotFound(w, r)
			return
		}
		if code := int(s.status.Load()); code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{
				"kid": s.kid,
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	}))
	s.issuer = s.server.URL + "/realms/test"
	t.Cleanup(s.server.Close)
	return s
}

func (s *jwksTestServer) sign(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func (s *jwksTestServer) validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":                s.issuer,
		"sub":                "user-1",
		"preferred_username": "11144477735",
		"exp":                time.Now().Add(5 * time.Minute).Unix(),
		"iat":                time.Now().Unix(),
	}
}

func TestJWTVerifier_ValidTokenReturnsPreferredUsername(t *testing.T) {
	s := newJWKSTestServer(t)
	v := NewJWTVerifier(s.issuer+"/", nil)

	got, err := v.VerifiedPreferredUsername(context.Background(), s.sign(t, s.key, s.kid, s.validClaims()))
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if got != "11144477735" {
		t.Fatalf("preferred_username = %q, want 11144477735", got)
	}
}

func TestJWTVerifier_RejectsInvalidTokens(t *testing.T) {
	s := newJWKSTestServer(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	expired := s.validClaims()
	expired["exp"] = time.Now().Add(-10 * time.Minute).Unix()
	wrongIssuer := s.validClaims()
	wrongIssuer["iss"] = "https://attacker.example/realms/test"
	noExp := s.validClaims()
	delete(noExp, "exp")
	noUsername := s.validClaims()
	delete(noUsername, "preferred_username")

	hmacToken := jwt.NewWithClaims(jwt.SigningMethodHS256, s.validClaims())
	hmacToken.Header["kid"] = s.kid
	hmacSigned, err := hmacToken.SignedString([]byte("shared-secret"))
	if err != nil {
		t.Fatalf("sign hmac: %v", err)
	}

	validSigned := s.sign(t, s.key, s.kid, s.validClaims())
	parts := strings.Split(validSigned, ".")
	forgedPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"` + s.issuer + `","preferred_username":"99999999999","exp":` +
		big.NewInt(time.Now().Add(time.Hour).Unix()).String() + `}`))
	tampered := parts[0] + "." + forgedPayload + "." + parts[2]

	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT","kid":"test-kid"}`)) + "." +
		forgedPayload + "."

	cases := map[string]string{
		"signed by another key":  s.sign(t, otherKey, s.kid, s.validClaims()),
		"unknown kid":            s.sign(t, otherKey, "other-kid", s.validClaims()),
		"expired":                s.sign(t, s.key, s.kid, expired),
		"wrong issuer":           s.sign(t, s.key, s.kid, wrongIssuer),
		"without exp":            s.sign(t, s.key, s.kid, noExp),
		"without username":       s.sign(t, s.key, s.kid, noUsername),
		"hmac algorithm":         hmacSigned,
		"tampered payload":       tampered,
		"alg none":               unsigned,
		"not a jwt":              "not-a-token",
		"payload-only fake sign": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + forgedPayload + ".fake-signature",
	}

	v := NewJWTVerifier(s.issuer, nil)
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := v.VerifiedPreferredUsername(context.Background(), token)
			if err == nil {
				t.Fatalf("token accepted with preferred_username %q", got)
			}
			if got != "" {
				t.Fatalf("rejected token leaked preferred_username %q", got)
			}
		})
	}
}

func TestJWTVerifier_JWKSUnavailableRejects(t *testing.T) {
	s := newJWKSTestServer(t)
	s.status.Store(http.StatusInternalServerError)
	v := NewJWTVerifier(s.issuer, nil)

	if _, err := v.VerifiedPreferredUsername(context.Background(), s.sign(t, s.key, s.kid, s.validClaims())); err == nil {
		t.Fatal("token accepted while JWKS is unavailable")
	}
}

func TestJWTVerifier_IssuerUnreachableRejects(t *testing.T) {
	s := newJWKSTestServer(t)
	token := s.sign(t, s.key, s.kid, s.validClaims())
	v := NewJWTVerifier(s.issuer, nil)
	s.server.Close()

	if _, err := v.VerifiedPreferredUsername(context.Background(), token); err == nil {
		t.Fatal("token accepted while issuer is unreachable")
	}
}

func TestJWTVerifier_UnknownKidRefreshIsThrottled(t *testing.T) {
	s := newJWKSTestServer(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	v := NewJWTVerifier(s.issuer, nil)

	if _, err := v.VerifiedPreferredUsername(context.Background(), s.sign(t, s.key, s.kid, s.validClaims())); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	for i := 0; i < 20; i++ {
		_, _ = v.VerifiedPreferredUsername(context.Background(), s.sign(t, otherKey, "random-kid", s.validClaims()))
	}
	if got := s.requests.Load(); got != 1 {
		t.Fatalf("JWKS fetched %d times, want 1 (refresh must be throttled)", got)
	}
}

func TestJWTVerifier_KeepsCachedKeyWhenRefreshFails(t *testing.T) {
	s := newJWKSTestServer(t)
	now := time.Now()
	v := NewJWTVerifier(s.issuer, nil)
	v.now = func() time.Time { return now }

	if _, err := v.VerifiedPreferredUsername(context.Background(), s.sign(t, s.key, s.kid, s.validClaims())); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}

	s.status.Store(http.StatusServiceUnavailable)
	now = now.Add(2 * time.Hour)
	claims := s.validClaims()
	claims["exp"] = now.Add(5 * time.Minute).Unix()
	if _, err := v.VerifiedPreferredUsername(context.Background(), s.sign(t, s.key, s.kid, claims)); err != nil {
		t.Fatalf("cached key not used when refresh fails: %v", err)
	}
}

func TestJWTVerifier_DisabledWithoutIssuer(t *testing.T) {
	if v := NewJWTVerifier("  ", nil); v != nil {
		t.Fatal("verifier built without issuer")
	}
	var v *JWTVerifier
	if _, err := v.VerifiedPreferredUsername(context.Background(), "x.y.z"); err != ErrJWTVerifierDisabled {
		t.Fatalf("nil verifier error = %v, want ErrJWTVerifierDisabled", err)
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]string{
		"Bearer abc.def.ghi": "abc.def.ghi",
		"bearer abc":         "abc",
		"":                   "",
		"Basic abc":          "",
		"Bearer":             "",
		"Bearer a b":         "",
	}
	for header, want := range cases {
		if got := BearerToken(header); got != want {
			t.Errorf("BearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}
