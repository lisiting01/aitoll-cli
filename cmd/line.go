package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lisiting01/aitoll-cli/internal/api"
	"github.com/lisiting01/aitoll-cli/internal/claudesettings"
	"github.com/spf13/cobra"
)

// headerPrefix is the namespace for all aitoll-controlled custom headers.
// `aitoll line reset` and gateway forwarding both key off of this prefix.
const headerPrefix = "x-aitoll-line-"

// lineModeHeader carries the strict/loose toggle.
const lineModeHeader = "x-aitoll-line-mode"

var (
	lineScope      string
	lineLooseMode  bool
	lineResetScope string
)

var lineCmd = &cobra.Command{
	Use:   "line",
	Short: "Pin service lines per working directory via Claude Code custom headers",
}

var lineSetCmd = &cobra.Command{
	Use:   "set <service-name> <line-name>",
	Short: "Pin a service to a specific line for the current working directory",
	Args:  cobra.ExactArgs(2),
	RunE:  runLineSet,
}

var lineUnsetCmd = &cobra.Command{
	Use:   "unset <service-name>",
	Short: "Remove the line pin for a service in the current working directory",
	Args:  cobra.ExactArgs(1),
	RunE:  runLineUnset,
}

var lineResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Remove all aitoll line pins in the current working directory",
	Args:  cobra.NoArgs,
	RunE:  runLineReset,
}

var lineShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show line pins effective in the current working directory",
	Args:  cobra.NoArgs,
	RunE:  runLineShow,
}

func init() {
	rootCmd.AddCommand(lineCmd)
	lineCmd.AddCommand(lineSetCmd, lineUnsetCmd, lineResetCmd, lineShowCmd)

	lineSetCmd.Flags().StringVar(&lineScope, "scope", "local",
		"Which settings file to write: 'local' (.claude/settings.local.json) or 'project' (.claude/settings.json)")
	lineSetCmd.Flags().BoolVar(&lineLooseMode, "loose", false,
		"If the pinned line becomes unavailable, fall back silently instead of failing the request")

	lineUnsetCmd.Flags().StringVar(&lineScope, "scope", "local",
		"Which settings file to modify: 'local' or 'project'")

	lineResetCmd.Flags().StringVar(&lineResetScope, "scope", "local",
		"Which settings file to clear: 'local', 'project', or 'all'")
}

// resolveScope normalizes the --scope flag value.
func resolveScope(raw string) (claudesettings.Scope, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "local":
		return claudesettings.ScopeLocal, nil
	case "project":
		return claudesettings.ScopeProject, nil
	default:
		return "", fmt.Errorf("unknown --scope %q (expected 'local' or 'project')", raw)
	}
}

func runLineSet(cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	lineName := args[1]

	scope, err := resolveScope(lineScope)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to read working directory: %w", err)
	}

	creds, client, err := loadCredsAndClient(cmd)
	if err != nil {
		return err
	}
	if creds == nil {
		return nil
	}

	passcardID, passcardName, err := resolvePasscardID(client)
	if err != nil {
		return err
	}

	pref, err := client.GetPasscardPreference(creds.Token, passcardID)
	if err != nil {
		return fmt.Errorf("failed to fetch passcard routes: %w", err)
	}

	svc := findServiceByName(pref.Services, serviceName)
	if svc == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Service %q not found on passcard %q. Available services:\n", serviceName, passcardName)
		for _, s := range pref.Services {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", s.ServiceName)
		}
		return nil
	}

	var matchedLine *api.Line
	for i := range svc.Lines {
		if strings.EqualFold(svc.Lines[i].Name, lineName) {
			matchedLine = &svc.Lines[i]
			break
		}
	}
	if matchedLine == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Line %q not found for service %q. Available lines:\n", lineName, svc.ServiceName)
		for _, l := range svc.Lines {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", l.Name)
		}
		return nil
	}

	headerKey := headerPrefix + svc.ServiceID
	if err := claudesettings.SetCustomHeader(cwd, scope, headerKey, matchedLine.ID); err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	// Manage the mode header at the same scope, so that loose/strict travel
	// with the pin and can be toggled by re-running `set` with/without --loose.
	if lineLooseMode {
		if err := claudesettings.SetCustomHeader(cwd, scope, lineModeHeader, "loose"); err != nil {
			return fmt.Errorf("failed to write mode header: %w", err)
		}
	} else {
		// strict is the default; clear any previous loose marker to keep state explicit.
		if _, _, err := claudesettings.UnsetCustomHeader(cwd, scope, lineModeHeader); err != nil {
			return fmt.Errorf("failed to clear mode header: %w", err)
		}
	}

	settingsPath, _ := claudesettings.PathForScope(cwd, scope)
	mode := "strict"
	if lineLooseMode {
		mode = "loose"
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"Pinned %s → %s in %s (mode=%s).\nRestart Claude Code in this directory for the change to take effect.\n",
		svc.ServiceName, matchedLine.Name, settingsPath, mode)
	return nil
}

