package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInformationalExecutable(t *testing.T) {
	dir := t.TempDir()
	binary := buildCLI(t, dir)
	marker := credentialProbe(t, dir)
	cacheRoot := isolateExecutable(t, filepath.Join(dir, "cache"))
	cache := filepath.Join(cacheRoot, "orgtop", "enrichment-v1.db")
	if err := os.MkdirAll(filepath.Dir(cache), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte("preserve cache sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, info := range []string{"--help", "-h", "--version", "-v"} {
		for _, args := range [][]string{
			{"--bad", info}, {info, "--reset-cache", "--bad"},
			{info, "--repo"}, {"--repo=broken", info},
		} {
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, binary, args...)
				var stdout, stderr bytes.Buffer
				cmd.Stdout, cmd.Stderr = &stdout, &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("exit: %v; stderr: %s", err, stderr.String())
				}
				if info == "--help" || info == "-h" {
					if stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), "Usage: orgtop ") || strings.Contains(stderr.String(), "flag provided") {
						t.Fatalf("help streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
					}
				} else if stdout.String() != "orgtop dev\n" || stderr.Len() != 0 {
					t.Fatalf("version streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatalf("credential hook ran: %v", err)
				}
				if got, err := os.ReadFile(cache); err != nil || string(got) != "preserve cache sentinel" {
					t.Fatalf("cache changed: %q, %v", got, err)
				}
			})
		}
	}
}
