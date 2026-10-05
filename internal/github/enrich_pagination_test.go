package github_test

import (
	"fmt"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

func TestCommitPaginationProof(t *testing.T) {
	tests := []struct {
		name, link string
		complete   bool
	}{
		{"absent", "", true},
		{"previous only", `<%s>; rel="prev"`, true},
		{"increasing", `<%s>; rel="next"`, true},
		{"escaped letter", `<%s>; rel="ne\xt"`, true},
		{"escaped space", `<%s>; rel="prev\ next"`, true},
		{"escaped non-next", `<%s>; rel="la\st"`, true},
		{"escaped dangling", `<%s>; rel="ne\xt\`, false},
		{"escaped trailing junk", `<%s>; rel="ne\xt"junk`, false},
		{"relation list", `<%s>; rel="prev next"`, true},
		{"quoted comma", `<%s>; title="page, two"; rel="next"`, true},
		{"quoted semicolon", `<%s>; title="page; two"; rel="next"`, true},
		{"escaped quote", `<%s>; title="page\",; two"; rel="next"`, true},
		{"relation first", `<%s>; rel="next"; title="page, two"`, true},
		{"false relation in title", `<%s>; title="page; rel=\"next\", two"; rel="prev"`, true},
		{"unclosed title", `<%s>; title="page, two; rel="next"`, false},
		{"dangling escape", `<%s>; rel="next"; title="page\`, false},
		{"missing brackets", `%s; rel="next"`, false},
		{"missing closing bracket", `<%s; rel="next"`, false},
		{"unquoted relation", `<%s>; rel=next`, false},
		{"unclosed relation", `<%s>; rel="next`, false},
		{"extra relation quote", `<%s>; rel="next""`, false},
		{"missing relation separator", `<%s> rel="next"`, false},
		{"missing relation list separator", `<%s> rel="prev next"`, false},
		{"missing relation equals", `<%s> rel "next"`, false},
		{"relation trailing junk", `<%s>; rel="next"junk`, false},
		{"single quoted relation", `<%s>; rel='next'`, false},
		{"duplicate next", `<%s>; rel="next", <%s>; rel="next"`, false},
		{"duplicate page", `<%s&page=999>; rel="next"`, false},
		{"duplicate per page", `<%s&per_page=50>; rel="next"`, false},
		{"bad query escape", `<%s&ignored=%%zz>; rel="next"`, false},
		{"bad query separator", `<%s&ignored=x;y>; rel="next"`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			responses := map[string]stubResponse{}
			server := enrichFixtureServer(t, responses)
			target := server.URL + commitPath(3)
			link := ""
			if test.link != "" {
				if test.name == "duplicate next" {
					link = fmt.Sprintf(test.link, target, target)
				} else {
					link = fmt.Sprintf(test.link, target)
				}
			}
			responses[commitPath(1)] = stubResponse{body: commitPage(fileRecordsFrom(7, 1)), header: map[string]string{"Link": link}}
			responses[commitPath(3)] = stubResponse{body: commitPage(fileRecordsFrom(19, 1))}
			enricher, client := newTestEnricher(t, server)
			outcome := changed(t, enricher, mustCommitEvidence(t, commitSHA))
			wantKind, wantPaths, wantRequests := domain.OutcomeIncomplete, 0, 1
			if test.complete {
				wantKind, wantPaths = domain.OutcomeComplete, 1
				if test.name != "absent" && test.name != "previous only" && test.name != "false relation in title" && test.name != "escaped non-next" {
					wantPaths, wantRequests = 2, 2
				}
			}
			if outcome.Kind() != wantKind || len(outcome.Paths()) != wantPaths || len(client.recorded()) != wantRequests {
				t.Fatalf("kind/paths/requests = %v/%d/%d, want %v/%d/%d", outcome.Kind(), len(outcome.Paths()), len(client.recorded()), wantKind, wantPaths, wantRequests)
			}
		})
	}
}

func TestCommitPaginationRejectsBackwardAndRepeatedPages(t *testing.T) {
	for _, next := range []int{2, 3} {
		t.Run(fmt.Sprint(next), func(t *testing.T) {
			responses := map[string]stubResponse{}
			server := enrichFixtureServer(t, responses)
			responses[commitPath(1)] = stubResponse{body: commitPage(fileRecordsFrom(7, 1)), header: nextLink(server, 3)}
			responses[commitPath(3)] = stubResponse{body: commitPage(fileRecordsFrom(19, 1)), header: nextLink(server, next)}
			responses[commitPath(2)] = stubResponse{body: commitPage(fileRecordsFrom(31, 1))}
			enricher, client := newTestEnricher(t, server)
			outcome := changed(t, enricher, mustCommitEvidence(t, commitSHA))
			if outcome.Kind() != domain.OutcomeIncomplete || len(outcome.Paths()) != 0 || len(client.recorded()) != 2 {
				t.Fatalf("kind/paths/requests = %v/%d/%d, want incomplete/0/2", outcome.Kind(), len(outcome.Paths()), len(client.recorded()))
			}
		})
	}
}

func TestCommitPaginationRejectsSamePageAtDifferentURL(t *testing.T) {
	responses := map[string]stubResponse{}
	server := enrichFixtureServer(t, responses)
	alias := "/repos/Acme/Web/commits/" + commitSHA + "?per_page=100&page=3"
	responses[commitPath(1)] = stubResponse{body: commitPage(fileRecordsFrom(7, 1)), header: nextLink(server, 3)}
	responses[commitPath(3)] = stubResponse{body: commitPage(fileRecordsFrom(19, 1)), header: map[string]string{
		"Link": fmt.Sprintf(`<%s%s>; rel="next"`, server.URL, alias)}}
	responses[alias] = stubResponse{body: commitPage(fileRecordsFrom(31, 1))}
	enricher, client := newTestEnricher(t, server)
	outcome := changed(t, enricher, mustCommitEvidence(t, commitSHA))
	if outcome.Kind() != domain.OutcomeIncomplete || len(outcome.Paths()) != 0 || len(client.recorded()) != 2 {
		t.Fatalf("kind/paths/requests = %v/%d/%d, want incomplete/0/2", outcome.Kind(), len(outcome.Paths()), len(client.recorded()))
	}
}
