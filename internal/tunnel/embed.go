// Package tunnel provides a WebSocket-to-TCP tunnel using the embedded wstunnel binary.
package tunnel

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

//go:embed bin/*
var binaries embed.FS

// ExtractWstunnel extracts the platform-specific wstunnel binary to a temp file
// and returns its path. The caller is responsible for removing it.
func ExtractWstunnel() (string, error) {
	name, err := binaryName()
	if err != nil {
		return "", err
	}

	data, err := binaries.ReadFile("bin/" + name)
	if err != nil {
		return "", fmt.Errorf("wstunnel binary not found for %s/%s: %w", runtime.GOOS, runtime.GOARCH, err)
	}

	tmp, err := os.CreateTemp("", "wstunnel-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write binary: %w", err)
	}

	if err := os.Chmod(tmp.Name(), 0755); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("chmod: %w", err)
	}

	return tmp.Name(), nil
}

func binaryName() (string, error) {
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return "wstunnel_linux_amd64", nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "wstunnel_darwin_arm64", nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return "wstunnel_darwin_amd64", nil
	default:
		// Fallback: try system-installed wstunnel
		path, err := exec.LookPath("wstunnel")
		if err != nil {
			return "", fmt.Errorf("no embedded wstunnel for %s/%s and none found in PATH", runtime.GOOS, runtime.GOARCH)
		}
		return filepath.Base(path), nil
	}
}
