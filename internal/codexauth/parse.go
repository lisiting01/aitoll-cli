package codexauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AuthJSONInfo summarizes the interesting fields of ~/.codex/auth.json
// without exposing tokens. Used by `aitoll codex-auth whoami` /
// `status` / `doctor`.
type AuthJSONInfo struct {
	AuthMode    string    // typically "ChatGPT" or "ApiKey"
	Email       string    // OpenAI account email (parsed from id_token)
	AccountID   string    // chatgpt account_id
	PlanType    string    // Free / Plus / Pro / Team
	LastRefresh time.Time // tokens.access_token / tokens.id_token last refresh
}

// ParseAuthJSON extracts a summary from a parsed auth.json blob. It is
// tolerant: missing fields produce empty strings, never errors. The only
// hard error is invalid id_token JWT structure (3 dot-separated segments
// expected); even then we just leave the email/account fields empty.
func ParseAuthJSON(blob map[string]any) AuthJSONInfo {
	info := AuthJSONInfo{}
	if mode, ok := blob["auth_mode"].(string); ok {
		info.AuthMode = mode
	}
	if lr, ok := blob["last_refresh"].(string); ok {
		if t, err := time.Parse(time.RFC3339, lr); err == nil {
			info.LastRefresh = t
		}
	}

	tokens, _ := blob["tokens"].(map[string]any)
	if tokens == nil {
		return info
	}
	if accID, ok := tokens["account_id"].(string); ok {
		info.AccountID = accID
	}

	idToken, _ := tokens["id_token"].(string)
	if idToken == "" {
		return info
	}
	claims, err := decodeJWTPayload(idToken)
	if err != nil {
		return info
	}
	if email, ok := claims["email"].(string); ok {
		info.Email = email
	}
	// OpenAI puts plan info under chatgpt_account_id / chatgpt_plan_type
	// or under nested objects — try a couple of common shapes.
	if pt, ok := claims["chatgpt_plan_type"].(string); ok {
		info.PlanType = pt
	} else if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		if pt, ok := auth["chatgpt_plan_type"].(string); ok {
			info.PlanType = pt
		}
	}
	if info.AccountID == "" {
		if accID, ok := claims["chatgpt_account_id"].(string); ok {
			info.AccountID = accID
		}
	}
	return info
}

// decodeJWTPayload parses the middle (payload) segment of a JWT and
// returns it as a generic map. Signature is NOT verified — we only read
// the email claim for display purposes.
func decodeJWTPayload(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("expected 3-segment JWT, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some tokens use standard base64 with padding; try that too.
		payload, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("decode JWT payload: %w", err)
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parse JWT claims: %w", err)
	}
	return claims, nil
}
