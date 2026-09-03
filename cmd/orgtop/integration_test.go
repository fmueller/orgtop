package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/fmueller/orgtop/internal/cli"
	"github.com/fmueller/orgtop/internal/github"
	"github.com/fmueller/orgtop/internal/tui"
)

// These flows exercise the boundaries the binary actually wires together:
// argument parsing, the GitHub source over a fixture endpoint, the normalized
// domain, and the Bubble Tea shell. They use synthetic activity and a resolved
// sentinel credential, so no live GitHub account or token is involved.

// The narrowest terminal the shell contracts for (A-010), and a terminal wide
// enough for the richest layouts.
const (
	narrowWidth  = 40
	narrowHeight = 10
	wideWidth    = 120
	wideHeight   = 30
)

// narrowBodyHeight is the row budget the body has at that narrowest terminal,
// once the shared header and footer take one line each.
const narrowBodyHeight = narrowHeight - 2

// streamChromeLines is the further content Stream reserves for its own chrome:
// the coverage disclosure and the sticky column headings (FR-007), so its
// event-row budget is two short of Overview's.
const streamChromeLines = 2

// streamRowBudget is the event-row budget Stream has at that narrowest terminal.
const streamRowBudget = narrowBodyHeight - streamChromeLines

// scrollRepositories selects two more repositories than that budget can show at
// once, so the last one is only reachable by scrolling.
const scrollRepositories = narrowBodyHeight + 2

// scrollStreamEvents is two more events than Stream can show at once, so the
// oldest is only reachable by scrolling.
const scrollStreamEvents = streamRowBudget + 2

// cannedResponse is one scripted repository response.
type cannedResponse struct {
	status int
	body   string
}

// eventsEndpoint serves the bounded events endpoint from a per-repository queue
// of canned responses. The last queued response repeats, so a flow that only
// varies its first refresh needs a single entry.
type eventsEndpoint struct {
	mu sync.Mutex
	// script maps a request path to the responses it serves in order.
	script map[string][]cannedResponse
	// served counts the requests each path has answered.
	served map[string]int
	// authorized records that a request arrived with the resolved credential,
	// so the secrecy assertions are about a token that was really sent.
	authorized bool
	// gate blocks every response until it is closed, when non-nil.
	gate chan struct{}
	// reached reports every request that started, so a flow can wait for one.
	reached chan struct{}
}

// newEndpoint returns an endpoint serving the scripted responses.
func newEndpoint(script map[string][]cannedResponse) *eventsEndpoint {
	return &eventsEndpoint{
		script:  script,
		served:  make(map[string]int),
		reached: make(chan struct{}, 16),
	}
}

// ServeHTTP answers the request with the next response scripted for its path.
func (e *eventsEndpoint) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	e.mu.Lock()
	queue := e.script[request.URL.Path]
	index := min(e.served[request.URL.Path], len(queue)-1)
	e.served[request.URL.Path]++
	if request.Header.Get("Authorization") == "Bearer "+sentinelToken {
		e.authorized = true
	}
	gate := e.gate
	e.mu.Unlock()

	select {
	case e.reached <- struct{}{}:
	default:
	}

	if gate != nil {
		select {
		case <-gate:
		case <-request.Context().Done():
			return
		}
	}

	if index < 0 {
		http.Error(writer, "no scripted response", http.StatusNotFound)
		return
	}
	response := queue[index]
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(response.status)
	_, _ = writer.Write([]byte(response.body))
}

// sawCredential reports whether the endpoint authenticated a request with the
// resolved credential.
func (e *eventsEndpoint) sawCredential() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.authorized
}

// awaitRequest fails unless a request reaches the endpoint before the timeout.
// what names the flow that was expected to start it.
func (e *eventsEndpoint) awaitRequest(t *testing.T, what string) {
	t.Helper()

	select {
	case <-e.reached:
	case <-time.After(launchTimeout):
		t.Fatalf("%s never reached the source", what)
	}
}

// requestCount reports how many requests a repository path answered.
func (e *eventsEndpoint) requestCount(repository string) int {
	return e.servedCount(eventsPath(repository))
}

// servedCount reports how many requests a path answered.
func (e *eventsEndpoint) servedCount(path string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.served[path]
}

// eventsPath is the bounded events path the source requests for a repository.
func eventsPath(repository string) string { return "/repos/" + repository + "/events" }

// scrollRepository names the selected repository at a zero-based position. The
// fixed-width suffix keeps every identity distinct as a substring.
func scrollRepository(index int) string { return fmt.Sprintf("acme/r%02d", index) }

// ok queues a successful page.
func ok(body string) cannedResponse { return cannedResponse{status: http.StatusOK, body: body} }

// serverError queues the response class that fails a refresh without naming a
// credential.
func serverError() cannedResponse {
	return cannedResponse{status: http.StatusInternalServerError, body: `{"message":"upstream is unavailable"}`}
}

