package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/lisiting01/aitoll-cli/internal/api"
	"github.com/lisiting01/aitoll-cli/internal/auth"
	"github.com/lisiting01/aitoll-cli/internal/config"
	"github.com/spf13/cobra"
)

var passcardCmd = &cobra.Command{
	Use:   "passcard",
	Short: "Manage passcards and service line preferences",
}

var passcardListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your passcards",
	RunE:  runPasscardList,
}

var passcardRoutesCmd = &cobra.Command{
	Use:   "routes",
	Short: "Show available lines for each service on the current passcard",
	Args:  cobra.NoArgs,
	RunE:  runPasscardRoutes,
}

var passcardSwitchCmd = &cobra.Command{
	Use:   "switch <service-name> <line-name>",
	Short: "Switch the active line for a service (use 'auto' as line-name to reset to automatic)",
	Args:  cobra.ExactArgs(2),
	RunE:  runPasscardSwitch,
}

func init() {
	rootCmd.AddCommand(passcardCmd)
	passcardCmd.AddCommand(passcardListCmd, passcardRoutesCmd, passcardSwitchCmd)
}

// loadCredsAndClient loads stored JWT credentials and returns an API client.
func loadCredsAndClient(cmd *cobra.Command) (*auth.StoredCredentials, *api.Client, error) {
	store := auth.NewTokenStore()
	creds, err := store.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read credentials: %w", err)
	}
	if creds == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "Not logged in. Run 'aitoll login' to authenticate.")
		return nil, nil, nil
	}
	return creds, api.NewClient(creds.BaseURL), nil
}

// claudeSettings is the minimal structure of ~/.claude/settings.json we care about.
type claudeSettings struct {
	Env map[string]string `json:"env"`
}

// readPasscardAPIKey reads the active passcard API key from ~/.claude/settings.json.
func readPasscardAPIKey() (string, error) {
	settingsPath, err := config.ClaudeSettingsPath()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("~/.claude/settings.json not found — no active API key configured")
		}
		return "", fmt.Errorf("failed to read ~/.claude/settings.json: %w", err)
	}

	var settings claudeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return "", fmt.Errorf("failed to parse ~/.claude/settings.json: %w", err)
	}

	apiKey := settings.Env["ANTHROPIC_AUTH_TOKEN"]
	if apiKey == "" {
		return "", fmt.Errorf("ANTHROPIC_AUTH_TOKEN not set in ~/.claude/settings.json")
	}
	if !strings.HasPrefix(apiKey, "pass_") {
		return "", fmt.Errorf("ANTHROPIC_AUTH_TOKEN in ~/.claude/settings.json is not a passcard key (expected pass_...)")
	}

	return apiKey, nil
}

// resolvePasscardID reads the active API key and resolves it to a passcard ID via the platform API.
func resolvePasscardID(client *api.Client) (string, string, error) {
	apiKey, err := readPasscardAPIKey()
	if err != nil {
		return "", "", err
	}

	pass, err := client.GetCurrentPasscard(apiKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve passcard: %w", err)
	}

	return pass.ID, pass.Name, nil
}

func runPasscardList(cmd *cobra.Command, args []string) error {
	creds, client, err := loadCredsAndClient(cmd)
	if err != nil {
		return err
	}
	if creds == nil {
		return nil
	}

	passes, err := client.ListPasscards(creds.Token)
	if err != nil {
		return fmt.Errorf("failed to list passcards: %w", err)
	}

	if len(passes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No passcards found.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%-24s  %-24s  %s\n", "ID", "NAME", "STATUS")
	for _, p := range passes {
		status := "disabled"
		if p.Enabled {
			status = "enabled"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-24s  %-24s  %s\n", p.ID, truncate(p.Name, 24), status)
	}
	return nil
}

func runPasscardRoutes(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("failed to get passcard routes: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Passcard: %s\n\n", passcardName)

	if len(pref.Services) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No services found for this passcard.")
		return nil
	}

	for _, svc := range pref.Services {
		fmt.Fprintf(cmd.OutOrStdout(), "Service: %s\n\n", svc.ServiceName)
		if len(svc.Lines) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "  (no lines available)")
		} else {
			for _, line := range svc.Lines {
				current := ""
				if line.ID == svc.SelectedLineID {
					current = "  [current]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %-24s  %s%s\n",
					line.Name, truncate(line.Description, 40), current)
			}
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}
	return nil
}

func runPasscardSwitch(cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	lineArg := args[1]

	creds, client, err := loadCredsAndClient(cmd)
	if err != nil {
		return err
	}
	if creds == nil {
		return nil
	}

	passcardID, _, err := resolvePasscardID(client)
	if err != nil {
		return err
	}

	pref, err := client.GetPasscardPreference(creds.Token, passcardID)
	if err != nil {
		return fmt.Errorf("failed to get passcard routes: %w", err)
	}

	// Find service by name (case-insensitive)
	var matchedSvc *api.ServicePreference
	for i := range pref.Services {
		if strings.EqualFold(pref.Services[i].ServiceName, serviceName) {
			matchedSvc = &pref.Services[i]
			break
		}
	}
	if matchedSvc == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Service %q not found. Available services:\n", serviceName)
		for _, svc := range pref.Services {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", svc.ServiceName)
		}
		return nil
	}

	// Handle "auto" — reset to automatic selection
	if strings.EqualFold(lineArg, "auto") {
		if err := client.SwitchPasscardLine(creds.Token, passcardID, matchedSvc.ServiceID, nil); err != nil {
			return fmt.Errorf("failed to switch line: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Reset %q to automatic selection.\n", matchedSvc.ServiceName)
		return nil
	}

	// Find line by name (case-insensitive)
	var matchedLineID string
	var matchedLineName string
	for _, line := range matchedSvc.Lines {
		if strings.EqualFold(line.Name, lineArg) {
			matchedLineID = line.ID
			matchedLineName = line.Name
			break
		}
	}
	if matchedLineID == "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Line %q not found for %q. Available lines:\n", lineArg, matchedSvc.ServiceName)
		for _, line := range matchedSvc.Lines {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", line.Name)
		}
		return nil
	}

	if err := client.SwitchPasscardLine(creds.Token, passcardID, matchedSvc.ServiceID, &matchedLineID); err != nil {
		return fmt.Errorf("failed to switch line: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Switched %q to %q.\n", matchedSvc.ServiceName, matchedLineName)
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "."
}
