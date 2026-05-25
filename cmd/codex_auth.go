package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/api"
	"github.com/lisiting01/aitoll-cli/internal/auth"
	"github.com/lisiting01/aitoll-cli/internal/codexauth"
	"github.com/spf13/cobra"
)

// codex-auth flags
var (
	leaseDuration    string
	leaseForce       bool
	leaseNoBackup    bool
	leaseJSON        bool
	releaseKeepAuth  bool
	releaseJSON      bool
	statusJSON       bool
	refreshJSON      bool
	whoamiJSON       bool
	importNoBackup   bool
)

var codexAuthCmd = &cobra.Command{
	Use:   "codex-auth",
	Short: "Lease a Codex OpenAI account from the aitoll pool",
	Long: `Manage ~/.codex/auth.json via the aitoll account-lease pool.

Run 'aitoll codex-auth lease' to borrow a managed OpenAI account: this writes
~/.codex/auth.json (after backing up the existing one) and prints the email +
password you'll use to sign into the ChatGPT mobile app. Then 'aitoll
codex-remote start' connects this machine for remote dispatch.

If you have your own OpenAI account, skip this entirely — 'codex login' +
'aitoll codex-remote start' still works as before.`,
}

var codexAuthLeaseCmd = &cobra.Command{
	Use:   "lease",
	Short: "Borrow an OpenAI account, writing tokens to ~/.codex/auth.json",
	RunE:  runCodexAuthLease,
}

var codexAuthReleaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Return the leased account; restore the previous auth.json",
	RunE:  runCodexAuthRelease,
}

var codexAuthStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether the local auth.json is leased or owned",
	RunE:  runCodexAuthStatus,
}

var codexAuthRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Re-fetch auth.json from the platform (use when tokens are nearing expiry)",
	RunE:  runCodexAuthRefresh,
}

var codexAuthWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print the OpenAI email parsed from ~/.codex/auth.json (offline)",
	RunE:  runCodexAuthWhoami,
}

var codexAuthImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Copy an external auth.json into ~/.codex/ (after backup)",
	Args:  cobra.ExactArgs(1),
	RunE:  runCodexAuthImport,
}

var codexAuthDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose codex-auth prerequisites",
	RunE:  runCodexAuthDoctor,
}

func init() {
	rootCmd.AddCommand(codexAuthCmd)
	codexAuthCmd.AddCommand(
		codexAuthLeaseCmd,
		codexAuthReleaseCmd,
		codexAuthStatusCmd,
		codexAuthRefreshCmd,
		codexAuthWhoamiCmd,
		codexAuthImportCmd,
		codexAuthDoctorCmd,
	)

	codexAuthLeaseCmd.Flags().StringVar(&leaseDuration, "duration", "14d", "Lease duration: 7d, 14d, or 30d")
	codexAuthLeaseCmd.Flags().BoolVar(&leaseForce, "force", false, "Replace an existing active lease (cancels the old one)")
	codexAuthLeaseCmd.Flags().BoolVar(&leaseNoBackup, "no-backup", false, "Skip backing up the current auth.json before overwrite")
	codexAuthLeaseCmd.Flags().BoolVar(&leaseJSON, "json", false, "Output machine-readable JSON")

	codexAuthReleaseCmd.Flags().BoolVar(&releaseKeepAuth, "keep-auth", false, "Keep the leased auth.json instead of restoring the backup")
	codexAuthReleaseCmd.Flags().BoolVar(&releaseJSON, "json", false, "Output machine-readable JSON")

	codexAuthStatusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output machine-readable JSON")
	codexAuthRefreshCmd.Flags().BoolVar(&refreshJSON, "json", false, "Output machine-readable JSON")
	codexAuthWhoamiCmd.Flags().BoolVar(&whoamiJSON, "json", false, "Output machine-readable JSON")
	codexAuthImportCmd.Flags().BoolVar(&importNoBackup, "no-backup", false, "Skip backing up the current auth.json before overwrite")
}

// =============================================================================
// helpers
// =============================================================================