// pushEvent renders one synthetic push event for the repository, identified by
// the id its page gives it and occurring at the given instant.
func pushEvent(repository, id string, occurred time.Time) map[string]any {
	return map[string]any{
		"id":         id,
		"type":       "PushEvent",
		"actor":      map[string]any{"login": "alice"},
		"repo":       map[string]any{"name": repository},
		"payload":    map[string]any{"size": 1, "ref": "refs/heads/main", "head": "cafe"},
		"created_at": occurred.Format(time.RFC3339),
	}
}

// wiredInstant is the instant every wired flow pins its shell clock to. The
// shell anchors rendered ages to its own last-success instant, so pinning it
// makes every age a fixture states an age the render must show, rather than a
// value that drifts with how long the test runs (NFR-006).
var wiredInstant = time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)

// eventsPage renders a synthetic events page for the repository: count push
// events, newest first, each naming its own one-based position.
func eventsPage(repository string, count int) string {
	page := make([]map[string]any, 0, count)
	for index := range count {
		id := fmt.Sprintf("%s-%03d", strings.ReplaceAll(repository, "/", "-"), index)
		page = append(page, pushEvent(repository, id, wiredInstant.Add(-time.Duration(index)*time.Minute)))
	}
	return encode(page)
}

// agedEventsPage renders a synthetic events page whose push events occurred the
// given durations before the pinned instant, newest first. Every flow pins its
// shell clock to wiredInstant, so an offset from it is exactly the age the
// shell renders, on every run and however long the test takes (NFR-006).
func agedEventsPage(repository string, ages ...time.Duration) string {
	page := make([]map[string]any, 0, len(ages))
	for index, age := range ages {
		id := fmt.Sprintf("%s-aged-%03d", strings.ReplaceAll(repository, "/", "-"), index)
		page = append(page, pushEvent(repository, id, wiredInstant.Add(-age)))
	}
	return encode(page)
}

// pullRequestPage renders one opened pull request event for the repository.
func pullRequestPage(repository string) string {
	return encode([]map[string]any{{
		"id":         repository + "-pr",
		"type":       "PullRequestEvent",
		"actor":      map[string]any{"login": "bob"},
		"repo":       map[string]any{"name": repository},
		"payload":    map[string]any{"action": "opened", "pull_request": map[string]any{"number": 7}},
		"created_at": "2026-08-22T11:30:00Z",
	}})
}

// emptyPage is a completely successful refresh that returned no activity.
const emptyPage = "[]"

// encode renders a synthetic page as the JSON body the source parses.
func encode(page []map[string]any) string {
	encoded, err := json.Marshal(page)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// serveEndpoint starts the endpoint and returns the source adapter the binary
// wires around it: the real GitHub source, pointed at the test server and
// authenticated with the resolved sentinel credential.
func serveEndpoint(t *testing.T, endpoint *eventsEndpoint) sourceAdapter {
	t.Helper()

	server := httptest.NewServer(endpoint)
	t.Cleanup(server.Close)
	return sourceAdapter{source: github.Source{
		Client:     server.Client(),
		BaseURL:    server.URL,
		Credential: sentinelCredential(t),
	}}
}

// flow drives the wired shell deterministically. It runs the commands the model
// returns rather than starting a Bubble Tea program, so every step is ordered.
type flow struct {
	t     *testing.T
	model tui.Model
	// adapter is the same wired source the shell refreshes through, kept so a
	// flow can also observe the failure text the shell never renders in full.
	adapter sourceAdapter
}

// newFlow wires args through argument parsing, the resolved sentinel
// credential, the GitHub source, and the shell, exactly as the binary does.
func newFlow(t *testing.T, endpoint *eventsEndpoint, args ...string) *flow {
	t.Helper()

	config, err := cli.ParseArgs("orgtop", args, &strings.Builder{})
	if err != nil {
		t.Fatalf("parsing %v failed: %v", args, err)
	}

	adapter := serveEndpoint(t, endpoint)
	options := []tui.Option{tui.WithClock(func() time.Time { return wiredInstant })}
	if len(config.Organizations) > 0 {
		// The binary expands organization selectors through the same source, so
		// a wired flow binds the expansion the same way launch does.
		options = append(options, tui.WithExpander(expansionAdapter{
			source:  adapter.source,
			request: expansionRequest(config),
		}))
	}
	model, err := tui.New(t.Context(), config.Scopes, adapter, options...)
	if err != nil {
		t.Fatalf("building the shell for %v failed: %v", args, err)
	}
	return &flow{
		t:       t,
		model:   model,
		adapter: adapter,
	}
}

// apply drives messages through the shell.
func (f *flow) apply(messages ...tea.Msg) {
	f.t.Helper()
	for _, message := range messages {
		next, _ := f.model.Update(message)
		model, isModel := next.(tui.Model)
		if !isModel {
			f.t.Fatalf("Update returned %T, want tui.Model", next)
		}
		f.model = model
	}
}

// refresh performs one complete refresh through the wired source. Init is the
// exported command that starts a refresh; the scheduled attempts the shell
// drives itself are a package-internal message with a polling-floor delay, so
// an integration flow starts each later attempt the same way.
func (f *flow) refresh() {
	f.t.Helper()
	cmd := f.model.Init()
	if cmd == nil {
		f.t.Fatal("the shell started no refresh")
	}
	f.apply(cmd())
}

// size reports a terminal size to the shell.
func (f *flow) size(width, height int) {
	f.t.Helper()
	f.apply(tea.WindowSizeMsg{Width: width, Height: height})
}

// press drives one keystroke.
func (f *flow) press(keystroke string) {
	f.t.Helper()
	if keystroke == "tab" {
		f.apply(tea.KeyPressMsg{Code: tea.KeyTab})
		return
	}
	f.apply(tea.KeyPressMsg{Code: []rune(keystroke)[0], Text: keystroke})
}

// render returns what the shell renders at the size.
func (f *flow) render(width, height int) string {
	f.t.Helper()
	f.size(width, height)
	return f.model.View().Content
}

// body returns the non-empty content lines between the shared chrome.
func body(t *testing.T, content string) []string {
	t.Helper()
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		t.Fatalf("render has no body:\n%s", content)
	}
	kept := make([]string, 0, len(lines)-2)
	for _, line := range lines[1 : len(lines)-1] {
		if strings.TrimSpace(line) != "" {
			kept = append(kept, line)
		}
	}
	return kept
}

