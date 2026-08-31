package main

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/cli"
	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/tui"
)

// launch runs the terminal UI against the public GitHub API.
func launch(ctx context.Context, config cli.Config, credential auth.Credential) error {
	return launchProgram(ctx, config.Scopes, newSourceAdapter(credential))
}

// launchProgram runs the Bubble Tea shell for the selection until it exits. The program
// and every refresh it starts share one context, so a canceled process context
// ends both, and returning cancels whatever source work is still in flight no
// matter which path ended the program (NFR-001).
func launchProgram(ctx context.Context, scopes domain.ScopeSet, source tui.Source, options ...tea.ProgramOption) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	model, err := tui.New(ctx, scopes, source)
	if err != nil {
		return fmt.Errorf("building the terminal ui: %w", err)
	}

	programOptions := append([]tea.ProgramOption{tea.WithContext(ctx)}, options...)
	program := tea.NewProgram(model, programOptions...)
	if _, err := program.Run(); err != nil && !isShutdown(err) {
		return fmt.Errorf("running the terminal ui: %w", err)
	}
	return nil
}

// isShutdown reports whether the program ended through a requested shutdown
// rather than a failure: an interrupt and a canceled process context both
// terminate cleanly (FR-001).
func isShutdown(err error) bool {
	return errors.Is(err, tea.ErrInterrupted) || errors.Is(err, context.Canceled)
}
