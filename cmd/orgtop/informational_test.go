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
	binary := filepath.Join(dir, "orgtop")
	if output, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	marker := filepath.Join(dir, "credential-called")
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\necho called > '"+marker+"'\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cacheRoot := filepath.Join(dir, "cache")
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
				cmd.Env = append(os.Environ(), "GH_TOKEN=", "GITHUB_TOKEN=", "PATH="+dir, "XDG_CACHE_HOME="+cacheRoot)
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
