package middleware

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	jwksMaxBodyBytes          = 1 << 20
	jwksDefaultCacheTTL       = time.Hour
	jwksDefaultMinRefresh     = time.Minute
	jwksDefaultRequestTimeout = 3 * time.Second
	jwtVerifierLeeway         = 30 * time.Second
)

// ErrJWTVerifierDisabled is returned when no issuer is configured.
var ErrJWTVerifierDisabled = errors.New("jwt verifier disabled: issuer not configured")

// JWTVerifier verifies the signature, issuer and expiry of Keycloak access tokens against the
// realm JWKS. Routes whose gateway action is public reach the API without any token
// validation by Cerbos, so a claim read there must be verified here before it is trusted.
type JWTVerifier struct {
	issuer     string
	jwksURL    string
	client     *http.Client
	cacheTTL   time.Duration
	minRefresh time.Duration
	now        func() time.Time

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	fetchedAt   time.Time
	lastAttempt time.Time
}

type verifiedClaims struct {
	PreferredUsername string `json:"preferred_username"`
	jwt.RegisteredClaims
}

// NewJWTVerifier builds a verifier for a Keycloak realm issuer URL
// (https://<host>/realms/<realm>). An empty issuer yields nil: callers must treat every token
// as unverified.
func NewJWTVerifier(issuer string, client *http.Client) *JWTVerifier {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: jwksDefaultRequestTimeout}
	}
	return &JWTVerifier{
		issuer:     issuer,
		jwksURL:    issuer + "/protocol/openid-connect/certs",
		client:     client,
		cacheTTL:   jwksDefaultCacheTTL,
		minRefresh: jwksDefaultMinRefresh,
		now:        time.Now,
	}
}

// VerifiedPreferredUsername verifies the token and returns its preferred_username claim.
func (v *JWTVerifier) VerifiedPreferredUsername(ctx context.Context, token string) (string, error) {
	if v == nil {
		return "", ErrJWTVerifierDisabled
	}
	claims := &verifiedClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims,
		func(t *jwt.Token) (interface{}, error) {
			kid, _ := t.Header["kid"].(string)
			if kid == "" {
				return nil, errors.New("token without kid")
			}
			return v.publicKey(ctx, kid)
		},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(jwtVerifierLeeway),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil {
		return "", err
	}
	if !parsed.Valid {
		return "", errors.New("invalid token")
	}
	if claims.PreferredUsername == "" {
		return "", errors.New("token without preferred_username")
	}
	return claims.PreferredUsername, nil
}

// publicKey returns the key for kid, refreshing the JWKS when the cache is stale or the kid is
// unknown. Refreshes are spaced by minRefresh so tokens with random kids cannot flood Keycloak.
func (v *JWTVerifier) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := v.now()
	key, known := v.keys[kid]
	fresh := v.keys != nil && now.Sub(v.fetchedAt) < v.cacheTTL
	if known && fresh {
		return key, nil
	}

	if v.lastAttempt.IsZero() || now.Sub(v.lastAttempt) >= v.minRefresh {
		v.lastAttempt = now
		keys, err := v.fetchJWKS(ctx)
		if err == nil {
			v.keys = keys
			v.fetchedAt = now
			key, known = v.keys[kid]
		} else if !known {
			return nil, fmt.Errorf("jwks unavailable: %w", err)
		}
	}

	if !known {
		return nil, fmt.Errorf("unknown kid %q", kid)
	}
	return key, nil
}

type jwksDocument struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (v *JWTVerifier) fetchJWKS(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	reqCtx, cancel := context.WithTimeout(ctx, jwksDefaultRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks status %d", resp.StatusCode)
	}

	var doc jwksDocument
	if err := json.NewDecoder(io.LimitReader(resp.Body, jwksMaxBodyBytes)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" || (k.Use != "" && k.Use != "sig") {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks without usable RSA signing keys")
	}
	return keys, nil
}

func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	e := new(big.Int).SetBytes(eBytes)
	if !e.IsInt64() || e.Int64() < 3 || e.Int64() > 1<<31-1 {
		return nil, errors.New("invalid RSA exponent")
	}
	n := new(big.Int).SetBytes(nBytes)
	if n.BitLen() < 2048 {
		return nil, errors.New("RSA modulus too short")
	}
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// BearerToken returns the token of an "Authorization: Bearer <token>" header, or "".
func BearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