// requireAuth returns the user's stored credentials and an api Client, or
// emits a friendly "not logged in" message and returns (nil, nil, nil).
// Caller should treat (nil, nil, nil) as a signal to bail with exit 0.
func requireAuth(cmd *cobra.Command) (*auth.StoredCredentials, *api.Client, error) {
	store := auth.NewTokenStore()
	creds, err := store.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("read credentials: %w", err)
	}
	if creds == nil || creds.Token == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Not logged in. Run 'aitoll login' first.")
		return nil, nil, nil
	}
	return creds, api.NewClient(creds.BaseURL), nil
}

// formatRemaining prints "in 13d 4h" / "expired 2h ago" style.
func formatRemaining(d time.Duration) string {
	if d < 0 {
		return fmt.Sprintf("expired %s ago", roundDuration(-d))
	}
	return fmt.Sprintf("in %s", roundDuration(d))
}

func roundDuration(d time.Duration) string {
	if d >= 24*time.Hour {
		days := int(d / (24 * time.Hour))
		hours := int((d % (24 * time.Hour)) / time.Hour)
		if hours == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if d >= time.Hour {
		return d.Round(time.Hour).String()
	}
	if d >= time.Minute {
		return d.Round(time.Minute).String()
	}
	return d.Round(time.Second).String()
}

func parseTimeOrZero(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func emitJSON(cmd *cobra.Command, v interface{}) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// describeError unwraps EnvelopeError into a human-friendly tail line.
func describeError(err error) string {
	var ee *api.EnvelopeError
	if errors.As(err, &ee) {
		switch ee.Code {
		case "already_leased":
			return "you already hold an active lease — use --force to replace it"
		case "pool_empty":
			return "the lease pool has no available accounts; try again later"
		case "account_health_failed":
			return "no candidate account had a refreshable token (server-side issue)"
		case "agent_user_not_allowed":
			return "agent users cannot lease — sign in as a human user"
		case "lease_expired":
			return "your lease has expired; run 'aitoll codex-auth release' then 'lease' again"
		case "no_active_lease":
			return "no active lease to refresh"
		case "unauthorized":
			return "session token rejected — run 'aitoll login' again"
		}
		return ee.Error()
	}
	return err.Error()
}

// =============================================================================
// lease
// =============================================================================

func runCodexAuthLease(cmd *cobra.Command, args []string) error {
	creds, client, err := requireAuth(cmd)
	if err != nil {
		return err
	}
	if creds == nil {
		os.Exit(1)
	}

	// Acquire local lock so two concurrent CLIs don't race on auth.json.
	lock, err := codexauth.Acquire(2 * time.Second)
	if err != nil {
		return fmt.Errorf("could not acquire local lock: %w", err)
	}
	defer lock.Release()

	if !leaseJSON {
		fmt.Fprintln(cmd.OutOrStdout(), "Authenticating with platform...")
	}
	data, err := client.LeaseAccount(creds.Token, api.LeaseRequest{
		Duration: leaseDuration,
		Force:    leaseForce,
	})
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Lease failed:", describeError(err))
		var ee *api.EnvelopeError
		if errors.As(err, &ee) {
			switch ee.Code {
			case "unauthorized":
				os.Exit(1)
			case "pool_empty":
				os.Exit(2)
			case "already_leased":
				os.Exit(3)
			}
		}
		os.Exit(1)
		return nil
	}

	// Backup current auth.json (if any).
	var backupPath string
	if !leaseNoBackup {
		bp, berr := codexauth.Backup()
		if berr != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: backup failed:", berr)
		}
		backupPath = bp
	}

	// Write the new auth.json atomically.
	if err := codexauth.WriteAuthJSONAtomic(data.AuthJSON); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Lease succeeded on platform but writing auth.json failed:", err)
		os.Exit(4)
	}

	// Persist a local cache of lease metadata for fast status/doctor reads.
	expires := parseTimeOrZero(data.ExpiresAt)
	refreshAt := parseTimeOrZero(data.RefreshAt)
	leasedAt := time.Now()
	cache := &codexauth.LeaseCache{
		LeaseID:      data.LeaseID,
		OpenAIEmail:  data.OpenAI.Email,
		AccountType:  data.OpenAI.AccountType,
		LeasedAt:     leasedAt,
		ExpiresAt:    expires,
		RefreshAt:    refreshAt,
		BackupPath:   backupPath,
		PlatformBase: creds.BaseURL,
	}
	if err := codexauth.SaveLeaseCache(cache); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: could not write lease cache:", err)
	}

	if leaseJSON {
		out := map[string]interface{}{
			"leaseId":     data.LeaseID,
			"expiresAt":   data.ExpiresAt,
			"refreshAt":   data.RefreshAt,
			"openaiEmail": data.OpenAI.Email,
			"accountType": data.OpenAI.AccountType,
			"backupPath":  backupPath,
			"hasPassword": data.OpenAI.Password != nil,
		}
		if data.OpenAI.Password != nil {
			out["openaiPassword"] = *data.OpenAI.Password
		}
		return emitJSON(cmd, out)
	}

	authPath, _ := codexauth.AuthJSONPath()
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Acquired lease for OpenAI account: %s\n", data.OpenAI.Email)
	fmt.Fprintf(w, "Plan: %s | Lease expires: %s (%s)\n",
		data.OpenAI.AccountType,
		expires.Local().Format("2006-01-02 15:04:05"),
		formatRemaining(time.Until(expires)),
	)
	if backupPath != "" {
		fmt.Fprintf(w, "\nWrote %s\n  (backup at %s)\n", authPath, backupPath)
	} else {
		fmt.Fprintf(w, "\nWrote %s\n", authPath)
	}
	if data.OpenAI.Password != nil {
		fmt.Fprintln(w, "\nTo use this account on the ChatGPT mobile app:")
		fmt.Fprintf(w, "  Email:    %s\n", data.OpenAI.Email)
		fmt.Fprintf(w, "  Password: %s\n", *data.OpenAI.Password)
		fmt.Fprintln(w, "  (Shown once. The password is encrypted on the platform; it won't be shown again.)")
	} else {
		fmt.Fprintln(w, "\nNo password is configured for this account on the platform.")
		fmt.Fprintln(w, "If you need to sign into the ChatGPT mobile app, contact admin or use your own credentials.")
	}
	fmt.Fprintln(w, "\nNext: aitoll codex-remote start")
	return nil
}

