package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// These flows exercise the closed v0.2.0 binary path end to end: argument
// parsing, the GitHub source, the changed-file enrichment and its SQLite cache,
// Scope membership, aggregation, and the rendered views. The same fixture
// endpoint answers every request, so a flow can state the exact request budget
// one refresh spends across the poll and enrichment pipelines (T-069).

// The pinned objects every commit fixture below is built from. A push carrying
// distinct valid before and head SHAs and a size of one is exactly the
// one-commit evidence the adapter settles through the commits endpoint.
const (
	fixtureBefore = "1111111111111111111111111111111111111111"
	insideHead    = "2222222222222222222222222222222222222222"
	outsideHead   = "3333333333333333333333333333333333333333"
)

// The repositories the flows below select.
const (
	backend  = "acme/backend"
	frontend = "acme/frontend"
)

// commitPath is the changed-file evidence path the enricher requests for a
// one-commit push of the repository.
func commitPath(repository, head string) string {
	return "/repos/" + repository + "/commits/" + head
}

// commitPush renders one synthetic one-commit push whose changed files the
// enrichment must acquire before any path membership is decided.
func commitPush(repository, id, head string, age time.Duration) map[string]any {
	event := pushEvent(repository, id, wiredInstant.Add(-age))
	event["payload"] = map[string]any{
		"size":   1,
		"ref":    "refs/heads/main",
		"before": fixtureBefore,
		"head":   head,
	}
	return event
}

// commitPage renders the commits response that proves a commit's changed paths:
// the requested identity, the sole parent the event's before SHA must match, and
// a final page carrying no next link.
func commitPage(head string, paths ...string) string {
	files := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		files = append(files, map[string]any{"filename": path, "status": "modified"})
	}
	encoded, err := json.Marshal(map[string]any{
		"sha":     head,
		"parents": []map[string]any{{"sha": fixtureBefore}},
		"files":   files,
	})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// mixedScopeScript is the fixture a mixed-Scope flow serves: one push inside the
// path Scope and one outside it, each with its own commit evidence.
func mixedScopeScript() map[string][]cannedResponse {
	return map[string][]cannedResponse{
		eventsPath(backend): {ok(encode([]map[string]any{
			commitPush(backend, "inside", insideHead, time.Minute),
			commitPush(backend, "outside", outsideHead, 2*time.Minute),
		}))},
		commitPath(backend, insideHead):  {ok(commitPage(insideHead, "src/main.go"))},
		commitPath(backend, outsideHead): {ok(commitPage(outsideHead, "go.mod"))},
	}
}

// TestMixedScopeFlowDecidesMembershipThroughWiredEnrichment proves the closed
// pipeline through the binary: a repository Scope and a path Scope over the same
// repository both reach Stream, and the path Scope's membership is settled from
// changed-file evidence the wired enrichment actually acquired rather than left
// unresolved (FR-003, FR-004, A-002, A-003).
func TestMixedScopeFlowDecidesMembershipThroughWiredEnrichment(t *testing.T) {
	endpoint := newEndpoint(mixedScopeScript())
	run := newFlow(t, endpoint, "--repo", backend, "--path", backend+":src")
	run.refresh()

	// Overview lists both Scopes with their own direct counts: the in-path push
	// is counted under each Scope it belongs to, never averaged or attributed to
	// only one of them.
	overview := body(t, run.render(wideWidth, wideHeight))
	if len(overview) != 2 {
		t.Fatalf("overview rendered %d rows, want one per Scope:\n%v", len(overview), overview)
	}
	assertContains(t, overview[0], "R1", backend, "2 activity", "2 pushes")
	assertContains(t, overview[1], "P2", backend+":src", "1 activity", "1 push")

	run.press("2")
	rows := streamBody(t, run.render(wideWidth, wideHeight))
	if len(rows) != 2 {
		t.Fatalf("stream rendered %d event rows, want 2:\n%v", len(rows), rows)
	}
	// The snapshot is reverse-chronological, so the newer in-path push leads.
	assertContains(t, rows[0], "in R1, P2")
	assertContains(t, rows[1], "in R1")
	assertAbsent(t, rows[1], "P2")
	for _, row := range rows {
		assertAbsent(t, row, "unresolved")
	}
}

