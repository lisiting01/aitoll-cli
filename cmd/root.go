package cmd

import (
	"fmt"
	"os"

	"github.com/lisiting01/aitoll-cli/internal/config"
	"github.com/lisiting01/aitoll-cli/internal/updater"
	"github.com/spf13/cobra"
)

var (
	baseURL    string
	appVersion = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "aitoll",
	Short: "AI-Toll CLI",
	Long:  "Command-line interface for the AI-Toll platform (aitoll.net)",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "aitoll %s\n", appVersion)
	},
}

// SetVersion sets the version string (called from main.go with build-time value).
func SetVersion(v string) {
	appVersion = v
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&baseURL, "base-url", "https://aitoll.net", "Platform base URL")
	rootCmd.AddCommand(versionCmd)
	rootCmd.PersistentPreRun = backgroundUpdateCheck
}

// backgroundUpdateCheck runs a non-blocking version check before any command.
// It prints a one-line notice if a newer version is available, then continues.
func backgroundUpdateCheck(cmd *cobra.Command, args []string) {
	// Skip for the update command itself to avoid redundant output.
	if cmd.Name() == "update" {
		return
	}

	cfg, err := config.LoadAppConfig()
	if err != nil || !cfg.AutoUpdate {
		return
	}

	done := make(chan *updater.ReleaseInfo, 1)
	go func() {
		info, err := updater.CheckLatest()
		if err != nil {
			done <- nil
			return
		}
		done <- info
	}()

	// We don't block — the goroutine result is intentionally discarded here.
	// Instead we use a select with no default so the check is truly fire-and-forget.
	// To surface the notice we'd need to wait, which would slow every command.
	// So we do a brief non-blocking peek: if the result is already ready, show it.
	select {
	case info := <-done:
		if info != nil && updater.IsNewer(appVersion, info.TagName) {
			fmt.Fprintf(os.Stderr, "\nA new version of aitoll is available: %s (run 'aitoll update' to upgrade)\n\n", info.TagName)
		}
	default:
		// Not ready yet — skip silently.
	}
}