// streamBody returns the event rows of a Stream render: the body lines below the
// coverage disclosure and the sticky column headings Stream keeps above them at
// every scroll position (FR-010). It fails when the headings are missing, so a
// row assertion can never silently be made against Stream's own chrome.
func streamBody(t *testing.T, content string) []string {
	t.Helper()
	lines := body(t, content)
	for index, line := range lines {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == "age" {
			return lines[index+1:]
		}
	}
	t.Fatalf("no stream body line names the sticky column headings:\n%s", content)
	return nil
}

// assertContains fails unless the content names every want.
func assertContains(t *testing.T, content string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(content, want) {
			t.Errorf("output does not contain %q:\n%s", want, content)
		}
	}
}

// assertAbsent fails when the content names any unwanted string.
func assertAbsent(t *testing.T, content string, unwanted ...string) {
	t.Helper()
	for _, absent := range unwanted {
		if strings.Contains(content, absent) {
			t.Errorf("output unexpectedly contains %q:\n%s", absent, content)
		}
	}
}

func TestOneRepositoryFlowRendersOnlyTheSelectedRepository(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(eventsPage("acme/backend", 2))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend")
	run.refresh()

	overview := run.render(wideWidth, wideHeight)
	assertContains(t, overview, "acme/backend", "2 activity", "2 pushes")
	assertAbsent(t, overview, "acme/frontend", "ERROR", "STALE")

	run.press("2")
	stream := run.model.View().Content
	// The body also carries Stream's sticky heading row, so the event rows are
	// counted below it.
	if rows := streamBody(t, stream); len(rows) != 2 {
		t.Errorf("stream rendered %d event rows, want 2:\n%v", len(rows), rows)
	}
	assertContains(t, stream, "acme/backend", "alice", "push")

	if !endpoint.sawCredential() {
		t.Error("the wired source never authenticated with the resolved credential")
	}
}

