package main

import (
	"context"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/tui"
)

// launchTimeout bounds every program launch test so a shutdown regression fails
// instead of hanging the suite.
const launchTimeout = 10 * time.Second

// blockingSource keeps one refresh in flight until its context is canceled,
// which is what shutdown has to do (NFR-001).
type blockingSource struct {
	started  chan struct{}
	canceled chan error
}

func newBlockingSource() blockingSource {
	return blockingSource{started: make(chan struct{}, 1), canceled: make(chan error, 1)}
}

func (b blockingSource) Refresh(ctx context.Context, _ domain.Scope) (tui.Result, error) {
	b.started <- struct{}{}
	<-ctx.Done()
	b.canceled <- ctx.Err()
	return tui.Result{}, ctx.Err()
}

// mustScope builds a Scope for the launch and adapter tests.
func mustScope(t *testing.T, values ...string) domain.Scope {
	t.Helper()

	scope, err := domain.NewScope(values)
	if err != nil {
		t.Fatalf("NewScope(%v) failed: %v", values, err)
	}
	return scope
}

// headless returns the program options that run the shell without a terminal.
func headless(input io.Reader) []tea.ProgramOption {
	return []tea.ProgramOption{
		tea.WithInput(input),
		tea.WithOutput(io.Discard),
		tea.WithoutSignalHandler(),
	}
}

// awaitRefresh fails unless the first refresh reaches the source.
func awaitRefresh(t *testing.T, source blockingSource) {
	t.Helper()

	select {
	case <-source.started:
	case <-time.After(launchTimeout):
		t.Fatal("the launch never started a refresh")
	}
}

// awaitCancellation fails unless shutdown cancels the in-flight refresh.
func awaitCancellation(t *testing.T, source blockingSource) {
	t.Helper()

	select {
	case err := <-source.canceled:
		if err == nil {
			t.Error("the in-flight refresh reported no cancellation cause")
		}
	case <-time.After(launchTimeout):
		t.Fatal("shutdown left the in-flight source request running")
	}
}

func TestQuitKeystrokeExitsCleanlyAndCancelsInFlightSourceWork(t *testing.T) {
	source := newBlockingSource()
	input, keystrokes := io.Pipe()
	defer func() { _ = input.Close() }()

	exit := make(chan error, 1)
	go func() {
		exit <- launchProgram(context.Background(), mustScope(t, "acme/backend"), source, headless(input)...)
	}()

	awaitRefresh(t, source)
	if _, err := keystrokes.Write([]byte("q")); err != nil {
		t.Fatalf("writing the quit keystroke failed: %v", err)
	}

	select {
	case err := <-exit:
		if err != nil {
			t.Errorf("launchProgram returned %v, want a clean exit", err)
		}
	case <-time.After(launchTimeout):
		t.Fatal("the quit keystroke did not end the program")
	}
	awaitCancellation(t, source)
}

func TestProcessCancellationExitsCleanlyAndCancelsInFlightSourceWork(t *testing.T) {
	source := newBlockingSource()
	input, keystrokes := io.Pipe()
	defer func() { _ = keystrokes.Close() }()
	defer func() { _ = input.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	exit := make(chan error, 1)
	go func() {
		exit <- launchProgram(ctx, mustScope(t, "acme/backend"), source, headless(input)...)
	}()

	awaitRefresh(t, source)
	cancel()

	select {
	case err := <-exit:
		if err != nil {
			t.Errorf("launchProgram returned %v, want a clean exit", err)
		}
	case <-time.After(launchTimeout):
		t.Fatal("the canceled process context did not end the program")
	}
	awaitCancellation(t, source)
}
