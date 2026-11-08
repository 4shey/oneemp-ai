package sheets

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"one_more_percent/internal/config"
)

type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type tokenSource struct {
	email      string
	privateKey *rsa.PrivateKey
	tokenURI   string
	scope      string
	httpClient *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func newTokenSource(cfg *config.Config) (*tokenSource, error) {
	raw := []byte(cfg.GoogleCredentialsJSON)
	if len(raw) == 0 {
		b, err := os.ReadFile(cfg.GoogleCredentialsFile)
		if err != nil {
			return nil, fmt.Errorf("baca GOOGLE_CREDENTIALS_FILE: %w", err)
		}
		raw = b
	}

	var sa serviceAccount
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, fmt.Errorf("parse kredensial Google: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" || sa.TokenURI == "" {
		return nil, fmt.Errorf("kredensial Google tidak lengkap (client_email, private_key, token_uri)")
	}

	key, err := parsePrivateKey(sa.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &tokenSource{
		email:      sa.ClientEmail,
		privateKey: key,
		tokenURI:   sa.TokenURI,
		scope:      cfg.GoogleScope,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func parsePrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("private_key bukan PEM yang valid")
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private_key bukan RSA")
		}
		return rsaKey, nil
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private_key: %w", err)
	}
	return key, nil
}

func (t *tokenSource) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token != "" && time.Until(t.expires) > time.Minute {
		return t.token, nil
	}

	assertion, err := t.signedJWT()
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("buat request token Google: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("minta token Google: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("token Google gagal HTTP %d: %s", resp.StatusCode, truncate(string(body), 400))
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse token Google: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("token Google kosong")
	}

	t.token = parsed.AccessToken
	expires := parsed.ExpiresIn
	if expires <= 0 {
		expires = 3600
	}
	t.expires = time.Now().Add(time.Duration(expires) * time.Second)
	return t.token, nil
}

func (t *tokenSource) signedJWT() (string, error) {
	now := time.Now()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":   t.email,
		"scope": t.scope,
		"aud":   t.tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)

	hashed := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, t.privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("tanda tangan JWT: %w", err)
	}

	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
