package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/cli"
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

func (b blockingSource) Refresh(ctx context.Context, _ domain.ScopeSet) (tui.Result, error) {
	b.started <- struct{}{}
	<-ctx.Done()
	b.canceled <- ctx.Err()
	return tui.Result{}, ctx.Err()
}

// mustScope builds a Scope for the launch and adapter tests.
func mustScope(t *testing.T, values ...string) domain.ScopeSet {
	t.Helper()

	scope, err := domain.NewRepositoryScopeSet(values)
	if err != nil {
		t.Fatalf("NewRepositoryScopeSet(%v) failed: %v", values, err)
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

func TestLaunchWithoutASourceFailsBeforeTakingTheTerminal(t *testing.T) {
	err := launchProgram(context.Background(), mustScope(t, "acme/backend"), nil, nil, nil, tui.Capabilities{})

	if !errors.Is(err, tui.ErrNoSource) {
		t.Fatalf("launchProgram without a source returned %v, want tui.ErrNoSource", err)
	}
}

func TestQuitKeystrokeExitsCleanlyAndCancelsInFlightSourceWork(t *testing.T) {
	source := newBlockingSource()
	input, keystrokes := io.Pipe()
	defer func() { _ = input.Close() }()

	exit := make(chan error, 1)
	go func() {
		exit <- launchProgram(context.Background(), mustScope(t, "acme/backend"), source, nil, nil, tui.Capabilities{}, headless(input)...)
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
		exit <- launchProgram(ctx, mustScope(t, "acme/backend"), source, nil, nil, tui.Capabilities{}, headless(input)...)
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

// blockingExpander keeps one expansion in flight until its context is canceled,
// which is what shutdown has to do for expansion work too (NFR-001).
type blockingExpander struct {
	started  chan struct{}
	canceled chan error
}

func newBlockingExpander() blockingExpander {
	return blockingExpander{started: make(chan struct{}, 1), canceled: make(chan error, 1)}
}

func (b blockingExpander) Expand(ctx context.Context) (tui.Expansion, error) {
	b.started <- struct{}{}
	<-ctx.Done()
	b.canceled <- ctx.Err()
	return tui.Expansion{}, ctx.Err()
}

func TestOrganizationOnlyLaunchExpandsBeforeItPollsAndCancelsThatWork(t *testing.T) {
	source := newBlockingSource()
	expander := newBlockingExpander()
	input, keystrokes := io.Pipe()
	defer func() { _ = input.Close() }()

	exit := make(chan error, 1)
	go func() {
		exit <- launchProgram(context.Background(), domain.ScopeSet{}, source, expander, nil, tui.Capabilities{}, headless(input)...)
	}()

	select {
	case <-expander.started:
	case <-time.After(launchTimeout):
		t.Fatal("the organization-only launch never started an expansion")
	}
	select {
	case <-source.started:
		t.Fatal("the launch polled before its expansion completed")
	case <-time.After(50 * time.Millisecond):
	}

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
	select {
	case err := <-expander.canceled:
		if err == nil {
			t.Error("the in-flight expansion reported no cancellation cause")
		}
	case <-time.After(launchTimeout):
		t.Fatal("shutdown left the in-flight expansion running")
	}
}

func TestLaunchBindsAnExpanderOnlyForAnOrganizationSelection(t *testing.T) {
	selectors, err := cli.ParseArgs("orgtop", []string{"--org", "acme"}, io.Discard)
	if err != nil {
		t.Fatalf("parsing an organization selection failed: %v", err)
	}
	if got := expanderFor(auth.Credential{}, selectors); got == nil {
		t.Error("an organization selection got no expander, want the launch to expand before polling")
	}

	exact, err := cli.ParseArgs("orgtop", []string{"--repo", "acme/backend"}, io.Discard)
	if err != nil {
		t.Fatalf("parsing an exact selection failed: %v", err)
	}
	if got := expanderFor(auth.Credential{}, exact); got != nil {
		t.Errorf("an exact selection got expander %v, want none so it polls the selection it was given", got)
	}
}

// launchEnvironment is the set of variables the rendering capability
// resolution reads. Every one is pinned per fixture, so an inherited value of
// the developer's own terminal never decides a launch test. WT_SESSION and
// GOOGLE_CLOUD_SHELL belong to the set because the profile stack forces
// truecolor on either one without consulting TERM at all, so a run under
// Windows Terminal or Cloud Shell would otherwise resolve a colour the fixture
// never asked for.
var launchEnvironment = []string{
	"TERM", "COLORTERM", "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "TTY_FORCE",
	"LANG", "LC_ALL", "LC_CTYPE", "WT_SESSION", "GOOGLE_CLOUD_SHELL",
}

// pinEnvironment clears the launch environment and applies the fixture's own
// values for the duration of the test.
func pinEnvironment(t *testing.T, values map[string]string) {
	t.Helper()

	for _, name := range launchEnvironment {
		t.Setenv(name, "")
	}
	for name, value := range values {
		t.Setenv(name, value)
	}
}

func TestLaunchResolvesRenderingCapabilitiesFromTheProcessEnvironment(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{
			name:   "utf-8 truecolor",
			values: map[string]string{"LANG": "en_US.UTF-8", "TERM": "xterm", "COLORTERM": "truecolor", "CLICOLOR_FORCE": "1"},
			want:   "utf-8/truecolor",
		},
		{
			name:   "utf-8 ansi",
			values: map[string]string{"LC_ALL": "C.UTF-8", "TERM": "xterm", "CLICOLOR_FORCE": "1"},
			want:   "utf-8/ansi",
		},
		{
			name:   "non-utf-8 locale keeps colour",
			values: map[string]string{"LANG": "en_US.ISO-8859-1", "TERM": "xterm", "COLORTERM": "truecolor", "CLICOLOR_FORCE": "1"},
			want:   "ascii/truecolor",
		},
		{
			name:   "dumb terminal",
			values: map[string]string{"LANG": "en_US.UTF-8", "TERM": "dumb", "COLORTERM": "truecolor", "CLICOLOR_FORCE": "1"},
			want:   "ascii/no-color",
		},
		{
			name:   "NO_COLOR changes colour only",
			values: map[string]string{"LANG": "en_US.UTF-8", "TERM": "xterm", "COLORTERM": "truecolor", "CLICOLOR_FORCE": "1", "NO_COLOR": "x"},
			want:   "utf-8/no-color",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			pinEnvironment(t, testCase.values)

			if got := resolveCapabilities(io.Discard).String(); got != testCase.want {
				t.Errorf("the launch resolved %q for %v, want %q", got, testCase.values, testCase.want)
			}
		})
	}
}

func TestLaunchModelRendersAtTheResolvedCapabilities(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{
			name:   "a utf-8 terminal draws the shared glyph",
			values: map[string]string{"LANG": "en_US.UTF-8", "TERM": "xterm", "COLORTERM": "truecolor", "CLICOLOR_FORCE": "1"},
			want:   "↑ push",
		},
		{
			name:   "a dumb terminal falls back to ascii",
			values: map[string]string{"LANG": "en_US.UTF-8", "TERM": "dumb", "CLICOLOR_FORCE": "1"},
			want:   "P push",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			pinEnvironment(t, testCase.values)

			model, err := launchModel(t.Context(), mustScope(t, "acme/backend"), newBlockingSource(), nil, nil, resolveCapabilities(io.Discard))
			if err != nil {
				t.Fatalf("building the launch model failed: %v", err)
			}
			sized, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
			rain, _ := sized.Update(tea.KeyPressMsg{Code: '3', Text: "3"})

			if content := rain.View().Content; !strings.Contains(content, testCase.want) {
				t.Errorf("the launched Rain legend omits %q:\n%s", testCase.want, content)
			}
		})
	}
}
