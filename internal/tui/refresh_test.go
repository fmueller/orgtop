package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/fmueller/orgtop/internal/domain"
)

// outcome is one scripted refresh result.
type outcome struct {
	result Result
	err    error
}

// fakeSource is the injected activity source under test. It is deterministic:
// no clock, no network, and no goroutine of its own. A refresh runs only when
// the lifecycle command that owns it is executed.
type fakeSource struct {
	// outcomes are returned in order; the last one repeats.
	outcomes []outcome
	// calls counts started refreshes.
	calls int
	// contexts records the context of every started refresh.
	contexts []context.Context
}

func (s *fakeSource) Refresh(ctx context.Context, _ domain.Scope) (Result, error) {
	s.calls++
	s.contexts = append(s.contexts, ctx)
	if len(s.outcomes) == 0 {
		return Result{}, nil
	}
	next := s.outcomes[min(s.calls-1, len(s.outcomes)-1)]
	return next.result, next.err
}

// recorder captures the delays the lifecycle schedules the next attempt with.
type recorder struct {
	delays []time.Duration
}

// tick is the timer seam that records the delay instead of waiting.
func (r *recorder) tick(delay time.Duration) tea.Cmd {
	r.delays = append(r.delays, delay)
	return func() tea.Msg { return refreshDueMsg{} }
}

// lifecycle builds a model whose clock and timer are deterministic.
func lifecycle(t *testing.T, source Source, at time.Time, timer *recorder, values ...string) Model {
	t.Helper()
	model, err := New(context.Background(), testScope(t, values...), source)
	if err != nil {
		t.Fatalf("building the lifecycle model failed: %v", err)
	}
	model.now = func() time.Time { return at }
	model.tick = timer.tick
	return model
}

// testRepository parses a repository identifier for a scripted result.
func testRepository(t *testing.T, value string) domain.Repository {
	t.Helper()
	repository, err := domain.ParseRepository(value)
	if err != nil {
		t.Fatalf("parsing the test repository %q failed: %v", value, err)
	}
	return repository
}

// activity returns a scripted successful result holding one event per named
// repository.
func activity(t *testing.T, values ...string) Result {
	t.Helper()
	repositories := make([]domain.RepositoryActivity, 0, len(values))
	for index, value := range values {
		repository := testRepository(t, value)
		repositories = append(repositories, domain.RepositoryActivity{
			Repository: repository,
			Events: []domain.Event{{
				ID:          repository.Key() + "-" + strconv.Itoa(index),
				OccurredAt:  time.Date(2026, time.August, 22, 10, index, 0, 0, time.UTC),
				Repository:  repository,
				Category:    domain.CategoryPush,
				EntityKind:  domain.EntityRepository,
				Description: "pushed",
			}},
		})
	}
	return Result{Repositories: repositories}
}

// run executes the command and feeds the resulting message back into the model.
func run(t *testing.T, model Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command to run")
	}
	return apply(t, model, cmd())
}

func TestInitRendersLoadingAndStartsRefreshThroughACommand(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend")

	cmd := model.Init()
	if cmd == nil {
		t.Fatal("Init returned no command, want the first refresh")
	}
	if source.calls != 0 {
		t.Errorf("Init itself called the source %d times, want the command to own the I/O", source.calls)
	}
	if content := model.View().Content; !strings.Contains(content, "LOADING") {
		t.Errorf("initial render does not show LOADING:\n%s", content)
	}

	cmd()
	if source.calls != 1 {
		t.Errorf("running the Init command called the source %d times, want 1", source.calls)
	}
}

func TestShellStaysResponsiveWhileARefreshIsPending(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend")
	model.Init()

	model, _ = apply(t, model, tea.WindowSizeMsg{Width: narrowWidth, Height: narrowHeight}, press("2"))
	if model.mode != ModeStream {
		t.Errorf("mode is %v while a refresh is pending, want ModeStream", model.mode)
	}
	if model.width != narrowWidth || model.height != narrowHeight {
		t.Errorf("dimensions are %dx%d while a refresh is pending, want %dx%d", model.width, model.height, narrowWidth, narrowHeight)
	}
	assertFits(t, model.View().Content, narrowWidth, narrowHeight)

	_, cmd := apply(t, model, press("q"))
	if cmd == nil {
		t.Fatal("quit produced no command while a refresh is pending")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Errorf("quit produced %T while a refresh is pending, want tea.QuitMsg", cmd())
	}
}

