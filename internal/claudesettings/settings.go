// Package claudesettings reads and writes the Claude Code `.claude/settings*.json`
// files in the current working directory. It is intentionally narrow: only
// `env.ANTHROPIC_CUSTOM_HEADERS` is manipulated. Other keys are preserved.
package claudesettings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Scope selects which settings file to operate on inside the current
// working directory.
type Scope string

const (
	// ScopeLocal targets `.claude/settings.local.json` (per-developer, not committed).
	ScopeLocal Scope = "local"
	// ScopeProject targets `.claude/settings.json` (team-shared, committed).
	ScopeProject Scope = "project"
)

// CustomHeadersKey is the env var name Claude Code reads to inject custom
// headers into every API request it makes.
const CustomHeadersKey = "ANTHROPIC_CUSTOM_HEADERS"

// PathForScope returns the settings file path for the given scope, relative
// to the supplied working directory.
func PathForScope(cwd string, scope Scope) (string, error) {
	switch scope {
	case ScopeLocal:
		return filepath.Join(cwd, ".claude", "settings.local.json"), nil
	case ScopeProject:
		return filepath.Join(cwd, ".claude", "settings.json"), nil
	default:
		return "", fmt.Errorf("unknown scope %q (expected 'local' or 'project')", scope)
	}
}

// SetCustomHeader upserts a single "Name: Value" entry into
// ANTHROPIC_CUSTOM_HEADERS for the given scope, preserving any other headers
// the user already configured.
func SetCustomHeader(cwd string, scope Scope, headerName, headerValue string) error {
	path, err := PathForScope(cwd, scope)
	if err != nil {
		return err
	}
	settings, err := readSettings(path)
	if err != nil {
		return err
	}

	headers := parseCustomHeaders(settings.envValue(CustomHeadersKey))
	headers[normalizeHeaderName(headerName)] = headerValue
	settings.setEnv(CustomHeadersKey, serializeCustomHeaders(headers))

	return writeSettings(path, settings)
}

// UnsetCustomHeader removes a single header from ANTHROPIC_CUSTOM_HEADERS.
// Returns the previous value (or "") and whether anything was removed.
func UnsetCustomHeader(cwd string, scope Scope, headerName string) (string, bool, error) {
	path, err := PathForScope(cwd, scope)
	if err != nil {
		return "", false, err
	}
	settings, err := readSettings(path)
	if err != nil {
		return "", false, err
	}
	if settings.Env == nil {
		return "", false, nil
	}

	headers := parseCustomHeaders(settings.envValue(CustomHeadersKey))
	key := normalizeHeaderName(headerName)
	prev, existed := headers[key]
	if !existed {
		return "", false, nil
	}
	delete(headers, key)

	if len(headers) == 0 {
		settings.deleteEnv(CustomHeadersKey)
	} else {
		settings.setEnv(CustomHeadersKey, serializeCustomHeaders(headers))
	}

	if err := writeSettings(path, settings); err != nil {
		return "", false, err
	}
	return prev, true, nil
}

// ListCustomHeaders returns the parsed map of headers from
// ANTHROPIC_CUSTOM_HEADERS for the given scope. Missing file returns an empty
// map without error.
func ListCustomHeaders(cwd string, scope Scope) (map[string]string, error) {
	path, err := PathForScope(cwd, scope)
	if err != nil {
		return nil, err
	}
	settings, err := readSettings(path)
	if err != nil {
		return nil, err
	}
	return parseCustomHeaders(settings.envValue(CustomHeadersKey)), nil
}

// RemoveHeadersByPrefix deletes every header whose name starts with the given
// prefix (case-insensitive). Returns the removed entries. Used by
// `aitoll line reset` to wipe every x-aitoll-line-* pin in one shot.
func RemoveHeadersByPrefix(cwd string, scope Scope, prefix string) (map[string]string, error) {
	path, err := PathForScope(cwd, scope)
	if err != nil {
		return nil, err
	}
	settings, err := readSettings(path)
	if err != nil {
		return nil, err
	}

	headers := parseCustomHeaders(settings.envValue(CustomHeadersKey))
	prefixLower := strings.ToLower(prefix)
	removed := map[string]string{}
	for name, value := range headers {
		if strings.HasPrefix(strings.ToLower(name), prefixLower) {
			removed[name] = value
			delete(headers, name)
		}
	}
	if len(removed) == 0 {
		return removed, nil
	}

	if len(headers) == 0 {
		settings.deleteEnv(CustomHeadersKey)
	} else {
		settings.setEnv(CustomHeadersKey, serializeCustomHeaders(headers))
	}
	if err := writeSettings(path, settings); err != nil {
		return nil, err
	}
	return removed, nil
}

// --- internal ---

// settingsFile mirrors the shape of `.claude/settings*.json` only as far as
// we need to. Unknown top-level keys are preserved verbatim through Extras.
type settingsFile struct {
	Env    map[string]string      `json:"env,omitempty"`
	Extras map[string]json.RawMessage
}

func (s *settingsFile) envValue(key string) string {
	if s.Env == nil {
		return ""
	}
	return s.Env[key]
}

func (s *settingsFile) setEnv(key, value string) {
	if s.Env == nil {
		s.Env = map[string]string{}
	}
	s.Env[key] = value
}

func (s *settingsFile) deleteEnv(key string) {
	if s.Env == nil {
		return
	}
	delete(s.Env, key)
}

func readSettings(path string) (*settingsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &settingsFile{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	settings := &settingsFile{Extras: map[string]json.RawMessage{}}
	for k, v := range raw {
		if k == "env" {
			if err := json.Unmarshal(v, &settings.Env); err != nil {
				return nil, fmt.Errorf("parse env in %s: %w", path, err)
			}
			continue
		}
		settings.Extras[k] = v
	}
	return settings, nil
}

func writeSettings(path string, settings *settingsFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	out := map[string]json.RawMessage{}
	for k, v := range settings.Extras {
		out[k] = v
	}
	if len(settings.Env) > 0 {
		envBytes, err := json.Marshal(settings.Env)
		if err != nil {
			return err
		}
		out["env"] = envBytes
	}

	// Stable key order so the file diffs cleanly.
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf strings.Builder
	buf.WriteString("{\n")
	for i, k := range keys {
		keyBytes, _ := json.Marshal(k)
		buf.WriteString("  ")
		buf.Write(keyBytes)
		buf.WriteString(": ")
		var pretty any
		if err := json.Unmarshal(out[k], &pretty); err == nil {
			indented, err := json.MarshalIndent(pretty, "  ", "  ")
			if err != nil {
				return err
			}
			buf.Write(indented)
		} else {
			buf.Write(out[k])
		}
		if i < len(keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")

	if len(keys) == 0 {
		// Empty settings: just remove the file rather than leave a stub.
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	return os.WriteFile(path, []byte(buf.String()), 0o644)
}

// parseCustomHeaders splits the multi-line ANTHROPIC_CUSTOM_HEADERS value
// (`Name: Value\nName2: Value2`) into a map. Header names are normalized to
// canonical lowercase to support stable upsert/delete by name.
func parseCustomHeaders(raw string) map[string]string {
	headers := map[string]string{}
	if raw == "" {
		return headers
	}
	// Claude Code accepts newline-separated entries. Be lenient about CR.
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		if name == "" {
			continue
		}
		headers[normalizeHeaderName(name)] = value
	}
	return headers
}

// serializeCustomHeaders renders the headers back into the multi-line
// representation Claude Code expects. Output is sorted for stable diffs.
func serializeCustomHeaders(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, name+": "+headers[name])
	}
	return strings.Join(lines, "\n")
}

func normalizeHeaderName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
