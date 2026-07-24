// Package sso implements minimal OpenID Connect authentication.
//
// Only the Authorization Code flow is supported. Discovery is performed once
// per provider via /.well-known/openid-configuration; tokens are exchanged at
// the token endpoint, and the ID token is verified against the provider JWKS.
//
// We deliberately do not use go-oidc to keep dependencies small — this file is
// ~200 lines including JWKS handling.
package sso

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Provider struct {
	ID             string
	Name           string
	Issuer         string
	ClientID       string
	ClientSecret   string
	Scopes         string
	AllowedDomains string
	DefaultRole    string
	Enabled        bool
}

type discoveryDoc struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Use string `json:"use"`
	Alg string `json:"alg"`
}

type jwksDoc struct {
	Keys []jwksKey `json:"keys"`
}

type cachedProvider struct {
	disco    discoveryDoc
	keys     map[string]*rsa.PublicKey
	expires  time.Time
}

type Client struct {
	http  *http.Client
	cache sync.Map // issuer -> *cachedProvider
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 10 * time.Second}}
}

// UserInfo is what we extract from the verified ID token + userinfo.
type UserInfo struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

func (c *Client) discover(ctx context.Context, issuer string) (*cachedProvider, error) {
	if v, ok := c.cache.Load(issuer); ok {
		cp := v.(*cachedProvider)
		if time.Now().Before(cp.expires) {
			return cp, nil
		}
	}
	base := strings.TrimRight(issuer, "/")
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("discovery failed: %s", resp.Status)
	}
	var d discoveryDoc
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	keys, err := c.fetchJWKS(ctx, d.JWKSURI)
	if err != nil {
		return nil, err
	}
	cp := &cachedProvider{disco: d, keys: keys, expires: time.Now().Add(1 * time.Hour)}
	c.cache.Store(issuer, cp)
	return cp, nil
}

func (c *Client) fetchJWKS(ctx context.Context, uri string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var j jwksDoc
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}
	out := make(map[string]*rsa.PublicKey, len(j.Keys))
	for _, k := range j.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := 0
		for _, b := range eb {
			e = e<<8 | int(b)
		}
		// pad to 4 bytes for binary read
		if len(eb) < 4 {
			buf := make([]byte, 4)
			copy(buf[4-len(eb):], eb)
			e = int(binary.BigEndian.Uint32(buf))
		}
		out[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}
	}
	return out, nil
}

// AuthURL builds the authorization URL and returns it along with state + nonce
// values the caller must persist in an HTTP-only cookie for the callback.
func (c *Client) AuthURL(ctx context.Context, p *Provider, redirectURI string) (authURL, state, nonce string, err error) {
	cp, err := c.discover(ctx, p.Issuer)
	if err != nil {
		return "", "", "", err
	}
	state = randomHex(16)
	nonce = randomHex(16)
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", p.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", p.Scopes)
	q.Set("state", state)
	q.Set("nonce", nonce)
	return cp.disco.AuthorizationEndpoint + "?" + q.Encode(), state, nonce, nil
}

type tokenResponse struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Exchange swaps the auth code for tokens, verifies the ID token, and returns
// the authenticated user.
func (c *Client) Exchange(ctx context.Context, p *Provider, code, redirectURI, expectedNonce string) (*UserInfo, error) {
	cp, err := c.discover(ctx, p.Issuer)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", p.ClientID)
	form.Set("client_secret", p.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", cp.disco.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (%s): %s", resp.Status, string(body))
	}
	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, err
	}
	if tok.IDToken == "" {
		return nil, errors.New("no id_token in response")
	}

	claims, err := c.verifyIDToken(tok.IDToken, cp, p.ClientID)
	if err != nil {
		return nil, err
	}
	if expectedNonce != "" {
		nc, _ := claims["nonce"].(string)
		if subtle.ConstantTimeCompare([]byte(nc), []byte(expectedNonce)) != 1 {
			return nil, errors.New("nonce mismatch")
		}
	}
	info := &UserInfo{
		Subject: asString(claims["sub"]),
		Email:   asString(claims["email"]),
		Name:    asString(claims["name"]),
	}
	// Some IdPs (notably certain Azure AD / Auth0 configurations) send
	// email_verified as the string "true" rather than a JSON bool.
	switch v := claims["email_verified"].(type) {
	case bool:
		info.EmailVerified = v
	case string:
		info.EmailVerified = v == "true"
	}
	if info.Subject == "" {
		return nil, errors.New("id token missing sub")
	}
	return info, nil
}

func (c *Client) verifyIDToken(tokenStr string, cp *cachedProvider, audience string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(jwt.WithIssuer(cp.disco.Issuer), jwt.WithAudience(audience))
	token, err := parser.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		key, ok := cp.keys[kid]
		if !ok {
			return nil, fmt.Errorf("no matching key for kid %q", kid)
		}
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid id token")
	}
	return claims, nil
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// HashState returns a short stable hash of a state string, used for logging.
func HashState(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}
