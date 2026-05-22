package claudesettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSetCustomHeader_PreservesUnrelatedKeys verifies that writing a single
// header does not clobber other env keys or other top-level settings.
func TestSetCustomHeader_PreservesUnrelatedKeys(t *testing.T) {
	cwd := t.TempDir()
	settingsDir := filepath.Join(cwd, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(settingsDir, "settings.local.json")
	initial := `{
  "permissions": { "allow": ["Bash(go test:*)"] },
  "env": {
    "OTHER_VAR": "keep-me",
    "ANTHROPIC_CUSTOM_HEADERS": "existing-header: keep\nx-other: alsokeep"
  }
}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetCustomHeader(cwd, ScopeLocal, "x-aitoll-line-svc1", "lineA"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Permissions map[string]any    `json:"permissions"`
		Env         map[string]string `json:"env"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, data)
	}
	if parsed.Permissions == nil {
		t.Errorf("permissions block was dropped:\n%s", data)
	}
	if parsed.Env["OTHER_VAR"] != "keep-me" {
		t.Errorf("OTHER_VAR was dropped:\n%s", data)
	}

	headers := parseCustomHeaders(parsed.Env["ANTHROPIC_CUSTOM_HEADERS"])
	if headers["existing-header"] != "keep" {
		t.Errorf("existing-header was dropped: %+v", headers)
	}
	if headers["x-other"] != "alsokeep" {
		t.Errorf("x-other was dropped: %+v", headers)
	}
	if headers["x-aitoll-line-svc1"] != "lineA" {
		t.Errorf("new pin not written: %+v", headers)
	}
}

// TestUnsetCustomHeader_RemovesEnvKeyWhenEmpty ensures that removing the last
// header leaves the env block free of an empty ANTHROPIC_CUSTOM_HEADERS string.
func TestUnsetCustomHeader_RemovesEnvKeyWhenEmpty(t *testing.T) {
	cwd := t.TempDir()
	if err := SetCustomHeader(cwd, ScopeLocal, "x-aitoll-line-only", "L"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UnsetCustomHeader(cwd, ScopeLocal, "x-aitoll-line-only"); err != nil {
		t.Fatal(err)
	}

	path, _ := PathForScope(cwd, ScopeLocal)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected settings file to be removed when empty, got err=%v", err)
	}
}

// TestRemoveHeadersByPrefix_ResetsAllPinsButKeepsUnrelated covers the
// `aitoll line reset` happy path.
func TestRemoveHeadersByPrefix_ResetsAllPinsButKeepsUnrelated(t *testing.T) {
	cwd := t.TempDir()
	if err := SetCustomHeader(cwd, ScopeLocal, "x-aitoll-line-a", "lineA"); err != nil {
		t.Fatal(err)
	}
	if err := SetCustomHeader(cwd, ScopeLocal, "x-aitoll-line-b", "lineB"); err != nil {
		t.Fatal(err)
	}
	if err := SetCustomHeader(cwd, ScopeLocal, "x-not-aitoll", "leaveMeAlone"); err != nil {
		t.Fatal(err)
	}

	removed, err := RemoveHeadersByPrefix(cwd, ScopeLocal, "x-aitoll-line-")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Errorf("expected 2 removed headers, got %d: %+v", len(removed), removed)
	}

	left, err := ListCustomHeaders(cwd, ScopeLocal)
	if err != nil {
		t.Fatal(err)
	}
	if left["x-not-aitoll"] != "leaveMeAlone" {
		t.Errorf("unrelated header lost: %+v", left)
	}
	if _, ok := left["x-aitoll-line-a"]; ok {
		t.Errorf("pin still present after reset: %+v", left)
	}
}
