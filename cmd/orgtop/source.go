package main

import (
	"context"
	"errors"
	"time"

	"github.com/fmueller/orgtop/internal/auth"
	"github.com/fmueller/orgtop/internal/domain"
	"github.com/fmueller/orgtop/internal/github"
	"github.com/fmueller/orgtop/internal/tui"
)

// sourceAdapter adapts the GitHub activity source to the shell's refresh seam.
// The shell declares its own Source and Result because internal/tui must not
// import internal/github (NFR-004); this is the only place the two meet.
type sourceAdapter struct {
	source github.Source
}

// newSourceAdapter binds the adapter to the public GitHub API.
func newSourceAdapter(credential auth.Credential) sourceAdapter {
	return sourceAdapter{source: github.NewSource(credential)}
}

// Refresh maps one atomic GitHub refresh onto the lifecycle's Result. Both
// outcomes carry scheduling metadata: a failure reports the source's retry
// delay so the lifecycle never re-derives GitHub's rules.
func (a sourceAdapter) Refresh(ctx context.Context, scope domain.Scope) (tui.Result, error) {
	refresh, err := a.source.Refresh(ctx, scope)
	if err != nil {
		return tui.Result{Delay: retryDelay(err)}, err
	}
	return tui.Result{Repositories: refresh.Repositories, Delay: refresh.PollDelay}, nil
}

// retryDelay reports the delay the source attached to a failed refresh, or zero
// when the failure carries none; the lifecycle applies its own polling floor.
func retryDelay(err error) time.Duration {
	var failure *github.RefreshError
	if errors.As(err, &failure) {
		return failure.RetryDelay
	}
	return 0
}