func TestMultipleRepositoryFlowRendersEverySelectedRepository(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"):  {ok(eventsPage("acme/backend", 2))},
		eventsPath("acme/frontend"): {ok(pullRequestPage("acme/frontend"))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend", "--repo", "acme/frontend")
	run.refresh()

	overview := run.render(wideWidth, wideHeight)
	rows := body(t, overview)
	if len(rows) != 2 {
		t.Fatalf("overview rendered %d rows, want one per selected repository:\n%v", len(rows), rows)
	}
	assertContains(t, rows[0], "acme/backend", "2 activity", "2 pushes")
	assertContains(t, rows[1], "acme/frontend", "1 activity", "1 pull request")

	for _, repository := range []string{"acme/backend", "acme/frontend"} {
		if count := endpoint.requestCount(repository); count != 1 {
			t.Errorf("%s answered %d requests, want exactly one per refresh", repository, count)
		}
	}
}

func TestAnUnselectedRepositoryNeverReachesTheRenderedActivity(t *testing.T) {
	// A selected endpoint that returns another repository's activity fails the
	// refresh at the source boundary rather than becoming a candidate (A-002).
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(eventsPage("acme/other", 1))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend")
	run.refresh()

	content := run.render(wideWidth, wideHeight)
	assertContains(t, content, "ERROR")
	assertAbsent(t, content, "acme/other")
}

func TestViewNavigationPreservesTheLoadedSnapshotAndScrollPosition(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(eventsPage("acme/backend", 20))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend")
	run.refresh()
	run.size(narrowWidth, narrowHeight)

	assertContains(t, run.model.View().Content, "OVERVIEW", "acme/backend")

	run.press("2")
	assertContains(t, run.model.View().Content, "STREAM")
	run.press("down")
	run.press("down")
	scrolled := body(t, run.model.View().Content)

	run.press("1")
	assertContains(t, run.model.View().Content, "OVERVIEW", "acme/backend")
	run.press("tab")
	restored := body(t, run.model.View().Content)

	if len(scrolled) == 0 || len(restored) == 0 {
		t.Fatalf("stream rendered no rows before (%d) or after (%d) the switch", len(scrolled), len(restored))
	}
	if scrolled[0] != restored[0] {
		t.Errorf("stream resumed at %q, want the pre-switch position %q", restored[0], scrolled[0])
	}
	if endpoint.requestCount("acme/backend") != 1 {
		t.Errorf("view switching triggered %d requests, want the loaded snapshot to be reused", endpoint.requestCount("acme/backend"))
	}
}

// TestOverviewScrollsToEverySelectedRepositoryAtTheNarrowestSize proves the
// reachability contract end to end: at 40x10 a Scope that overflows the body
// still lets an operator reach its last repository, through the same wired
// binary path a real launch takes (FR-009, A-010).
func TestOverviewScrollsToEverySelectedRepositoryAtTheNarrowestSize(t *testing.T) {
	script := make(map[string][]cannedResponse, scrollRepositories)
	args := make([]string, 0, 2*scrollRepositories)
	for index := range scrollRepositories {
		repository := scrollRepository(index)
		script[eventsPath(repository)] = []cannedResponse{ok(eventsPage(repository, 1))}
		args = append(args, "--repo", repository)
	}

	run := newFlow(t, newEndpoint(script), args...)
	run.refresh()

	first, last := scrollRepository(0), scrollRepository(scrollRepositories-1)
	top := run.render(narrowWidth, narrowHeight)
	assertContains(t, top, "OVERVIEW", first)
	assertAbsent(t, top, last)
	if rows := body(t, top); len(rows) != narrowBodyHeight {
		t.Errorf("overview rendered %d rows at %dx%d, want the full body budget of %d:\n%s",
			len(rows), narrowWidth, narrowHeight, narrowBodyHeight, top)
	}

	run.press("pgdown")
	bottom := run.model.View().Content
	assertContains(t, bottom, "OVERVIEW", last)
	assertAbsent(t, bottom, first)

	// A page past the end stays on the last row rather than scrolling into
	// blank space, so the window never leaves the content.
	run.press("pgdown")
	if clamped := run.model.View().Content; clamped != bottom {
		t.Errorf("paging past the last row moved the window:\n%s\nwant:\n%s", clamped, bottom)
	}

	run.press("pgup")
	assertContains(t, run.model.View().Content, first)
}

func TestTheShellStaysInteractiveWhileARefreshIsPending(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(eventsPage("acme/backend", 1))},
	})
	endpoint.gate = make(chan struct{})
	run := newFlow(t, endpoint, "--repo", "acme/backend")

	pending := make(chan tea.Msg, 1)
	cmd := run.model.Init()
	go func() { pending <- cmd() }()

	endpoint.awaitRequest(t, "the pending refresh")

	// The shell answers navigation, resize, and scrolling while the request is
	// still open, which is what keeps the update loop off the I/O path (A-006).
	run.size(wideWidth, wideHeight)
	run.press("2")
	assertContains(t, run.model.View().Content, "STREAM", "LOADING")
	run.press("1")
	assertContains(t, run.model.View().Content, "OVERVIEW", "LOADING")

	close(endpoint.gate)
	select {
	case message := <-pending:
		run.apply(message)
	case <-time.After(launchTimeout):
		t.Fatal("the pending refresh never completed")
	}
	assertContains(t, run.model.View().Content, "acme/backend", "1 activity")
	assertAbsent(t, run.model.View().Content, "LOADING")
}

func TestEmptySuccessRecordsARefreshRatherThanAnError(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"):  {ok(emptyPage)},
		eventsPath("acme/frontend"): {ok(emptyPage)},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend", "--repo", "acme/frontend")
	run.refresh()

	overview := run.render(wideWidth, wideHeight)
	assertContains(t, overview, "acme/backend", "acme/frontend", "No activity", "updated ")
	assertAbsent(t, overview, "ERROR", "STALE", "LOADING")

	run.press("2")
	assertContains(t, run.model.View().Content, "No recent activity")
}