// =============================================================================
// release
// =============================================================================

func runCodexAuthRelease(cmd *cobra.Command, args []string) error {
	creds, client, err := requireAuth(cmd)
	if err != nil {
		return err
	}
	if creds == nil {
		os.Exit(1)
	}

	lock, err := codexauth.Acquire(2 * time.Second)
	if err != nil {
		return fmt.Errorf("could not acquire local lock: %w", err)
	}
	defer lock.Release()

	alreadyExpired, hadActive, err := client.ReleaseLease(creds.Token)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Release failed:", describeError(err))
		os.Exit(1)
		return nil
	}

	// Restore the most recent backup unless --keep-auth.
	var restored string
	if !releaseKeepAuth {
		restored, _ = codexauth.RestoreLatest()
		if restored == "" {
			// no backup — delete auth.json so the user knows to re-login
			if err := codexauth.DeleteAuthJSON(); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "Warning: could not remove leased auth.json:", err)
			}
		}
	}

	_ = codexauth.ClearLeaseCache()

	if releaseJSON {
		return emitJSON(cmd, map[string]interface{}{
			"hadActiveLease": hadActive,
			"alreadyExpired": alreadyExpired,
			"restoredFrom":   restored,
			"keepAuth":       releaseKeepAuth,
		})
	}

	w := cmd.OutOrStdout()
	switch {
	case !hadActive:
		fmt.Fprintln(w, "No active lease on the platform; nothing to release server-side.")
	case alreadyExpired:
		fmt.Fprintln(w, "Lease was already past its expiry — server now marks it expired.")
	default:
		fmt.Fprintln(w, "Lease released.")
	}
	switch {
	case releaseKeepAuth:
		fmt.Fprintln(w, "Local ~/.codex/auth.json kept (--keep-auth).")
	case restored != "":
		fmt.Fprintf(w, "Restored ~/.codex/auth.json from %s\n", restored)
	default:
		fmt.Fprintln(w, "No backup found; ~/.codex/auth.json removed. Run 'codex login' to set up your own.")
	}
	return nil
}

