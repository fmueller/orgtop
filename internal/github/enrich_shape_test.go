package github_test

import (
	"fmt"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

func TestEnrichmentRequiresFilesArray(t *testing.T) {
	for _, shape := range []struct {
		name     string
		field    string
		complete bool
	}{
		{name: "absent"},
		{name: "null", field: `,"files":null`},
		{name: "object", field: `,"files":{}`},
		{name: "string", field: `,"files":"[]"`},
		{name: "number", field: `,"files":1`},
		{name: "boolean", field: `,"files":false`},
		{name: "empty array", field: `,"files":[]`, complete: true},
	} {
		for _, endpoint := range []string{"commit", "later commit page", "compare"} {
			t.Run(endpoint+"/"+shape.name, func(t *testing.T) {
				responses := map[string]stubResponse{}
				server := enrichFixtureServer(t, responses)
				descriptor := mustCommitEvidence(t, commitSHA)
				body := fmt.Sprintf(`{"sha":%q,"parents":[{"sha":%q}]%s}`, commitSHA, parentSHA, shape.field)
				switch endpoint {
				case "commit":
					responses[commitPath(1)] = stubResponse{body: body}
				case "later commit page":
					responses[commitPath(1)] = stubResponse{body: commitPage(fileRecords(1)), header: nextLink(server, 2)}
					responses[commitPath(2)] = stubResponse{body: body}
				case "compare":
					descriptor = mustCompareEvidence(t, parentSHA, commitSHA)
					responses[comparePath(parentSHA, commitSHA)] = stubResponse{body: fmt.Sprintf(
						`{"url":%q,"base_commit":{"sha":%q}%s}`, server.URL+comparePath(parentSHA, commitSHA), parentSHA, shape.field)}
				}
				enricher, client := newTestEnricher(t, server)
				outcome := changed(t, enricher, descriptor)
				wantStatus := domain.OutcomeIncomplete
				wantPaths, wantRequests := 0, 1
				if endpoint == "later commit page" {
					wantRequests = 2
					if shape.complete {
						wantPaths = 1
					}
				}
				if shape.complete {
					wantStatus = domain.OutcomeComplete
				}
				if outcome.Kind() != wantStatus || len(outcome.Paths()) != wantPaths {
					t.Errorf("status/paths = %v/%v, want %v/%d", outcome.Kind(), pathStrings(outcome), wantStatus, wantPaths)
				}
				if len(client.recorded()) != wantRequests {
					t.Errorf("requests = %d, want %d", len(client.recorded()), wantRequests)
				}
			})
		}
	}
}