func TestInitialFailureStaysInteractiveAndRecoversOnTheNextSuccess(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {serverError(), ok(eventsPage("acme/backend", 3))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend")
	run.refresh()

	failed := run.render(wideWidth, wideHeight)
	assertContains(t, failed, "ERROR", "unexpected github response")
	assertAbsent(t, failed, "STALE", "updated ")

	// The shell is still navigable under the error state.
	run.press("2")
	assertContains(t, run.model.View().Content, "STREAM", "ERROR")
	run.press("1")

	run.refresh()
	recovered := run.model.View().Content
	assertContains(t, recovered, "acme/backend", "3 activity", "updated ")
	assertAbsent(t, recovered, "ERROR", "STALE")
}

func TestAPartialFailureKeepsTheSnapshotStaleAndRecovers(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(eventsPage("acme/backend", 2))},
		eventsPath("acme/frontend"): {
			ok(eventsPage("acme/frontend", 1)),
			serverError(),
			ok(eventsPage("acme/frontend", 4)),
		},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend", "--repo", "acme/frontend")
	run.refresh()

	current := run.render(wideWidth, wideHeight)
	assertContains(t, current, "acme/frontend", "1 activity")
	assertAbsent(t, current, "STALE")

	run.refresh()
	stale := run.model.View().Content
	assertContains(t, stale, "STALE", "acme/backend", "2 activity", "acme/frontend", "1 activity", "updated ")
	assertAbsent(t, stale, "4 activity")

	run.refresh()
	recovered := run.model.View().Content
	assertContains(t, recovered, "acme/frontend", "4 activity")
	assertAbsent(t, recovered, "STALE", "ERROR")
}

func TestBothCompletedViewsRetainRequiredContextAtTheNarrowestSize(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"):  {ok(eventsPage("acme/backend", 6))},
		eventsPath("acme/frontend"): {ok(pullRequestPage("acme/frontend"))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend", "--repo", "acme/frontend")
	run.refresh()

	for _, view := range []struct {
		name      string
		keystroke string
		label     string
		// rows returns the primary content lines of the view. Stream carries
		// its sticky column headings above them, and A-010 keeps the headings
		// subordinate to that content, so its rows are counted below them.
		rows func(*testing.T, string) []string
	}{
		{name: "overview", keystroke: "1", label: "OVERVIEW", rows: body},
		{name: "stream", keystroke: "2", label: "STREAM", rows: streamBody},
	} {
		t.Run(view.name, func(t *testing.T) {
			run.press(view.keystroke)
			content := run.render(narrowWidth, narrowHeight)

			lines := strings.Split(content, "\n")
			if len(lines) > narrowHeight {
				t.Errorf("%s used %d lines, want at most %d:\n%s", view.name, len(lines), narrowHeight, content)
			}
			assertContains(t, content, view.label, "POLLING", "q quit")
			if rows := view.rows(t, content); len(rows) == 0 {
				t.Errorf("%s kept no primary content line:\n%s", view.name, content)
			}
		})
	}
}

// TestStreamScrollsToEveryEventBelowItsStickyHeadingsAtTheNarrowestSize proves
// the Stream half of the reachability contract end to end: at 40x10 a snapshot
// that overflows the content area still lets an operator reach its oldest event,
// while the column headings naming that content stay on screen at every scroll
// position (FR-010, A-010).
func TestStreamScrollsToEveryEventBelowItsStickyHeadingsAtTheNarrowestSize(t *testing.T) {
	// One event per whole minute, so every row carries a distinct rendered age
	// and the window can be identified by the ages it shows.
	ages := make([]time.Duration, 0, scrollStreamEvents)
	for index := range scrollStreamEvents {
		ages = append(ages, time.Duration(index+1)*time.Minute)
	}
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(agedEventsPage("acme/backend", ages...))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend")
	run.refresh()
	run.press("2")

	newest, oldest := "1m", fmt.Sprintf("%dm", scrollStreamEvents)
	top := run.render(narrowWidth, narrowHeight)
	assertContains(t, top, "STREAM", "POLLING", "q quit")
	rows := streamBody(t, top)
	if len(rows) != streamRowBudget {
		t.Fatalf("stream rendered %d event rows at %dx%d, want the row budget of %d:\n%s",
			len(rows), narrowWidth, narrowHeight, streamRowBudget, top)
	}
	assertContains(t, rows[0], newest)
	assertAbsent(t, top, oldest)

	run.press("pgdown")
	bottom := run.model.View().Content
	scrolledRows := streamBody(t, bottom)
	if len(scrolledRows) != streamRowBudget {
		t.Fatalf("the scrolled stream rendered %d event rows, want the row budget of %d:\n%s",
			len(scrolledRows), streamRowBudget, bottom)
	}
	assertContains(t, scrolledRows[len(scrolledRows)-1], oldest)
	assertAbsent(t, bottom, " "+newest+" ")

	// A page past the oldest event stays on it rather than scrolling into blank
	// space, so the window never leaves the content.
	run.press("pgdown")
	if clamped := run.model.View().Content; clamped != bottom {
		t.Errorf("paging past the oldest event moved the window:\n%s\nwant:\n%s", clamped, bottom)
	}

	run.press("pgup")
	assertContains(t, streamBody(t, run.model.View().Content)[0], newest)
}

func TestQuitEndsTheWiredProgramAndCancelsTheInFlightRequest(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(eventsPage("acme/backend", 1))},
	})
	endpoint.gate = make(chan struct{})
	defer close(endpoint.gate)
	adapter := serveEndpoint(t, endpoint)

	input, keystrokes := io.Pipe()
	defer func() { _ = input.Close() }()

	exit := make(chan error, 1)
	go func() {
		exit <- launchProgram(context.Background(), mustScope(t, "acme/backend"), adapter, nil, nil, headless(input)...)
	}()

	endpoint.awaitRequest(t, "the launch")
	if _, err := keystrokes.Write([]byte("q")); err != nil {
		t.Fatalf("writing the quit keystroke failed: %v", err)
	}

	select {
	case err := <-exit:
		if err != nil {
			t.Errorf("launchProgram returned %v, want a clean exit", err)
		}
	case <-time.After(launchTimeout):
		t.Fatal("the quit keystroke did not end the wired program")
	}
}