// =============================================================================
// status
// =============================================================================

func runCodexAuthStatus(cmd *cobra.Command, args []string) error {
	creds, client, err := requireAuth(cmd)
	if err != nil {
		return err
	}

	// Always read local first; platform call is best-effort.
	cache, _ := codexauth.LoadLeaseCache()
	authPath, _ := codexauth.AuthJSONPath()
	authStat, _ := os.Stat(authPath)
	var local codexauth.AuthJSONInfo
	if authStat != nil {
		if blob, err := codexauth.ReadAuthJSON(); err == nil && blob != nil {
			local = codexauth.ParseAuthJSON(blob)
		}
	}

	var server *api.CodexAuthStatus
	if creds != nil {
		s, err := client.GetCodexAuthStatus(creds.Token)
		if err == nil {
			server = s
		} else if !statusJSON {
			fmt.Fprintln(cmd.ErrOrStderr(), "(could not reach platform: "+describeError(err)+")")
		}
	}

	if statusJSON {
		out := map[string]interface{}{
			"localAuthJson": map[string]interface{}{
				"present":     authStat != nil,
				"size":        sizeOrZero(authStat),
				"path":        authPath,
				"email":       local.Email,
				"accountId":   local.AccountID,
				"authMode":    local.AuthMode,
				"lastRefresh": local.LastRefresh,
			},
			"localCache": cache,
			"server":     server,
		}
		return emitJSON(cmd, out)
	}

	w := cmd.OutOrStdout()
	switch {
	case server != nil && server.Active:
		fmt.Fprintln(w, "Lease:        active")
		fmt.Fprintf(w, "OpenAI email: %s\n", server.OpenAIEmail)
		if server.AccountType != "" {
			fmt.Fprintf(w, "Plan:         %s\n", server.AccountType)
		}
		exp := parseTimeOrZero(server.ExpiresAt)
		fmt.Fprintf(w, "Leased at:    %s\n", parseTimeOrZero(server.LeasedAt).Local().Format("2006-01-02 15:04:05"))
		fmt.Fprintf(w, "Expires at:   %s (%s)\n", exp.Local().Format("2006-01-02 15:04:05"), formatRemaining(time.Until(exp)))
	case cache != nil && !cache.IsExpired():
		fmt.Fprintln(w, "Lease:        active (local cache; platform unreachable)")
		fmt.Fprintf(w, "OpenAI email: %s\n", cache.OpenAIEmail)
		fmt.Fprintf(w, "Expires at:   %s (%s)\n", cache.ExpiresAt.Local().Format("2006-01-02 15:04:05"), formatRemaining(cache.Remaining()))
	default:
		if local.Email != "" {
			fmt.Fprintf(w, "Lease:        none (auth.json owned by user, email: %s)\n", local.Email)
		} else if authStat != nil {
			fmt.Fprintln(w, "Lease:        none (auth.json present)")
		} else {
			fmt.Fprintln(w, "Lease:        none (no ~/.codex/auth.json)")
		}
	}

	if authStat != nil {
		fmt.Fprintf(w, "auth.json:    %s (%d bytes)\n", authPath, authStat.Size())
	} else {
		fmt.Fprintf(w, "auth.json:    %s (missing)\n", authPath)
	}
	if cache != nil && cache.BackupPath != "" {
		fmt.Fprintf(w, "Backup:       %s\n", cache.BackupPath)
	}
	return nil
}

func sizeOrZero(fi os.FileInfo) int64 {
	if fi == nil {
		return 0
	}
	return fi.Size()
}

// =============================================================================
// refresh
// =============================================================================

