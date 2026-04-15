package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	"github.com/lisiting01/aitoll-cli/internal/auth"
	"github.com/lisiting01/aitoll-cli/pkg/browser"
	"github.com/spf13/cobra"
)

var (
	callbackPort int
	noBrowser    bool
	forceLogin   bool
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with AI-Toll platform",
	Long:  "Open a browser to login to the AI-Toll platform. After successful login, the token will be saved locally.",
	RunE:  runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
	loginCmd.Flags().IntVar(&callbackPort, "port", defaultPort, "Local callback server port")
	loginCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Print login URL instead of opening browser")
	loginCmd.Flags().BoolVar(&forceLogin, "force", false, "Re-login even if already authenticated")
}

func runLogin(cmd *cobra.Command, args []string) error {
	store := auth.NewTokenStore()

	// Check existing auth
	if !forceLogin {
		valid, err := store.IsTokenValid()
		if err == nil && valid {
			creds, _ := store.Load()
			if creds != nil {
				claims, err := auth.DecodeTokenClaims(creds.Token)
				if err == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Already logged in as %s (%s). Use --force to re-login.\n", claims.Username, claims.Email)
					return nil
				}
			}
		}
	}

	// Generate random state for CSRF protection
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	// Start callback server
	cb := auth.NewCallbackServer(state)
	listenAddr, err := cb.Start(callbackPort)
	if err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}
	defer cb.Stop()

	// Build authorization URL
	authURL := fmt.Sprintf("%s/auth/cli-login?redirect_uri=%s&state=%s",
		baseURL,
		url.QueryEscape(listenAddr+"/callback"),
		url.QueryEscape(state),
	)

	// Open browser or print URL
	if noBrowser {
		fmt.Fprintf(cmd.OutOrStdout(), "Open this URL to login:\n%s\n", authURL)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Opening browser for login...\n")
		if err := browser.Open(authURL); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Could not open browser automatically. Open this URL manually:\n%s\n", authURL)
		}
	}

	// Wait for callback
	fmt.Fprintln(cmd.OutOrStdout(), "Waiting for authentication...")
	token, err := cb.WaitForResult(5 * time.Minute)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Store token
	if err := store.Save(token, baseURL); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	// Print success
	claims, err := auth.DecodeTokenClaims(token)
	if err != nil {
		fmt.Fprintln(cmd.OutOrStdout(), "Login successful!")
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s (%s)\n", claims.Username, claims.Email)
	return nil
}

const defaultPort = 17310
