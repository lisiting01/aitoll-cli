package api

import (
	"bytes"
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

// PasscardService is a service entry on a passcard.
type PasscardService struct {
	ServiceID   string `json:"serviceId"`
	ServiceName string `json:"serviceName"`
}

// Passcard represents a single passcard entry.
type Passcard struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Enabled     bool              `json:"enabled"`
	Services    []PasscardService `json:"services"`
}

// PasscardsResponse is the response from GET /api/user/passes.
type PasscardsResponse struct {
	Success bool       `json:"success"`
	Data    []Passcard `json:"data"`
	Error   string     `json:"error,omitempty"`
}

// Line represents a single route/line option within a service.
type Line struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ServicePreference represents one service's lines and current selection.
type ServicePreference struct {
	ServiceID      string `json:"serviceId"`
	ServiceName    string `json:"serviceName"`
	Lines          []Line `json:"lines"`
	SelectedLineID string `json:"selectedLineId"`
}

// PasscardPreferenceData is the data payload for the preference response.
type PasscardPreferenceData struct {
	Services []ServicePreference `json:"services"`
}

// PasscardPreferenceResponse is the response from GET /api/passes/{id}/preference.
type PasscardPreferenceResponse struct {
	Success bool                    `json:"success"`
	Data    *PasscardPreferenceData `json:"data"`
	Error   string                  `json:"error,omitempty"`
}

// SwitchLineRequest is the request body for PATCH /api/passes/{id}/preference.
type SwitchLineRequest struct {
	ServiceID      string  `json:"serviceId"`
	SelectedLineID *string `json:"selectedLineId"` // nil marshals as JSON null
}

// SwitchLineResponse is the response from PATCH /api/passes/{id}/preference.
type SwitchLineResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
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

// ListPasscards calls GET /api/user/passes and returns all passcards for the authenticated user.
func (c *Client) ListPasscards(token string) ([]Passcard, error) {
	url := fmt.Sprintf("%s/api/user/passes", c.baseURL)
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

	var result PasscardsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("API error: %s", result.Error)
	}

	return result.Data, nil
}

// GetPasscardPreference calls GET /api/passes/{id}/preference and returns available lines and current selection.
func (c *Client) GetPasscardPreference(token string, passcardID string) (*PasscardPreferenceData, error) {
	url := fmt.Sprintf("%s/api/passes/%s/preference", c.baseURL, passcardID)
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

	var result PasscardPreferenceResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("API error: %s", result.Error)
	}

	return result.Data, nil
}

// SwitchPasscardLine calls PATCH /api/passes/{id}/preference to switch the active line for a service.
// Pass nil for selectedLineID to reset to automatic selection.
func (c *Client) SwitchPasscardLine(token string, passcardID string, serviceID string, selectedLineID *string) error {
	reqBody := SwitchLineRequest{
		ServiceID:      serviceID,
		SelectedLineID: selectedLineID,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/passes/%s/preference", c.baseURL, passcardID)
	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var result SwitchLineResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("API error: %s", result.Error)
	}

	return nil
}

// CurrentPasscardResponse is the response from GET /api/passes/current.
type CurrentPasscardResponse struct {
	Success bool      `json:"success"`
	Data    *Passcard `json:"data"`
	Error   string    `json:"error,omitempty"`
}

// GetCurrentPasscard calls GET /api/passes/current with a passcard API key (not JWT).
func (c *Client) GetCurrentPasscard(apiKey string) (*Passcard, error) {
	url := fmt.Sprintf("%s/api/passes/current", c.baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result CurrentPasscardResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("API error: %s", result.Error)
	}

	return result.Data, nil
}