func runLineUnset(cmd *cobra.Command, args []string) error {
	serviceName := args[0]

	scope, err := resolveScope(lineScope)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to read working directory: %w", err)
	}

	creds, client, err := loadCredsAndClient(cmd)
	if err != nil {
		return err
	}
	if creds == nil {
		return nil
	}

	// We still need to resolve serviceName → serviceID to find the right header key.
	passcardID, _, err := resolvePasscardID(client)
	if err != nil {
		return err
	}
	pref, err := client.GetPasscardPreference(creds.Token, passcardID)
	if err != nil {
		return fmt.Errorf("failed to fetch passcard routes: %w", err)
	}
	svc := findServiceByName(pref.Services, serviceName)
	if svc == nil {
		return fmt.Errorf("service %q not found on the current passcard", serviceName)
	}

	headerKey := headerPrefix + svc.ServiceID
	_, removed, err := claudesettings.UnsetCustomHeader(cwd, scope, headerKey)
	if err != nil {
		return err
	}

	settingsPath, _ := claudesettings.PathForScope(cwd, scope)
	if !removed {
		fmt.Fprintf(cmd.OutOrStdout(), "No pin found for %s in %s.\n", svc.ServiceName, settingsPath)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(),
		"Removed pin for %s from %s.\nRestart Claude Code in this directory for the change to take effect.\n",
		svc.ServiceName, settingsPath)
	return nil
}

func runLineReset(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to read working directory: %w", err)
	}

	scopes, err := resolveResetScopes(lineResetScope)
	if err != nil {
		return err
	}

	totalRemoved := 0
	for _, scope := range scopes {
		settingsPath, _ := claudesettings.PathForScope(cwd, scope)

		// Clear all line pins and the mode header in one pass.
		removed, err := claudesettings.RemoveHeadersByPrefix(cwd, scope, headerPrefix)
		if err != nil {
			return fmt.Errorf("failed to reset %s: %w", settingsPath, err)
		}
		// Mode header sits adjacent to (but not under) headerPrefix, clean it too.
		if _, modeRemoved, err := claudesettings.UnsetCustomHeader(cwd, scope, lineModeHeader); err != nil {
			return fmt.Errorf("failed to clear mode header in %s: %w", settingsPath, err)
		} else if modeRemoved {
			removed[lineModeHeader] = "(loose)"
		}

		if len(removed) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "No aitoll pins in %s.\n", settingsPath)
			continue
		}

		// Best-effort: translate each x-aitoll-line-<serviceId> back to a service
		// name for the user. Failures here are non-fatal — we still report by ID.
		serviceNames := lookupServiceNamesForRemoved(cmd, removed)

		fmt.Fprintf(cmd.OutOrStdout(), "Cleared %d entry(s) from %s:\n", len(removed), settingsPath)
		keys := make([]string, 0, len(removed))
		for k := range removed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			label := key
			if strings.HasPrefix(key, headerPrefix) {
				svcID := strings.TrimPrefix(key, headerPrefix)
				if name, ok := serviceNames[svcID]; ok {
					label = name
				} else {
					label = "service " + svcID
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s (was %s)\n", label, removed[key])
		}
		totalRemoved += len(removed)
	}

	if totalRemoved > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Restart Claude Code in this directory for the changes to take effect.")
	}
	return nil
}