func TestCompleteSuccessPublishesTheSnapshotAndRecordsFreshness(t *testing.T) {
	at := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend", "acme/frontend")}}}
	model := lifecycle(t, source, at, &recorder{}, "acme/backend", "acme/frontend")

	model, _ = run(t, model, model.Init())

	if model.state.Freshness != FreshnessCurrent {
		t.Errorf("freshness is %v after a complete success, want FreshnessCurrent", model.state.Freshness)
	}
	if !model.state.LastSuccess.Equal(at) {
		t.Errorf("last success is %v, want %v", model.state.LastSuccess, at)
	}
	if model.state.Cause != "" {
		t.Errorf("cause is %q after a success, want it cleared", model.state.Cause)
	}
	if got := len(model.state.Snapshot.Events()); got != 2 {
		t.Errorf("snapshot holds %d events, want 2", got)
	}
	if got := len(model.state.Snapshot.Aggregates()); got != 2 {
		t.Errorf("snapshot holds %d aggregates, want one per selected repository", got)
	}
}

func TestEmptySuccessStillRecordsOneLastSuccess(t *testing.T) {
	at := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	empty := Result{Repositories: []domain.RepositoryActivity{{Repository: testRepository(t, "acme/backend")}}}
	source := &fakeSource{outcomes: []outcome{{result: empty}}}
	model := lifecycle(t, source, at, &recorder{}, "acme/backend")

	model, _ = run(t, model, model.Init())

	if model.state.Freshness != FreshnessCurrent {
		t.Errorf("freshness is %v after an empty success, want FreshnessCurrent", model.state.Freshness)
	}
	if !model.state.LastSuccess.Equal(at) {
		t.Errorf("last success is %v after an empty success, want %v", model.state.LastSuccess, at)
	}
	if got := len(model.state.Snapshot.Events()); got != 0 {
		t.Errorf("snapshot holds %d events after an empty success, want 0", got)
	}
	if got := len(model.state.Snapshot.Aggregates()); got != 1 {
		t.Errorf("snapshot holds %d aggregates after an empty success, want 1", got)
	}
}

func TestFirstFailureRendersTheErrorStateWithoutASnapshot(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{err: errors.New("refreshing acme/backend: github rate limit reached")}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend")

	model, _ = run(t, model, model.Init())

	if model.state.Freshness != FreshnessError {
		t.Errorf("freshness is %v after the first failure, want FreshnessError", model.state.Freshness)
	}
	if !model.state.LastSuccess.IsZero() {
		t.Errorf("last success is %v after the first failure, want the zero instant", model.state.LastSuccess)
	}
	if want := "refreshing acme/backend: github rate limit reached"; model.state.Cause != want {
		t.Errorf("cause is %q after the first failure, want %q", model.state.Cause, want)
	}
	if got := len(model.state.Snapshot.Events()); got != 0 {
		t.Errorf("snapshot holds %d events after the first failure, want 0", got)
	}
	if content := model.View().Content; !strings.Contains(content, "ERROR") {
		t.Errorf("render does not show ERROR after the first failure:\n%s", content)
	}
}

func TestLaterFailureKeepsTheSnapshotUnderStale(t *testing.T) {
	at := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	source := &fakeSource{outcomes: []outcome{
		{result: activity(t, "acme/backend")},
		{err: errors.New("refreshing acme/backend: github request failed")},
	}}
	model := lifecycle(t, source, at, &recorder{}, "acme/backend")

	model, cmd := run(t, model, model.Init())
	model, cmd = apply(t, model, cmd())
	model, _ = run(t, model, cmd)

	if model.state.Freshness != FreshnessStale {
		t.Errorf("freshness is %v after a later failure, want FreshnessStale", model.state.Freshness)
	}
	if !model.state.LastSuccess.Equal(at) {
		t.Errorf("last success is %v after a later failure, want the preserved %v", model.state.LastSuccess, at)
	}
	if got := len(model.state.Snapshot.Events()); got != 1 {
		t.Errorf("snapshot holds %d events after a later failure, want the preserved 1", got)
	}
	if model.state.Cause == "" {
		t.Error("cause is empty after a later failure, want a concise error")
	}
	if content := model.View().Content; !strings.Contains(content, "STALE") {
		t.Errorf("render does not show STALE after a later failure:\n%s", content)
	}
}

