package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func executablePath(dir, name, goos string) string {
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name)
}

func buildCLI(t *testing.T, dir string) string {
	t.Helper()
	binary := executablePath(dir, "orgtop", runtime.GOOS)
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	return binary
}

// A native gh executable keeps credential lookup observable on every platform.
// Build before isolating PATH/HOME, so the Go toolchain still uses its own cache.
func credentialProbe(t *testing.T, dir string) string {
	t.Helper()
	source := filepath.Join(dir, "gh.go")
	if err := os.WriteFile(source, []byte(`package main
import "os"
func main() {
	if err := os.WriteFile(os.Getenv("ORGTOP_CREDENTIAL_MARKER"), []byte("called"), 0600); err != nil {
		panic(err)
	}
	os.Exit(1)
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := executablePath(dir, "gh", runtime.GOOS)
	if output, err := exec.Command("go", "build", "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatalf("build credential probe: %v\n%s", err, output)
	}
	marker := filepath.Join(dir, "credential-called")
	t.Setenv("ORGTOP_CREDENTIAL_MARKER", marker)
	t.Setenv("PATH", dir)
	// Positive control: ensure a real gh lookup would reach the probe.
	output, err := exec.Command("gh", "auth", "token", "--hostname", "github.com").CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("credential probe exit: %v; output: %s", err, output)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "called" {
		t.Fatalf("credential probe marker: %q, %v", got, err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	return marker
}

func isolateExecutable(t *testing.T, root string) string {
	t.Helper()
	for _, key := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "XDG_CACHE_HOME"} {
		t.Setenv(key, root)
	}
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	return cacheRoot
}

func TestExecutablePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "directory with spaces")
	for _, tt := range []struct{ goos, name string }{
		{"windows", "orgtop.exe"}, {"linux", "orgtop"}, {"darwin", "orgtop"},
	} {
		t.Run(tt.goos, func(t *testing.T) {
			if got, want := executablePath(dir, "orgtop", tt.goos), filepath.Join(dir, tt.name); got != want {
				t.Fatalf("executable path = %q, want %q", got, want)
			}
		})
	}
}

func TestExecutableIsolation(t *testing.T) {
	for _, key := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "XDG_CACHE_HOME", "GH_TOKEN", "GITHUB_TOKEN"} {
		t.Setenv(key, "inherited sentinel")
	}
	root := t.TempDir()
	got := isolateExecutable(t, root)
	for _, key := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "XDG_CACHE_HOME"} {
		if value := os.Getenv(key); value != root {
			t.Errorf("%s = %q, want isolated root %q", key, value, root)
		}
	}
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if os.Getenv(key) != "" {
			t.Errorf("%s retained an inherited credential", key)
		}
	}
	want := root
	if runtime.GOOS == "darwin" {
		want = filepath.Join(root, "Library", "Caches")
	}
	if got != want {
		t.Errorf("fixture cache root = %q, want %q", got, want)
	}
	if actual, err := os.UserCacheDir(); err != nil || actual != want {
		t.Errorf("OS cache root = %q, %v; want %q", actual, err, want)
	}
}
