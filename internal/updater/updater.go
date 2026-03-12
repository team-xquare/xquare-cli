package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const apiURL = "https://api.github.com/repos/team-xquare/xquare-cli/releases/latest"

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// LatestVersion fetches the latest release tag from GitHub (5s timeout).
func LatestVersion() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.TagName, nil
}

// IsNewerVersion reports whether latest is strictly newer than current.
// Returns false when current is "dev" or empty.
func IsNewerVersion(current, latest string) bool {
	if current == "dev" || current == "" {
		return false
	}
	cv := parseSemver(strings.TrimPrefix(current, "v"))
	lv := parseSemver(strings.TrimPrefix(latest, "v"))
	for i := range cv {
		if lv[i] > cv[i] {
			return true
		}
		if lv[i] < cv[i] {
			return false
		}
	}
	return false
}

func parseSemver(s string) [3]int {
	var v [3]int
	parts := strings.SplitN(s, ".", 3)
	for i, p := range parts {
		if i >= 3 {
			break
		}
		p = strings.SplitN(p, "-", 2)[0]
		v[i], _ = strconv.Atoi(p)
	}
	return v
}

// CheckForUpdate reads the version cache and prints a stderr notification when
// a newer version is available. The cache is refreshed in a background goroutine
// when it is missing or older than 24 hours.
func CheckForUpdate(currentVersion string) {
	cached, age := readCache()
	if cached != "" && IsNewerVersion(currentVersion, cached) {
		fmt.Fprintf(os.Stderr, "\n💡 New version %s available. Run: xquare upgrade\n\n", cached)
	}
	if cached == "" || age > 24*time.Hour {
		go func() {
			latest, err := LatestVersion()
			if err == nil {
				writeCache(latest)
			}
		}()
	}
}

// Upgrade downloads the latest release binary and atomically replaces the
// current executable.
func Upgrade(currentVersion string) error {
	fmt.Fprintln(os.Stderr, "Fetching latest release info...")
	rel, err := fetchRelease()
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}

	if !IsNewerVersion(currentVersion, rel.TagName) {
		fmt.Fprintf(os.Stderr, "Already up to date (%s)\n", currentVersion)
		return nil
	}

	assetName := archiveName(rel.TagName)
	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no binary found for %s/%s in release %s (expected %s)", runtime.GOOS, runtime.GOARCH, rel.TagName, assetName)
	}

	fmt.Fprintf(os.Stderr, "Downloading %s...\n", rel.TagName)
	binData, err := downloadAndExtract(downloadURL, assetName)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	if err := replaceExec(execPath, binData); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	writeCache(rel.TagName)
	fmt.Fprintf(os.Stderr, "✓ Updated to %s\n", rel.TagName)
	return nil
}

func fetchRelease() (*ghRelease, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

func archiveName(tag string) string {
	ver := strings.TrimPrefix(tag, "v")
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("xquare_%s_%s_%s.zip", ver, runtime.GOOS, runtime.GOARCH)
	}
	return fmt.Sprintf("xquare_%s_%s_%s.tar.gz", ver, runtime.GOOS, runtime.GOARCH)
}

func downloadAndExtract(url, assetName string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if strings.HasSuffix(assetName, ".zip") {
		return extractZip(resp.Body)
	}
	return extractTarGz(resp.Body)
}

func extractTarGz(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == "xquare" {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("xquare binary not found in archive")
}

func extractZip(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == "xquare.exe" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("xquare.exe not found in archive")
}

func replaceExec(execPath string, data []byte) error {
	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, ".xquare-update-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		cleanup()
		return err
	}

	// Windows cannot overwrite a running executable; rename it to .old first.
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(execPath, oldPath); err != nil {
			cleanup()
			return err
		}
	}

	if err := os.Rename(tmpPath, execPath); err != nil {
		cleanup()
		return err
	}
	return nil
}

// Cache helpers — stored in ~/.xquare/version_cache

func cachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".xquare", "version_cache")
}

func readCache() (version string, age time.Duration) {
	p := cachePath()
	info, err := os.Stat(p)
	if err != nil {
		return "", 0
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", 0
	}
	return strings.TrimSpace(string(data)), time.Since(info.ModTime())
}

func writeCache(version string) {
	_ = os.WriteFile(cachePath(), []byte(version), 0644)
}