func TestShutdownCancelsTheInFlightWiredRequest(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(eventsPage("acme/backend", 1))},
	})
	endpoint.gate = make(chan struct{})
	defer close(endpoint.gate)
	adapter := serveEndpoint(t, endpoint)

	ctx, cancel := context.WithCancel(context.Background())
	refreshed := make(chan error, 1)
	go func() {
		_, err := adapter.Refresh(ctx, mustScope(t, "acme/backend"))
		refreshed <- err
	}()

	endpoint.awaitRequest(t, "the refresh")
	cancel()

	select {
	case err := <-refreshed:
		if err == nil {
			t.Fatal("the canceled refresh reported no failure")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("the canceled refresh reported %v, want a context cancellation", err)
		}
	case <-time.After(launchTimeout):
		t.Fatal("shutdown left the in-flight source request running")
	}
}

func TestNoWiredFlowEverReportsTheCredential(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"):  {ok(eventsPage("acme/backend", 2)), serverError()},
		eventsPath("acme/frontend"): {{status: http.StatusUnauthorized, body: `{"message":"Bad credentials"}`}},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend", "--repo", "acme/frontend")

	captured := &strings.Builder{}
	for range 2 {
		run.refresh()
		for _, keystroke := range []string{"1", "2", "tab"} {
			run.press(keystroke)
			captured.WriteString(run.render(wideWidth, wideHeight))
			captured.WriteString(run.render(narrowWidth, narrowHeight))
		}
	}
	// The adapter's own failure carries the text the shell never renders in
	// full, so the secrecy assertions cover that too.
	if _, err := run.adapter.Refresh(t.Context(), mustScope(t, "acme/frontend")); err != nil {
		captured.WriteString(err.Error())
	}

	if !endpoint.sawCredential() {
		t.Fatal("no request authenticated with the resolved credential, so this proves nothing")
	}
	if strings.Contains(captured.String(), sentinelToken) {
		t.Error("a wired flow reported the credential value in its rendered output or errors")
	}
	if !strings.Contains(captured.String(), "ERROR") {
		t.Errorf("the flows never reached a failure state, so the failure copy is unchecked:\n%s", captured.String())
	}
}

func TestStartupFailsConciselyWhenNoLocalCredentialExists(t *testing.T) {
	// The real resolver, not a stub: an empty environment and a gh invocation
	// that reports no token is the documented "no credential" state (A-004).
	harness := &harness{}
	shell := harness.shell()
	shell.resolve = realResolver(t, "").Resolve

	code := shell.run(context.Background(), "orgtop", []string{"--repo", "acme/backend"})
	if code != exitFailure {
		t.Errorf("run exit code = %d, want %d", code, exitFailure)
	}
	if harness.launched.called {
		t.Error("a failed credential resolution still launched the terminal ui")
	}

	reported := strings.TrimSpace(harness.output.String())
	if lines := strings.Split(reported, "\n"); len(lines) != 1 {
		t.Errorf("startup reported %d lines, want one concise cause:\n%s", len(lines), reported)
	}
	assertContains(t, reported, "gh auth login")
	assertAbsent(t, reported, sentinelToken, "exit status", "executable file not found")
}