// TestPinnedRepositoryOnlyFlowPreservesV01BehaviorWithoutEnrichment is the
// pinned A-001 fixture. One exact repository-only invocation over two
// repositories proves the whole v0.1 contract through the v0.2.0 binary: the
// same one request per repository per refresh, the same atomic snapshot,
// Overview counts, Stream order, and polling label, the same stale behavior
// when one repository later fails, and no dependency on either changed-file
// enrichment or the cache. The flow binds a coordination that fails the test the
// moment it is asked for evidence, so the independence is proven by the request
// never being made rather than by output that could look the same either way
// (FR-001, A-001, A-016).
func TestPinnedRepositoryOnlyFlowPreservesV01BehaviorWithoutEnrichment(t *testing.T) {
	// The second frontend response fails, so the same fixture also pins what a
	// repository-only selection does when one repository stops answering.
	script := func() map[string][]cannedResponse {
		return map[string][]cannedResponse{
			eventsPath(backend): {ok(encode([]map[string]any{
				commitPush(backend, "inside", insideHead, time.Minute),
				commitPush(backend, "outside", outsideHead, 2*time.Hour),
			}))},
			eventsPath(frontend):             {ok(pullRequestPage(frontend)), serverError()},
			commitPath(backend, insideHead):  {ok(commitPage(insideHead, "src/main.go"))},
			commitPath(backend, outsideHead): {ok(commitPage(outsideHead, "go.mod"))},
		}
	}

	endpoint := newEndpoint(script())
	run := newFlowWith(t, endpoint, refusingEnricher{t}, "--repo", backend, "--repo", frontend)
	run.refresh()

	overview := run.render(wideWidth, wideHeight)
	assertContains(t, overview, "OVERVIEW", "POLLING", backend, "2 activity", "2 pushes", frontend, "1 pull request")
	assertAbsent(t, overview, "ERROR", "STALE", "CACHE DEGRADED")

	run.press("2")
	stream := run.render(wideWidth, wideHeight)
	rows := streamBody(t, stream)
	if len(rows) != 3 {
		t.Fatalf("stream rendered %d event rows, want 3:\n%v", len(rows), rows)
	}
	// The snapshot is one atomic reverse-chronological order across both
	// repositories, so the ages read straight down the column.
	for index, want := range []string{"1m", "30m", "2h"} {
		if age := strings.Fields(rows[index])[0]; age != want {
			t.Errorf("row %d is aged %q, want %q:\n%s", index, age, want, rows[index])
		}
	}
	// The flow runs against a coordination that fails the test when it is asked
	// for evidence, so reaching this point already proves the selection consulted
	// none. That no row is left waiting on path evidence is the operator-visible
	// half of the same contract.
	for _, row := range rows {
		assertAbsent(t, row, "unresolved", "?P")
	}

	for _, repository := range []string{backend, frontend} {
		if count := endpoint.requestCount(repository); count != 1 {
			t.Errorf("%s answered %d requests, want exactly one per refresh", repository, count)
		}
	}
	for _, head := range []string{insideHead, outsideHead} {
		if count := endpoint.servedCount(commitPath(backend, head)); count != 0 {
			t.Errorf("the repository-only refresh spent %d enrichment requests on %s, want none", count, head)
		}
	}

	// The cache decision must change nothing an operator sees for a
	// repository-only selection, so the same invocation under --no-cache renders
	// identically. The zero-dependency proof is the refused coordination above;
	// this pins that the flag stays inert for a v0.1 invocation.
	uncached := newFlowWith(t, newEndpoint(script()), refusingEnricher{t}, "--repo", backend, "--repo", frontend, "--no-cache")
	uncached.refresh()
	uncached.press("2")
	if got := uncached.render(wideWidth, wideHeight); got != stream {
		t.Errorf("the --no-cache render differs from the cached render:\nwant:\n%s\ngot:\n%s", stream, got)
	}

	// A repository that stops answering leaves the snapshot standing and marked
	// stale rather than emptying it.
	run.refresh()
	stale := run.render(wideWidth, wideHeight)
	assertContains(t, stale, "STALE")
	if rows := streamBody(t, stale); len(rows) != 3 {
		t.Errorf("the stale snapshot kept %d event rows, want the 3 it already held:\n%v", len(rows), rows)
	}
}