func runCodexAuthRefresh(cmd *cobra.Command, args []string) error {
	creds, client, err := requireAuth(cmd)
	if err != nil {
		return err
	}
	if creds == nil {
		os.Exit(1)
	}

	lock, err := codexauth.Acquire(2 * time.Second)
	if err != nil {
		return fmt.Errorf("could not acquire local lock: %w", err)
	}
	defer lock.Release()

	data, err := client.RefreshLease(creds.Token)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Refresh failed:", describeError(err))
		os.Exit(1)
		return nil
	}

	if err := codexauth.WriteAuthJSONAtomic(data.AuthJSON); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Got fresh tokens but writing auth.json failed:", err)
		os.Exit(4)
	}

	// Update cache.
	if cache, _ := codexauth.LoadLeaseCache(); cache != nil {
		cache.ExpiresAt = parseTimeOrZero(data.ExpiresAt)
		_ = codexauth.SaveLeaseCache(cache)
	}

	if refreshJSON {
		return emitJSON(cmd, map[string]interface{}{
			"refreshedAt": data.RefreshedAt,
			"expiresAt":   data.ExpiresAt,
		})
	}
	exp := parseTimeOrZero(data.ExpiresAt)
	fmt.Fprintf(cmd.OutOrStdout(), "auth.json refreshed; lease still expires %s (%s)\n",
		exp.Local().Format("2006-01-02 15:04:05"), formatRemaining(time.Until(exp)))
	return nil
}

// =============================================================================
// whoami
// =============================================================================

func runCodexAuthWhoami(cmd *cobra.Command, args []string) error {
	blob, err := codexauth.ReadAuthJSON()
	if err != nil {
		return err
	}
	if blob == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "No ~/.codex/auth.json present.")
		os.Exit(1)
	}
	info := codexauth.ParseAuthJSON(blob)
	if whoamiJSON {
		return emitJSON(cmd, info)
	}
	w := cmd.OutOrStdout()
	if info.Email == "" {
		fmt.Fprintln(w, "Could not parse email from id_token (auth_mode="+info.AuthMode+")")
	} else {
		fmt.Fprintf(w, "OpenAI email: %s\n", info.Email)
	}
	if info.AccountID != "" {
		fmt.Fprintf(w, "Account ID:   %s\n", info.AccountID)
	}
	if info.PlanType != "" {
		fmt.Fprintf(w, "Plan:         %s\n", info.PlanType)
	}
	if !info.LastRefresh.IsZero() {
		fmt.Fprintf(w, "Last refresh: %s\n", info.LastRefresh.Local().Format("2006-01-02 15:04:05"))
	}
	return nil
}

// =============================================================================
// import
// =============================================================================

func runCodexAuthImport(cmd *cobra.Command, args []string) error {
	src := args[0]
	body, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	// Validate JSON shape early so we don't overwrite auth.json with garbage.
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil {
		return fmt.Errorf("%s is not valid JSON: %w", src, err)
	}
	if !importNoBackup {
		if bp, berr := codexauth.Backup(); berr != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "Warning: backup failed:", berr)
		} else if bp != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Backed up existing auth.json to %s\n", bp)
		}
	}
	if err := codexauth.WriteAuthJSONRawAtomic(body); err != nil {
		return err
	}
	info := codexauth.ParseAuthJSON(probe)
	authPath, _ := codexauth.AuthJSONPath()
	fmt.Fprintf(cmd.OutOrStdout(), "Imported %s -> %s\n", src, authPath)
	if info.Email != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "OpenAI email: %s\n", info.Email)
	}
	return nil
}

// =============================================================================
// doctor
// =============================================================================

type doctorResult struct {
	level string // OK / WARN / ERROR
	msg   string
}

func runCodexAuthDoctor(cmd *cobra.Command, args []string) error {
	results := []doctorResult{
		checkLogin(),
		checkPlatformReachable(),
		checkCodexDir(),
		checkAuthJSONReadable(),
		checkLeaseStatus(),
		checkBackupsExist(),
	}

	hasError := false
	for _, r := range results {
		fmt.Fprintf(cmd.OutOrStdout(), "[%-5s] %s\n", r.level, r.msg)
		if r.level == "ERROR" {
			hasError = true
		}
	}
	if hasError {
		os.Exit(1)
	}
	return nil
}