func runLineShow(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to read working directory: %w", err)
	}

	scopes := []claudesettings.Scope{claudesettings.ScopeLocal, claudesettings.ScopeProject}

	// Resolve service & line names for friendlier output. Best-effort: if we
	// can't reach the API or the user isn't logged in, we still show the raw
	// header data.
	serviceLabels := map[string]string{} // serviceId → "ServiceName"
	lineLabels := map[string]map[string]string{} // serviceId → lineId → "LineName"
	if creds, client, err := loadCredsAndClient(cmd); err == nil && creds != nil {
		if passcardID, _, err := resolvePasscardID(client); err == nil {
			if pref, err := client.GetPasscardPreference(creds.Token, passcardID); err == nil {
				for _, svc := range pref.Services {
					serviceLabels[svc.ServiceID] = svc.ServiceName
					lineLabels[svc.ServiceID] = map[string]string{}
					for _, l := range svc.Lines {
						lineLabels[svc.ServiceID][l.ID] = l.Name
					}
				}
			}
		}
	}

	anyShown := false
	for _, scope := range scopes {
		headers, err := claudesettings.ListCustomHeaders(cwd, scope)
		if err != nil {
			return err
		}
		settingsPath, _ := claudesettings.PathForScope(cwd, scope)

		pins := map[string]string{}
		mode := "strict"
		for name, value := range headers {
			if strings.HasPrefix(name, headerPrefix) {
				pins[strings.TrimPrefix(name, headerPrefix)] = value
			}
			if name == lineModeHeader && strings.EqualFold(value, "loose") {
				mode = "loose"
			}
		}
		if len(pins) == 0 {
			continue
		}

		fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s (mode=%s)\n", scope, settingsPath, mode)
		ids := make([]string, 0, len(pins))
		for id := range pins {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, svcID := range ids {
			svcLabel := serviceLabels[svcID]
			if svcLabel == "" {
				svcLabel = "service " + svcID
			}
			lineID := pins[svcID]
			lineLabel := ""
			if m, ok := lineLabels[svcID]; ok {
				lineLabel = m[lineID]
			}
			if lineLabel == "" {
				lineLabel = "line " + lineID
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s → %s\n", svcLabel, lineLabel)
		}
		anyShown = true
	}

	if !anyShown {
		fmt.Fprintln(cmd.OutOrStdout(), "No aitoll line pins in this working directory.")
	}
	return nil
}

// --- helpers ---

func findServiceByName(services []api.ServicePreference, name string) *api.ServicePreference {
	for i := range services {
		if strings.EqualFold(services[i].ServiceName, name) {
			return &services[i]
		}
	}
	return nil
}

func resolveResetScopes(raw string) ([]claudesettings.Scope, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "local":
		return []claudesettings.Scope{claudesettings.ScopeLocal}, nil
	case "project":
		return []claudesettings.Scope{claudesettings.ScopeProject}, nil
	case "all":
		return []claudesettings.Scope{claudesettings.ScopeLocal, claudesettings.ScopeProject}, nil
	default:
		return nil, fmt.Errorf("unknown --scope %q (expected 'local', 'project', or 'all')", raw)
	}
}

// lookupServiceNamesForRemoved tries to translate serviceIds in `removed` back
// to friendly names via the passcard preference endpoint. Failures (no creds,
// no network) are swallowed — the caller falls back to the raw ID.
func lookupServiceNamesForRemoved(cmd *cobra.Command, removed map[string]string) map[string]string {
	names := map[string]string{}
	creds, client, err := loadCredsAndClient(cmd)
	if err != nil || creds == nil {
		return names
	}
	passcardID, _, err := resolvePasscardID(client)
	if err != nil {
		return names
	}
	pref, err := client.GetPasscardPreference(creds.Token, passcardID)
	if err != nil {
		return names
	}
	for _, svc := range pref.Services {
		names[svc.ServiceID] = svc.ServiceName
	}
	return names
}
