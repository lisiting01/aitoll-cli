package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/codexremote"
	"github.com/spf13/cobra"
)

var (
	startProxyOverride string
	startFreshLogs     bool
	startDebug         bool
	logsTailN          int
	logsFollow         bool
)

var codexRemoteCmd = &cobra.Command{
	Use:   "codex-remote",
	Short: "Run OpenAI Codex mobile remote control daemon on this machine",
	Long: `Manage the codex remote-control daemon — a thin wrapper around
'codex remote-control --enable remote_control' from the @openai/codex npm
package. The daemon establishes a persistent wss connection to ChatGPT so the
ChatGPT mobile app can dispatch Codex tasks to this computer.

This is unrelated to the AI-Toll platform itself; it's a convenience wrapper.`,
}

var codexRemoteStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the codex-remote daemon in the background",
	RunE:  runCodexRemoteStart,
}

var codexRemoteStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the codex-remote daemon",
	RunE:  runCodexRemoteStop,
}

var codexRemoteStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show codex-remote daemon status",
	RunE:  runCodexRemoteStatus,
}

var codexRemoteLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show codex-remote daemon logs",
	RunE:  runCodexRemoteLogs,
}

var codexRemoteDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose codex-remote prerequisites",
	RunE:  runCodexRemoteDoctor,
}

var codexRemoteConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage codex-remote configuration",
}

var codexRemoteConfigGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a config value (omit key to show all)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCodexRemoteConfigGet,
}

var codexRemoteConfigSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value (key: proxy)",
	Args:  cobra.ExactArgs(2),
	RunE:  runCodexRemoteConfigSet,
}

func init() {
	rootCmd.AddCommand(codexRemoteCmd)
	codexRemoteCmd.AddCommand(
		codexRemoteStartCmd,
		codexRemoteStopCmd,
		codexRemoteStatusCmd,
		codexRemoteLogsCmd,
		codexRemoteDoctorCmd,
		codexRemoteConfigCmd,
	)
	codexRemoteConfigCmd.AddCommand(codexRemoteConfigGetCmd, codexRemoteConfigSetCmd)

	codexRemoteStartCmd.Flags().StringVar(&startProxyOverride, "proxy", "", "Proxy URL (overrides config for this start only)")
	codexRemoteStartCmd.Flags().BoolVar(&startFreshLogs, "fresh-logs", false, "Truncate the log file before starting")
	codexRemoteStartCmd.Flags().BoolVar(&startDebug, "debug", false, "Enable verbose RUST_LOG output (codex=debug)")
	codexRemoteLogsCmd.Flags().IntVarP(&logsTailN, "tail", "n", 200, "Number of trailing lines to show")
	codexRemoteLogsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output as it grows")
}

func runCodexRemoteStart(cmd *cobra.Command, args []string) error {
	state, err := codexremote.Start(codexremote.StartOptions{
		Proxy:     startProxyOverride,
		FreshLogs: startFreshLogs,
		Debug:     startDebug,
	})
	if err != nil {
		return err
	}
	logPath, _ := codexremote.LogPath()
	fmt.Fprintf(cmd.OutOrStdout(), "Started codex-remote (PID %d, proxy %s)\n", state.PID, state.Proxy)
	fmt.Fprintf(cmd.OutOrStdout(), "Logs: %s\n", logPath)
	fmt.Fprintln(cmd.OutOrStdout(), "Tail logs to confirm wss connection: aitoll codex-remote logs --follow")
	return nil
}

func runCodexRemoteStop(cmd *cobra.Command, args []string) error {
	pid, err := codexremote.Stop()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(cmd.OutOrStdout(), "codex-remote is not running")
			return nil
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Stopped codex-remote (PID %d)\n", pid)
	return nil
}

func runCodexRemoteStatus(cmd *cobra.Command, args []string) error {
	state, err := codexremote.LoadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(cmd.OutOrStdout(), "codex-remote: not running")
			return nil
		}
		return err
	}
	if !codexremote.IsAlive(state.PID) {
		fmt.Fprintf(cmd.OutOrStdout(), "codex-remote: not running (stale PID %d in state file)\n", state.PID)
		return nil
	}
	uptime := time.Since(state.StartedAt).Round(time.Second)
	fmt.Fprintf(cmd.OutOrStdout(), "codex-remote: running\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  PID:        %d\n", state.PID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Started:    %s\n", state.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(cmd.OutOrStdout(), "  Uptime:     %s\n", uptime)
	fmt.Fprintf(cmd.OutOrStdout(), "  Proxy:      %s\n", state.Proxy)
	logPath, _ := codexremote.LogPath()
	fmt.Fprintf(cmd.OutOrStdout(), "  Log:        %s\n", logPath)
	return nil
}

func runCodexRemoteLogs(cmd *cobra.Command, args []string) error {
	if !logsFollow {
		return codexremote.TailLast(cmd.OutOrStdout(), logsTailN)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return codexremote.Follow(ctx, cmd.OutOrStdout(), logsTailN)
}

func runCodexRemoteDoctor(cmd *cobra.Command, args []string) error {
	results := codexremote.Doctor()
	hasError := false
	for _, r := range results {
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n", padLevel(r.Level.String()), r.Message)
		if r.Level == codexremote.LevelError {
			hasError = true
		}
	}
	if hasError {
		return errors.New("doctor reported one or more errors")
	}
	return nil
}

func runCodexRemoteConfigGet(cmd *cobra.Command, args []string) error {
	cfg, err := codexremote.LoadConfig()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "proxy = %s\n", cfg.Proxy)
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "proxy":
		fmt.Fprintf(cmd.OutOrStdout(), "proxy = %s\n", cfg.Proxy)
	default:
		return fmt.Errorf("unknown config key %q (available: proxy)", args[0])
	}
	return nil
}

func runCodexRemoteConfigSet(cmd *cobra.Command, args []string) error {
	key := strings.ToLower(args[0])
	val := args[1]
	cfg, err := codexremote.LoadConfig()
	if err != nil {
		return err
	}
	switch key {
	case "proxy":
		cfg.Proxy = val
	default:
		return fmt.Errorf("unknown config key %q (available: proxy)", key)
	}
	if err := codexremote.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %s\n", key, val)
	return nil
}

func padLevel(s string) string {
	const width = 5
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