func checkLogin() doctorResult {
	store := auth.NewTokenStore()
	creds, err := store.Load()
	if err != nil {
		return doctorResult{"ERROR", "could not read credentials: " + err.Error()}
	}
	if creds == nil || creds.Token == "" {
		return doctorResult{"ERROR", "not logged in — run 'aitoll login'"}
	}
	ok, _ := store.IsTokenValid()
	if !ok {
		return doctorResult{"ERROR", "session token expired — run 'aitoll login' again"}
	}
	return doctorResult{"OK", fmt.Sprintf("logged in (base %s)", creds.BaseURL)}
}

func checkPlatformReachable() doctorResult {
	store := auth.NewTokenStore()
	creds, _ := store.Load()
	if creds == nil {
		return doctorResult{"WARN", "skipped: no credentials"}
	}
	cli := api.NewClient(creds.BaseURL)
	if _, err := cli.GetCodexAuthStatus(creds.Token); err != nil {
		return doctorResult{"WARN", "platform /api/codex-auth/status unreachable: " + describeError(err)}
	}
	return doctorResult{"OK", "platform /api/codex-auth/status responded"}
}

func checkCodexDir() doctorResult {
	dir, err := codexauth.CodexDir()
	if err != nil {
		return doctorResult{"ERROR", err.Error()}
	}
	st, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return doctorResult{"WARN", dir + " missing — will be created on first lease"}
		}
		return doctorResult{"ERROR", "stat " + dir + ": " + err.Error()}
	}
	if !st.IsDir() {
		return doctorResult{"ERROR", dir + " is not a directory"}
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		return doctorResult{"WARN", fmt.Sprintf("%s permissions are %o (recommend 0700)", dir, st.Mode().Perm())}
	}
	return doctorResult{"OK", dir + " exists"}
}

func checkAuthJSONReadable() doctorResult {
	path, _ := codexauth.AuthJSONPath()
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return doctorResult{"WARN", path + " missing — run 'aitoll codex-auth lease' or 'codex login'"}
		}
		return doctorResult{"ERROR", "stat " + path + ": " + err.Error()}
	}
	blob, err := codexauth.ReadAuthJSON()
	if err != nil {
		return doctorResult{"ERROR", "parse " + path + ": " + err.Error()}
	}
	info := codexauth.ParseAuthJSON(blob)
	if info.Email != "" {
		return doctorResult{"OK", fmt.Sprintf("%s present (%d bytes, email=%s)", path, st.Size(), info.Email)}
	}
	return doctorResult{"OK", fmt.Sprintf("%s present (%d bytes)", path, st.Size())}
}

func checkLeaseStatus() doctorResult {
	cache, err := codexauth.LoadLeaseCache()
	if err != nil {
		return doctorResult{"WARN", "could not read lease cache: " + err.Error()}
	}
	if cache == nil {
		return doctorResult{"OK", "no active lease tracked locally (auth.json may be user-owned)"}
	}
	if cache.IsExpired() {
		return doctorResult{"WARN", fmt.Sprintf("local cache shows lease %s expired %s ago — run 'codex-auth release'",
			cache.OpenAIEmail, roundDuration(-cache.Remaining()))}
	}
	return doctorResult{"OK", fmt.Sprintf("active lease: %s, expires %s", cache.OpenAIEmail, formatRemaining(cache.Remaining()))}
}

func checkBackupsExist() doctorResult {
	backups, err := codexauth.ListBackups()
	if err != nil {
		return doctorResult{"WARN", "could not list backups: " + err.Error()}
	}
	if len(backups) == 0 {
		return doctorResult{"WARN", "no auth.backup.*.json files (release without --keep-auth would have nothing to restore)"}
	}
	return doctorResult{"OK", fmt.Sprintf("%d backup(s) available", len(backups))}
}

// silence unused-import warnings in case some flow paths above are stripped.
var _ = strings.HasPrefix
var _ = io.EOF
var _ = http.MethodGet
