package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	repoOwner = "lisiting01"
	repoName  = "agentsworkhub-cli"
	apiURL    = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases/latest"
)

// ReleaseInfo holds the relevant fields from the GitHub releases API.
type ReleaseInfo struct {
	TagName string        `json:"tag_name"`
	Assets  []ReleaseAsset `json:"assets"`
}

// ReleaseAsset is a single downloadable file in a release.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckLatest fetches the latest release tag from GitHub.
func CheckLatest() (*ReleaseInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var info ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}
	return &info, nil
}

// IsNewer returns true if latestTag is a higher version than currentVersion.
// Both are expected in "v1.2.3" or "1.2.3" format.
func IsNewer(currentVersion, latestTag string) bool {
	cur := strings.TrimPrefix(currentVersion, "v")
	lat := strings.TrimPrefix(latestTag, "v")
	if cur == "dev" || cur == "" {
		return false
	}
	return lat != cur
}

// assetName returns the expected release asset filename for the current platform.
// Convention: aitoll-<os>-<arch>[.exe]
func assetName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	name := fmt.Sprintf("aitoll-%s-%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// FindAsset locates the matching asset for the current platform in a release.
func FindAsset(info *ReleaseInfo) (*ReleaseAsset, error) {
	target := assetName()
	for i := range info.Assets {
		if strings.EqualFold(info.Assets[i].Name, target) {
			return &info.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("no asset found for %s/%s (expected %q)", runtime.GOOS, runtime.GOARCH, target)
}

// DownloadAndReplace downloads the asset and replaces the running binary.
// It writes to a temp file first, then atomically renames.
func DownloadAndReplace(asset *ReleaseAsset) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("cannot resolve symlinks: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(asset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	// Write to a temp file in the same directory so rename is atomic.
	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, ".aitoll-update-*")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { os.Remove(tmpPath) }() // clean up on failure

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write update: %w", err)
	}
	tmp.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// On Windows, we can't replace a running binary directly.
	// Rename the current binary to a .old file, then move the new one in.
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		os.Remove(oldPath) // ignore error — may not exist
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("failed to move current binary: %w", err)
		}
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	return nil
}
