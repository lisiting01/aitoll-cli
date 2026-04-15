package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// UserInfo represents the user info returned by /api/auth/me.
type UserInfo struct {
	ID                  string `json:"id"`
	Username            string `json:"username"`
	Email               string `json:"email"`
	Role                string `json:"role"`
	EmailVerified       bool   `json:"emailVerified"`
	PasswordResetRequired bool `json:"passwordResetRequired"`
	Timezone            string `json:"timezone,omitempty"`
	LastLoginAt         string `json:"lastLoginAt,omitempty"`
	CreatedAt           string `json:"createdAt,omitempty"`
}

// MeResponse represents the response from /api/auth/me.
type MeResponse struct {
	Success bool      `json:"success"`
	Data    *UserInfo `json:"data"`
	Error   string    `json:"error,omitempty"`
}

// Client is a simple HTTP client for the AI-Toll platform API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetMe calls GET /api/auth/me with the given token and returns user info.
func (c *Client) GetMe(token string) (*UserInfo, error) {
	url := fmt.Sprintf("%s/api/auth/me", c.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var meResp MeResponse
	if err := json.Unmarshal(body, &meResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !meResp.Success {
		return nil, fmt.Errorf("API error: %s", meResp.Error)
	}

	return meResp.Data, nil
}