// TestScopedFlowReusesCachedEvidenceAcrossRefreshes proves the launch's cache
// binding reaches the wired refresh: a second refresh of the same events settles
// the same membership without spending a single further enrichment request, and
// reports no degradation (FR-005, A-006).
func TestScopedFlowReusesCachedEvidenceAcrossRefreshes(t *testing.T) {
	endpoint := newEndpoint(mixedScopeScript())
	run := newFlow(t, endpoint, "--repo", backend, "--path", backend+":src")
	run.refresh()
	run.refresh()
	run.press("2")

	content := run.render(wideWidth, wideHeight)
	assertContains(t, content, "in R1, P2")
	assertAbsent(t, content, "unresolved", "CACHE DEGRADED")
	if count := endpoint.servedCount(commitPath(backend, insideHead)); count != 1 {
		t.Errorf("two refreshes spent %d enrichment requests, want the second served from the cache", count)
	}
	if count := endpoint.requestCount(backend); count != 2 {
		t.Errorf("the flow polled %d times, want one poll per refresh", count)
	}
}

// TestNoCacheScopedFlowReacquiresEvidenceEveryRefresh proves --no-cache reaches
// the wired coordination: membership stays identical, and every refresh
// reacquires its evidence from the source rather than reusing a stored record
// (FR-005, A-019).
func TestNoCacheScopedFlowReacquiresEvidenceEveryRefresh(t *testing.T) {
	endpoint := newEndpoint(mixedScopeScript())
	run := newFlow(t, endpoint, "--repo", backend, "--path", backend+":src", "--no-cache")
	run.refresh()
	run.refresh()
	run.press("2")

	content := run.render(wideWidth, wideHeight)
	assertContains(t, content, "in R1, P2")
	assertAbsent(t, content, "unresolved", "CACHE DEGRADED")
	if count := endpoint.servedCount(commitPath(backend, insideHead)); count != 2 {
		t.Errorf("--no-cache spent %d enrichment requests over two refreshes, want one per refresh", count)
	}
}

// TestScopedFlowLeavesFailedEvidenceUnresolvedAndRecovers proves the honest
// degraded path end to end: evidence the source could not settle keeps the path
// Scope undecided rather than deciding non-membership, and a later refresh that
// settles it resolves the same event (FR-004, A-011).
func TestScopedFlowLeavesFailedEvidenceUnresolvedAndRecovers(t *testing.T) {
	endpoint := newEndpoint(map[string][]cannedResponse{
		eventsPath(backend): {ok(encode([]map[string]any{
			commitPush(backend, "inside", insideHead, time.Minute),
		}))},
		commitPath(backend, insideHead): {serverError(), ok(commitPage(insideHead, "src/main.go"))},
	})
	run := newFlow(t, endpoint, "--repo", backend, "--path", backend+":src")
	run.refresh()
	run.press("2")

	failed := run.render(wideWidth, wideHeight)
	assertContains(t, failed, "unresolved", "?P2")
	assertAbsent(t, failed, "in R1, P2")

	run.refresh()
	recovered := run.render(wideWidth, wideHeight)
	assertContains(t, recovered, "in R1, P2")
	assertAbsent(t, recovered, "unresolved")
}

// TestRainViewPausesAndResumesThroughTheWiredFlow proves the third view is
// reachable and controllable through the binary: `3` selects Rain over the same
// loaded snapshot, `p` freezes the field and says so, and a second `p` resumes
// it without any further source or enrichment request (FR-008, A-008).
func TestRainViewPausesAndResumesThroughTheWiredFlow(t *testing.T) {
	endpoint := newEndpoint(mixedScopeScript())
	run := newFlow(t, endpoint, "--repo", backend, "--path", backend+":src")
	run.refresh()

	run.press("3")
	rain := run.render(wideWidth, wideHeight)
	assertContains(t, rain, "RAIN")
	assertAbsent(t, rain, "PAUSED")

	run.press("p")
	assertContains(t, run.render(wideWidth, wideHeight), "RAIN", "PAUSED")

	run.press("p")
	assertAbsent(t, run.render(wideWidth, wideHeight), "PAUSED")

	if count := endpoint.requestCount(backend); count != 1 {
		t.Errorf("Rain navigation polled %d times, want the loaded snapshot to be reused", count)
	}
	if count := endpoint.servedCount(commitPath(backend, insideHead)); count != 1 {
		t.Errorf("Rain navigation spent %d enrichment requests, want the settled evidence to be reused", count)
	}
}