func TestStartupPrefersTheEnvironmentCredentialOverTheGitHubCLI(t *testing.T) {
	harness := &harness{}
	shell := harness.shell()
	shell.resolve = realResolver(t, sentinelToken).Resolve

	if code := shell.run(context.Background(), "orgtop", []string{"--repo", "acme/backend"}); code != exitSuccess {
		t.Errorf("run exit code = %d, want %d", code, exitSuccess)
	}
	if !harness.launched.called {
		t.Fatal("a resolved credential did not launch the terminal ui")
	}
	if harness.launched.token != sentinelToken {
		t.Error("the launch did not receive the resolved credential")
	}
	assertAbsent(t, harness.output.String(), sentinelToken)
}

// TestMultiDaySnapshotRendersAgesRatherThanClockTimes guards FR-010 through the
// wired binary: a snapshot page reaches back weeks, so Stream must place each
// event by its age at the last successful refresh instead of by a time of day
// that cannot distinguish this afternoon from one three weeks ago.
func TestMultiDaySnapshotRendersAgesRatherThanClockTimes(t *testing.T) {
	const day = 24 * time.Hour
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(agedEventsPage("acme/backend", 2*time.Hour, 3*day, 20*day))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend")
	run.refresh()
	run.press("2")

	stream := run.render(wideWidth, wideHeight)
	rows := streamBody(t, stream)
	if len(rows) != 3 {
		t.Fatalf("stream rendered %d event rows, want 3:\n%v", len(rows), rows)
	}

	clock := regexp.MustCompile(`\d{1,2}:\d{2}`)
	for index, want := range []string{"2h", "3d", "2w"} {
		if age := strings.Fields(rows[index])[0]; age != want {
			t.Errorf("row %d is aged %q, want %q:\n%s", index, age, want, rows[index])
		}
		if clock.MatchString(rows[index]) {
			t.Errorf("row %d spells a wall-clock time of day:\n%s", index, rows[index])
		}
	}
}

// TestPinnedClockRendersTheSameAgesAcrossRefreshes proves the injected clock
// reaches the rendered ages: the shell re-stamps its last-success instant on
// every refresh, so with the clock pinned two refreshes separated by real
// elapsed time must render byte-identical ages, each one exactly the offset its
// fixture states (NFR-006).
func TestPinnedClockRendersTheSameAgesAcrossRefreshes(t *testing.T) {
	const day = 24 * time.Hour
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(agedEventsPage("acme/backend", 90*time.Minute, 3*day))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend")
	run.refresh()
	run.press("2")

	first := ages(t, run.render(wideWidth, wideHeight))
	if want := []string{"1h", "3d"}; !slices.Equal(first, want) {
		t.Fatalf("the first render is aged %v, want %v", first, want)
	}

	// The second refresh is a real round trip through the test server, so real
	// time has passed between the two renders the ages are compared across.
	run.refresh()
	if second := ages(t, run.render(wideWidth, wideHeight)); !slices.Equal(second, first) {
		t.Errorf("a later refresh is aged %v, want the pinned %v", second, first)
	}
	if endpoint.requestCount("acme/backend") != 2 {
		t.Fatalf("the flow performed %d refreshes, want 2", endpoint.requestCount("acme/backend"))
	}
}

// ages returns the age column of every event row a Stream render carries.
func ages(t *testing.T, content string) []string {
	t.Helper()
	rows := streamBody(t, content)
	kept := make([]string, 0, len(rows))
	for _, row := range rows {
		kept = append(kept, strings.Fields(row)[0])
	}
	return kept
}

// shortenedMark is the mark the shared chrome appends to content it shortened.
// It is duplicated here rather than exported, so a wired flow asserts the mark an
// operator sees rather than whatever internal/tui currently calls it.
const shortenedMark = "…"

// narrowStreamAges are the rendered ages the fixture below must produce at the
// narrowest size, spanning minutes to weeks so no single unit could have carried
// the whole span.
var narrowStreamAges = []string{"5m", "2h", "3d", "2w"}

// TestStreamKeepsEveryLegibilityAffordanceAtTheNarrowestSize covers the whole
// post-T-020 Stream surface through the wired binary at the size the contract is
// narrowest for: a snapshot spanning minutes to weeks has to render its ages,
// its sticky column headings, its coverage disclosure, and a shortening mark on
// whatever no longer fits, all inside the 40x10 budget (FR-007, FR-010, A-010).
// The pieces are asserted together because each one costs width or a chrome
// line, so proving them one at a time cannot show they coexist.
func TestStreamKeepsEveryLegibilityAffordanceAtTheNarrowestSize(t *testing.T) {
	const day = 24 * time.Hour
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath("acme/backend"): {ok(agedEventsPage("acme/backend", 5*time.Minute, 2*time.Hour, 3*day, 20*day))},
	})
	run := newFlow(t, endpoint, "--repo", "acme/backend")
	run.refresh()
	run.press("2")

	content := run.render(narrowWidth, narrowHeight)
	lines := strings.Split(content, "\n")
	if len(lines) > narrowHeight {
		t.Errorf("stream used %d lines, want at most %d:\n%s", len(lines), narrowHeight, content)
	}
	for index, line := range lines {
		if rendered := lipgloss.Width(line); rendered > narrowWidth {
			t.Errorf("line %d is %d columns wide, want at most %d:\n%s", index, rendered, narrowWidth, content)
		}
	}

	// The shared chrome and Stream's own two lines, above the event rows.
	assertContains(t, content, "STREAM", "q quit", fmt.Sprintf("showing %d events", len(narrowStreamAges)))
	if rendered := ages(t, content); !slices.Equal(rendered, narrowStreamAges) {
		t.Fatalf("stream is aged %v, want %v:\n%s", rendered, narrowStreamAges, content)
	}

	// The detail column cannot fit at this width, so every row has to say it was
	// shortened rather than end mid-word without a mark.
	for index, row := range streamBody(t, content) {
		if !strings.HasSuffix(strings.TrimRight(row, " "), shortenedMark) {
			t.Errorf("row %d was shortened without the mark %q:\n%s", index, shortenedMark, row)
		}
	}
}