func TestCompleteSuccessAfterAFailureClearsTheErrorState(t *testing.T) {
	at := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	source := &fakeSource{outcomes: []outcome{
		{err: errors.New("refreshing acme/backend: github request failed")},
		{result: activity(t, "acme/backend")},
	}}
	model := lifecycle(t, source, at, &recorder{}, "acme/backend")

	model, cmd := run(t, model, model.Init())
	model, cmd = apply(t, model, cmd())
	model, _ = run(t, model, cmd)

	if model.state.Freshness != FreshnessCurrent {
		t.Errorf("freshness is %v after recovery, want FreshnessCurrent", model.state.Freshness)
	}
	if model.state.Cause != "" {
		t.Errorf("cause is %q after recovery, want it cleared", model.state.Cause)
	}
	if got := len(model.state.Snapshot.Events()); got != 1 {
		t.Errorf("snapshot holds %d events after recovery, want 1", got)
	}
}

func TestFailedRefreshNeverPublishesPartialCandidates(t *testing.T) {
	partial := activity(t, "acme/backend")
	source := &fakeSource{outcomes: []outcome{{
		result: partial,
		err:    errors.New("refreshing acme/frontend: repository not found or inaccessible"),
	}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend", "acme/frontend")

	model, _ = run(t, model, model.Init())

	if got := len(model.state.Snapshot.Events()); got != 0 {
		t.Errorf("snapshot holds %d events after a failed refresh, want no partial candidates", got)
	}
	if got := len(model.state.Snapshot.Aggregates()); got != 0 {
		t.Errorf("snapshot holds %d aggregates after a failed refresh, want none published", got)
	}
}

func TestNextAttemptUsesTheReportedSchedulingMetadata(t *testing.T) {
	cases := []struct {
		name      string
		outcome   outcome
		wantDelay time.Duration
	}{
		{name: "recalculated success interval", outcome: outcome{result: Result{Delay: 90 * time.Second}}, wantDelay: 90 * time.Second},
		{name: "success below the floor", outcome: outcome{result: Result{Delay: 5 * time.Second}}, wantDelay: defaultDelay},
		{name: "success without metadata", outcome: outcome{result: Result{}}, wantDelay: defaultDelay},
		{
			name:      "rate limited failure",
			outcome:   outcome{result: Result{Delay: 15 * time.Minute}, err: errors.New("refreshing acme/backend: github rate limit reached")},
			wantDelay: 15 * time.Minute,
		},
		{
			name:      "ordinary failure",
			outcome:   outcome{result: Result{Delay: defaultDelay}, err: errors.New("refreshing acme/backend: github request failed")},
			wantDelay: defaultDelay,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			timer := &recorder{}
			source := &fakeSource{outcomes: []outcome{testCase.outcome}}
			model := lifecycle(t, source, fixedInstant, timer, "acme/backend")

			run(t, model, model.Init())
			if len(timer.delays) != 1 {
				t.Fatalf("scheduled %d timers after one completed refresh, want 1", len(timer.delays))
			}
			if timer.delays[0] != testCase.wantDelay {
				t.Errorf("scheduled the next attempt after %v, want %v", timer.delays[0], testCase.wantDelay)
			}
		})
	}
}

func TestTheNextTimerStartsOnlyAfterTheRefreshCompletes(t *testing.T) {
	timer := &recorder{}
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model := lifecycle(t, source, fixedInstant, timer, "acme/backend")

	cmd := model.Init()
	model, _ = apply(t, model, tea.WindowSizeMsg{Width: 80, Height: 24}, press("2"))
	if len(timer.delays) != 0 {
		t.Fatalf("scheduled %d timers while the refresh was pending, want none", len(timer.delays))
	}

	run(t, model, cmd)
	if len(timer.delays) != 1 {
		t.Errorf("scheduled %d timers after completion, want 1", len(timer.delays))
	}
}

func TestAtMostOneRefreshIsInFlight(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend")

	cmd := model.Init()
	model, due := apply(t, model, refreshDueMsg{})
	if due != nil {
		t.Fatalf("a due timer started a second refresh while one was in flight: %T", due())
	}

	model, _ = run(t, model, cmd)
	if _, due = apply(t, model, refreshDueMsg{}); due == nil {
		t.Fatal("a due timer started no refresh after the previous one completed")
	}
	due()
	if source.calls != 2 {
		t.Errorf("the source was called %d times, want exactly one call per due timer", source.calls)
	}
}

func TestShutdownCancelsInFlightSourceWork(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend")

	cmd := model.Init()
	_, quit := apply(t, model, press("q"))
	if quit == nil {
		t.Fatal("quit produced no command")
	}
	if _, isQuit := quit().(tea.QuitMsg); !isQuit {
		t.Fatalf("quit produced %T, want tea.QuitMsg", quit())
	}

	cmd()
	if len(source.contexts) != 1 {
		t.Fatalf("the source was started %d times, want 1", len(source.contexts))
	}
	if err := source.contexts[0].Err(); !errors.Is(err, context.Canceled) {
		t.Errorf("the in-flight refresh context reports %v after shutdown, want context.Canceled", err)
	}
}

func TestSchedulingMetadataStaysOutOfTheRenderedState(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{
		result: Result{Delay: 15 * time.Minute},
		err:    errors.New("refreshing acme/backend: github rate limit reached"),
	}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend")

	model, _ = run(t, model, model.Init())

	content := strings.ToLower(model.View().Content)
	for _, unwanted := range []string{"15m", "900", "retry", "delay"} {
		if strings.Contains(content, unwanted) {
			t.Errorf("render leaks the scheduling metadata %q:\n%s", unwanted, content)
		}
	}
}

func TestAPendingRefreshAfterASuccessKeepsTheSnapshotCurrent(t *testing.T) {
	at := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	source := &fakeSource{outcomes: []outcome{{result: activity(t, "acme/backend")}}}
	model := lifecycle(t, source, at, &recorder{}, "acme/backend")

	model, cmd := run(t, model, model.Init())
	model, _ = apply(t, model, cmd())

	if model.state.Freshness != FreshnessCurrent {
		t.Errorf("freshness is %v while the next refresh is pending, want the snapshot to stay current", model.state.Freshness)
	}
	if got := len(model.state.Snapshot.Events()); got != 1 {
		t.Errorf("snapshot holds %d events while the next refresh is pending, want the visible 1", got)
	}
	content := model.View().Content
	for _, marker := range []string{"LOADING", "ERROR", "STALE"} {
		if strings.Contains(content, marker) {
			t.Errorf("render shows the %q marker while a later refresh is pending:\n%s", marker, content)
		}
	}
	if !strings.Contains(content, transportLabel) {
		t.Errorf("render dropped the %q transport label while a later refresh is pending:\n%s", transportLabel, content)
	}
}

func TestTheReportedCauseDropsTerminalControlSequences(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{
		err: errors.New("refreshing acme/backend: \x1b[31;1mboom\x1b[0m\x07 status\x7f 500"),
	}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend")

	model, _ = run(t, model, model.Init())

	for _, character := range model.state.Cause {
		if !unicode.IsPrint(character) {
			t.Errorf("cause %q retains the non-printable rune %U", model.state.Cause, character)
		}
	}
	if strings.Contains(model.View().Content, "\x1b[31;1m") {
		t.Errorf("render relays a terminal control sequence:\n%q", model.View().Content)
	}
}

func TestTheReportedCauseIsCollapsedToOneHeaderLine(t *testing.T) {
	source := &fakeSource{outcomes: []outcome{{err: errors.New("refreshing acme/backend:\n\tunexpected github response: status 500")}}}
	model := lifecycle(t, source, fixedInstant, &recorder{}, "acme/backend")

	model, _ = run(t, model, model.Init())

	if want := "refreshing acme/backend: unexpected github response: status 500"; model.state.Cause != want {
		t.Errorf("cause is %q, want %q", model.state.Cause, want)
	}
}
