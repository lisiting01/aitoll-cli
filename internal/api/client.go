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

// GetProxyConfig calls GET /api/proxy/config and returns the raw sing-box
// JSON config the backend has provisioned for this user. The CLI does not
// interpret the body beyond a basic JSON-shape check at the caller.
func (c *Client) GetProxyConfig(token string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/proxy/config", c.baseURL)
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
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

// =============================================================================
// codex-auth (lease 租赁池) — v0.7.0+
// =============================================================================

// LeaseRequest is the body sent to POST /api/codex-auth/lease.
type LeaseRequest struct {
	Duration string `json:"duration,omitempty"` // "7d" / "14d" / "30d"; default 14d
	Force    bool   `json:"force,omitempty"`
}

// LeaseOpenAIInfo carries the OpenAI account credentials shown to the user
// once at lease time. Password is null when admin hasn't set one.
type LeaseOpenAIInfo struct {
	Email       string  `json:"email"`
	Password    *string `json:"password"`
	AccountType string  `json:"accountType"`
}

// LeaseData is the success payload of /api/codex-auth/lease.
type LeaseData struct {
	LeaseID   string                 `json:"leaseId"`
	ExpiresAt string                 `json:"expiresAt"`
	RefreshAt string                 `json:"refreshAt,omitempty"`
	OpenAI    LeaseOpenAIInfo        `json:"openai"`
	AuthJSON  map[string]interface{} `json:"authJson"`
}

// EnvelopeError is parsed when an envelope-style endpoint returns success=false.
type EnvelopeError struct {
	Code    string // body.error
	Message string // body.message (optional)
	Status  int    // HTTP status
}

func (e *EnvelopeError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s (%s, http %d)", e.Message, e.Code, e.Status)
	}
	return fmt.Sprintf("%s (http %d)", e.Code, e.Status)
}

// LeaseAccount calls POST /api/codex-auth/lease.
func (c *Client) LeaseAccount(token string, req LeaseRequest) (*LeaseData, error) {
	var resp struct {
		Success bool       `json:"success"`
		Data    *LeaseData `json:"data,omitempty"`
		Error   string     `json:"error,omitempty"`
		Message string     `json:"message,omitempty"`
	}
	status, err := c.doJSONEnvelope(token, "POST", "/api/codex-auth/lease", req, &resp)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, &EnvelopeError{Code: resp.Error, Message: resp.Message, Status: status}
	}
	return resp.Data, nil
}

// ReleaseLease calls POST /api/codex-auth/release.
// hadActiveLease==false means the user had no active lease (no-op release).
// alreadyExpired==true means the lease was past its expiresAt before release.
func (c *Client) ReleaseLease(token string) (alreadyExpired bool, hadActiveLease bool, err error) {
	var resp struct {
		Success bool `json:"success"`
		Data    *struct {
			ReleasedAt     string `json:"releasedAt"`
			AlreadyExpired bool   `json:"alreadyExpired"`
		} `json:"data"`
		Error   string `json:"error,omitempty"`
		Message string `json:"message,omitempty"`
	}
	status, err := c.doJSONEnvelope(token, "POST", "/api/codex-auth/release", struct{}{}, &resp)
	if err != nil {
		return false, false, err
	}
	if !resp.Success {
		return false, false, &EnvelopeError{Code: resp.Error, Message: resp.Message, Status: status}
	}
	if resp.Data == nil {
		return false, false, nil
	}
	return resp.Data.AlreadyExpired, true, nil
}

// CodexAuthStatus is the result of GET /api/codex-auth/status.
type CodexAuthStatus struct {
	Active          bool   `json:"active"`
	LeaseID         string `json:"leaseId,omitempty"`
	OpenAIEmail     string `json:"openaiEmail,omitempty"`
	AccountType     string `json:"accountType,omitempty"`
	LeasedAt        string `json:"leasedAt,omitempty"`
	ExpiresAt       string `json:"expiresAt,omitempty"`
	LastRefreshedAt string `json:"lastRefreshedAt,omitempty"`
}

// GetCodexAuthStatus calls GET /api/codex-auth/status.
func (c *Client) GetCodexAuthStatus(token string) (*CodexAuthStatus, error) {
	var resp struct {
		Success bool             `json:"success"`
		Data    *CodexAuthStatus `json:"data"`
		Error   string           `json:"error,omitempty"`
		Message string           `json:"message,omitempty"`
	}
	status, err := c.doJSONEnvelope(token, "GET", "/api/codex-auth/status", nil, &resp)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, &EnvelopeError{Code: resp.Error, Message: resp.Message, Status: status}
	}
	return resp.Data, nil
}

// RefreshData is the success payload of /api/codex-auth/refresh.
type RefreshData struct {
	AuthJSON    map[string]interface{} `json:"authJson"`
	RefreshedAt string                 `json:"refreshedAt"`
	ExpiresAt   string                 `json:"expiresAt"`
}

// RefreshLease calls POST /api/codex-auth/refresh.
func (c *Client) RefreshLease(token string) (*RefreshData, error) {
	var resp struct {
		Success bool         `json:"success"`
		Data    *RefreshData `json:"data"`
		Error   string       `json:"error,omitempty"`
		Message string       `json:"message,omitempty"`
	}
	status, err := c.doJSONEnvelope(token, "POST", "/api/codex-auth/refresh", struct{}{}, &resp)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, &EnvelopeError{Code: resp.Error, Message: resp.Message, Status: status}
	}
	return resp.Data, nil
}

// doJSONEnvelope performs a JSON request/response cycle for endpoints that
// follow the envelope pattern. body may be nil; out must be a pointer.
// Returns the HTTP status code and any transport-level error.
func (c *Client) doJSONEnvelope(
	token, method, path string,
	body interface{},
	out interface{},
) (int, error) {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if len(respBody) == 0 {
		return resp.StatusCode, fmt.Errorf("backend returned empty body (http %d)", resp.StatusCode)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return resp.StatusCode, fmt.Errorf("parse response (http %d): %w; body: %s", resp.StatusCode, err, truncate(string(respBody), 200))
	}
	return resp.StatusCode, nil
}