// organizationPath is the listing path expansion requests for a selector.
func organizationPath(organization string) string { return "/orgs/" + organization + "/repos" }

// listingRecord renders one organization listing record with its eligibility
// facts.
func listingRecord(repository string, archived, fork bool) map[string]any {
	owner, name, _ := strings.Cut(repository, "/")
	return map[string]any{
		"full_name": repository,
		"name":      name,
		"owner":     map[string]any{"login": owner, "type": "Organization"},
		"archived":  archived,
		"disabled":  false,
		"fork":      fork,
	}
}

// listingPage renders one organization listing page.
func listingPage(records ...map[string]any) string { return encode(records) }

func TestOrganizationFlowExpandsBeforeItPollsAndDisclosesTheSelection(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		organizationPath("acme"): {ok(listingPage(
			listingRecord("acme/backend", false, false),
			listingRecord("acme/legacy", true, false),
			listingRecord("acme/mirror", false, true),
		))},
		eventsPath("acme/backend"): {ok(eventsPage("acme/backend", 2))},
	})
	run := newFlow(t, endpoint, "--org", "acme")
	run.refresh()

	overview := run.render(wideWidth, wideHeight)
	assertContains(t, overview, "acme/backend", "2 activity", "selection: 1 repos · 1 scopes · 0 exact · 1 expanded")
	// The excluded repositories are never polled and never rendered, and a
	// bounded expansion is not presented as the whole organization.
	assertAbsent(t, overview, "acme/legacy", "acme/mirror", "ERROR", "STALE", sentinelToken)

	if got := endpoint.requestCount("acme/legacy"); got != 0 {
		t.Errorf("the excluded repository answered %d requests, want none", got)
	}
	if got := endpoint.servedCount(organizationPath("acme")); got != 1 {
		t.Errorf("the expansion spent %d listing requests, want 1", got)
	}
	if !endpoint.sawCredential() {
		t.Error("the wired expansion reached the endpoint without the resolved credential")
	}

	// A refresh before the next expansion is due reuses the fixed selection and
	// spends no listing request.
	run.refresh()
	if got := endpoint.servedCount(organizationPath("acme")); got != 1 {
		t.Errorf("the second refresh spent %d listing requests in total, want the fixed selection reused", got)
	}
	if got := endpoint.requestCount("acme/backend"); got != 2 {
		t.Errorf("the second refresh polled the selection %d times in total, want 2", got)
	}
}

func TestOrganizationFlowWithNoEligibleRepositoryPollsNothing(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		organizationPath("acme"): {ok(listingPage(listingRecord("acme/legacy", true, false)))},
	})
	run := newFlow(t, endpoint, "--org", "acme")
	run.refresh()

	overview := run.render(wideWidth, wideHeight)
	assertAbsent(t, overview, "ERROR", "STALE", "LOADING", sentinelToken)
	assertContains(t, overview, "selection: 0 repos · 0 scopes · 0 exact · 0 expanded")
	if got := endpoint.requestCount("acme/legacy"); got != 0 {
		t.Errorf("an empty selection polled %d times, want no request at all", got)
	}
}

func TestOrganizationFlowFailureIsAnErrorThatPollsNoSubset(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		organizationPath("acme"):   {serverError()},
		eventsPath("other/exact"):  {ok(eventsPage("other/exact", 1))},
		eventsPath("acme/backend"): {ok(eventsPage("acme/backend", 1))},
	})
	run := newFlow(t, endpoint, "--org", "acme", "--repo", "other/exact")
	run.refresh()

	overview := run.render(wideWidth, wideHeight)
	assertContains(t, overview, "ERROR")
	assertAbsent(t, overview, sentinelToken, "SELECTION STALE")
	if got := endpoint.requestCount("other/exact"); got != 0 {
		t.Errorf("a failed initial expansion polled the exact subset %d times, want none", got)
	}
}
