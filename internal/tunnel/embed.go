// Package tunnel provides a WebSocket-to-TCP tunnel using the embedded wstunnel binary.
package tunnel

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

//go:embed bin/*
var binaries embed.FS

// ExtractWstunnel extracts the platform-specific wstunnel binary to a temp file
// and returns its path. The caller is responsible for removing it.
// Returns (path, cleanup, error). Call cleanup() when done.
func ExtractWstunnel() (path string, cleanup func(), err error) {
	name := embeddedBinaryName()

	// Try embedded binary first
	if name != "" {
		data, readErr := binaries.ReadFile("bin/" + name)
		if readErr == nil {
			tmp, tmpErr := os.CreateTemp("", "wstunnel-*")
			if tmpErr != nil {
				return "", nil, fmt.Errorf("create temp file: %w", tmpErr)
			}
			if _, writeErr := tmp.Write(data); writeErr != nil {
				tmp.Close()
				os.Remove(tmp.Name())
				return "", nil, fmt.Errorf("write binary: %w", writeErr)
			}
			tmp.Close()
			if chmodErr := os.Chmod(tmp.Name(), 0700); chmodErr != nil {
				os.Remove(tmp.Name())
				return "", nil, fmt.Errorf("chmod: %w", chmodErr)
			}
			return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil
		}
	}

	// Fallback: use system-installed wstunnel
	sysPath, lookErr := exec.LookPath("wstunnel")
	if lookErr == nil {
		return sysPath, func() {}, nil
	}

	return "", nil, fmt.Errorf(
		"wstunnel not available for %s/%s and not found in PATH\n"+
			"Install from: https://github.com/erebe/wstunnel/releases",
		runtime.GOOS, runtime.GOARCH,
	)
}

// embeddedBinaryName returns the embedded binary filename for the current platform,
// or empty string if there is no embedded binary for this platform.
func embeddedBinaryName() string {
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return "wstunnel_linux_amd64"
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		return "wstunnel_linux_arm64"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return "wstunnel_darwin_amd64"
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "wstunnel_darwin_arm64"
	case runtime.GOOS == "windows" && runtime.GOARCH == "amd64":
		return "wstunnel_windows_amd64.exe"
	default:
		return ""
	}
}
