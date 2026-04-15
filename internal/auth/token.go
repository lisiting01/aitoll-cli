package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/config"
)

// StoredCredentials represents the saved credential file.
type StoredCredentials struct {
	Token      string `json:"token"`
	ObtainedAt string `json:"obtained_at"`
	BaseURL    string `json:"base_url"`
}

// TokenClaims represents the JWT payload claims.
type TokenClaims struct {
	UserID   string `json:"userId"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
	UserType string `json:"userType,omitempty"`
	Exp      int64  `json:"exp"`
}

// TokenStore handles reading/writing the credentials file.
type TokenStore struct{}

// NewTokenStore creates a new TokenStore.
func NewTokenStore() *TokenStore {
	return &TokenStore{}
}

// Save stores the token to the credentials file.
func (s *TokenStore) Save(token, baseURL string) error {
	if err := config.EnsureConfigDir(); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	path, err := config.CredentialsPath()
	if err != nil {
		return err
	}

	creds := StoredCredentials{
		Token:      token,
		ObtainedAt: time.Now().UTC().Format(time.RFC3339),
		BaseURL:    baseURL,
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write credentials: %w", err)
	}

	return nil
}

// Load reads the stored credentials. Returns nil if not found.
func (s *TokenStore) Load() (*StoredCredentials, error) {
	path, err := config.CredentialsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var creds StoredCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	return &creds, nil
}

// Delete removes the credentials file.
func (s *TokenStore) Delete() error {
	path, err := config.CredentialsPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// IsTokenValid checks if a stored token exists and is not expired.
func (s *TokenStore) IsTokenValid() (bool, error) {
	creds, err := s.Load()
	if err != nil || creds == nil {
		return false, err
	}

	claims, err := DecodeTokenClaims(creds.Token)
	if err != nil {
		return false, err
	}

	return time.Now().Unix() < claims.Exp, nil
}

// DecodeTokenClaims decodes the JWT payload without verifying the signature.
// It extracts userId, email, username, role, and exp from the token.
func DecodeTokenClaims(token string) (*TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	return &claims, nil
}
