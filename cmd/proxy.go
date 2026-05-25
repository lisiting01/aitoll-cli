package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/proxy"
	"github.com/spf13/cobra"
)

var (
	proxyStartFreshLogs bool
	proxyLogsTailN      int
	proxyLogsFollow     bool
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Local HTTP/SOCKS proxy backed by aitoll-curated nodes",
	Long: `Run an embedded sing-box mixed inbound on this machine, tunneling outbound
through a sing-box config the aitoll backend provisions for you. Other aitoll
subcommands (e.g. codex-remote) automatically use this proxy when no local
proxy software (Clash, V2RayN, etc.) is detected.

Run 'aitoll proxy doctor' if anything is off.`,
}

var proxyStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the aitoll proxy daemon in the background",
	RunE:  runProxyStart,
}

var proxyStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the aitoll proxy daemon",
	RunE:  runProxyStop,
}

var proxyStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show aitoll proxy daemon status",
	RunE:  runProxyStatus,
}

var proxyLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show aitoll proxy daemon logs",
	RunE:  runProxyLogs,
}

var proxyDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose aitoll proxy prerequisites",
	RunE:  runProxyDoctor,
}

var proxySyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull a fresh sing-box config from the aitoll backend",
	RunE:  runProxySync,
}

var proxyConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage aitoll proxy local configuration",
}

var proxyConfigGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a config value (omit key to show all)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runProxyConfigGet,
}

var proxyConfigSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value (key: listen_addr)",
	Args:  cobra.ExactArgs(2),
	RunE:  runProxyConfigSet,
}

// proxyDaemonCmd is hidden — invoked only by `proxy start` after fork.
var proxyDaemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "(internal) Run sing-box in this process; do not invoke directly",
	Hidden: true,
	RunE:   runProxyDaemon,
}

func init() {
	rootCmd.AddCommand(proxyCmd)
	proxyCmd.AddCommand(
		proxyStartCmd,
		proxyStopCmd,
		proxyStatusCmd,
		proxyLogsCmd,
		proxyDoctorCmd,
		proxySyncCmd,
		proxyConfigCmd,
		proxyDaemonCmd,
	)
	proxyConfigCmd.AddCommand(proxyConfigGetCmd, proxyConfigSetCmd)

	proxyStartCmd.Flags().BoolVar(&proxyStartFreshLogs, "fresh-logs", false, "Truncate the log file before starting")
	proxyLogsCmd.Flags().IntVarP(&proxyLogsTailN, "tail", "n", 200, "Number of trailing lines to show")
	proxyLogsCmd.Flags().BoolVarP(&proxyLogsFollow, "follow", "f", false, "Follow log output as it grows")
}

func runProxyStart(cmd *cobra.Command, args []string) error {
	state, err := proxy.Start(proxy.StartOptions{
		FreshLogs: proxyStartFreshLogs,
	})
	if err != nil && state == nil {
		return err
	}
	logPath, _ := proxy.LogPath()
	fmt.Fprintf(cmd.OutOrStdout(), "Started aitoll proxy (PID %d, listener %s)\n", state.PID, state.ListenAddr)
	fmt.Fprintf(cmd.OutOrStdout(), "Logs: %s\n", logPath)
	if err != nil {
		// non-fatal: daemon launched but listener wasn't ready in time
		fmt.Fprintf(cmd.OutOrStdout(), "Note: %s\n", err.Error())
	}
	return nil
}

func runProxyStop(cmd *cobra.Command, args []string) error {
	pid, err := proxy.Stop()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(cmd.OutOrStdout(), "aitoll proxy is not running")
			return nil
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Stopped aitoll proxy (PID %d)\n", pid)
	return nil
}

func runProxyStatus(cmd *cobra.Command, args []string) error {
	state, err := proxy.LoadState()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(cmd.OutOrStdout(), "aitoll proxy: not running")
			return nil
		}
		return err
	}
	if !proxy.IsAlive(state.PID) {
		fmt.Fprintf(cmd.OutOrStdout(), "aitoll proxy: not running (stale PID %d in state file)\n", state.PID)
		return nil
	}
	uptime := time.Since(state.StartedAt).Round(time.Second)
	fmt.Fprintf(cmd.OutOrStdout(), "aitoll proxy: running\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  PID:        %d\n", state.PID)
	fmt.Fprintf(cmd.OutOrStdout(), "  Started:    %s\n", state.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(cmd.OutOrStdout(), "  Uptime:     %s\n", uptime)
	fmt.Fprintf(cmd.OutOrStdout(), "  Listener:   %s\n", state.ListenAddr)
	fmt.Fprintf(cmd.OutOrStdout(), "  URL:        %s\n", proxy.ListenerURL(state.ListenAddr))
	if meta, _ := proxy.LoadSyncMeta(); meta != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  Synced:     %s (%s ago)\n",
			meta.SyncedAt.Format(time.RFC3339),
			time.Since(meta.SyncedAt).Round(time.Second))
	}
	logPath, _ := proxy.LogPath()
	fmt.Fprintf(cmd.OutOrStdout(), "  Log:        %s\n", logPath)
	return nil
}

func runProxyLogs(cmd *cobra.Command, args []string) error {
	if !proxyLogsFollow {
		return proxy.TailLast(cmd.OutOrStdout(), proxyLogsTailN)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return proxy.Follow(ctx, cmd.OutOrStdout(), proxyLogsTailN)
}

func runProxyDoctor(cmd *cobra.Command, args []string) error {
	results := proxy.Doctor()
	hasError := false
	for _, r := range results {
		fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n", padLevel(r.Level.String()), r.Message)
		if r.Level == proxy.LevelError {
			hasError = true
		}
	}
	if hasError {
		return errors.New("doctor reported one or more errors")
	}
	return nil
}

func runProxySync(cmd *cobra.Command, args []string) error {
	if err := proxy.Sync(); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Synced aitoll proxy config.")
	if state, err := proxy.LoadState(); err == nil && proxy.IsAlive(state.PID) {
		fmt.Fprintln(cmd.OutOrStdout(), "Note: daemon is running with the previous config.")
		fmt.Fprintln(cmd.OutOrStdout(), "Restart to pick up the new one: aitoll proxy stop && aitoll proxy start")
	}
	return nil
}

func runProxyConfigGet(cmd *cobra.Command, args []string) error {
	cfg, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "listen_addr = %s\n", cfg.ListenAddr)
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "listen_addr", "listen", "addr":
		fmt.Fprintf(cmd.OutOrStdout(), "listen_addr = %s\n", cfg.ListenAddr)
	default:
		return fmt.Errorf("unknown config key %q (available: listen_addr)", args[0])
	}
	return nil
}

func runProxyConfigSet(cmd *cobra.Command, args []string) error {
	key := strings.ToLower(args[0])
	val := args[1]
	cfg, err := proxy.LoadConfig()
	if err != nil {
		return err
	}
	switch key {
	case "listen_addr", "listen", "addr":
		cfg.ListenAddr = val
	default:
		return fmt.Errorf("unknown config key %q (available: listen_addr)", key)
	}
	if err := proxy.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Set listen_addr = %s\n", val)
	if state, err := proxy.LoadState(); err == nil && proxy.IsAlive(state.PID) {
		fmt.Fprintln(cmd.OutOrStdout(), "Note: daemon is running with the previous listen_addr.")
		fmt.Fprintln(cmd.OutOrStdout(), "Restart to pick up the change: aitoll proxy stop && aitoll proxy start")
	}
	return nil
}

func runProxyDaemon(cmd *cobra.Command, args []string) error {
	return proxy.RunDaemon()
}
