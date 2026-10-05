package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// commitEvidence walks the paginated files of one exact commit. Completeness is
// proven, never assumed: every page must repeat the requested commit identity
// and one consistent sole parent, pagination must advance safely within the
// closed page limit, and only a final page without a next link proves the set is
// whole. Any violation discards every path already collected, because a
// valid-looking subset would be indistinguishable from complete evidence.
func (e Enricher) commitEvidence(ctx context.Context, descriptor domain.EvidenceDescriptor) domain.EvidenceOutcome {
	paths := newPathSet()
	parent := ""
	endpoint := e.commitURL(descriptor)
	page := 1
	visited := map[string]struct{}{}

	for range domain.MaxEvidencePages {
		if _, repeated := visited[endpoint]; repeated {
			return domain.IncompleteOutcome(reasonMalformedBody)
		}
		visited[endpoint] = struct{}{}

		body, header, outcome, ok := e.read(ctx, endpoint)
		if !ok {
			return outcome
		}
		var response commitResponse
		if !decode(body, &response) {
			return domain.IncompleteOutcome(reasonMalformedBody)
		}
		soleParent, identical := commitPageIdentity(descriptor, response, parent)
		if !identical || !paths.addRecords(response.Files) {
			return domain.IncompleteOutcome(reasonMalformedBody)
		}
		parent = soleParent

		next, nextPage, present := e.nextCommitPage(header, descriptor, page)
		if !present {
			// A final page without a next link proves completeness, the empty
			// set included. Per-event applicability is then decided from the
			// verified sole parent rather than from another request.
			return domain.CompleteOutcome(domain.ProvenanceEventTime, paths.collected()).WithSoleParent(parent)
		}
		if next == "" {
			// A next link exists but is not safe to follow, so the remaining
			// pages are unreachable and the set is not whole.
			return domain.IncompleteOutcome(reasonMalformedBody)
		}
		endpoint = next
		page = nextPage
	}
	// A next link still present at the page bound proves incompleteness.
	return domain.IncompleteOutcome(reasonMalformedBody)
}

// commitPageIdentity verifies that a page describes the requested commit and one
// consistent sole parent. Commit pages cannot mutate by contract, so a changed
// identity or parent list is malformed rather than mixed across pages, and a
// commit without exactly one parent can prove no event's before object.
func commitPageIdentity(descriptor domain.EvidenceDescriptor, response commitResponse, seenParent string) (string, bool) {
	sha, ok := domain.NormalizeObjectSHA(strings.TrimSpace(response.SHA))
	if !ok || sha != descriptor.Head() {
		return "", false
	}
	if len(response.Parents) != 1 {
		return "", false
	}
	parent, ok := domain.NormalizeObjectSHA(strings.TrimSpace(response.Parents[0].SHA))
	if !ok || (seenParent != "" && parent != seenParent) {
		return "", false
	}
	return parent, true
}

// nextCommitPage reports the response's rel="next" target and whether one was
// offered at all. An offered target that is unsafe to follow comes back empty:
// it must be the same scheme and host as the configured API root, name the same
// canonical repository and exact commit, and carry only the page and per-page
// query OrgTop itself requested, advancing to an unseen page.
func (e Enricher) nextCommitPage(header http.Header, descriptor domain.EvidenceDescriptor, page int) (string, int, bool) {
	raw, present := linkRelation(header, "next")
	if !present {
		return "", 0, false
	}
	parsed, components, ok := parseAPIURL(e.baseURL(), raw)
	if !ok || !matchesEntityPath(components, descriptor.Repository(), "commits", descriptor.Head()) {
		return "", 0, true
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 2 || len(query["page"]) != 1 || len(query["per_page"]) != 1 || query.Get("per_page") != perPage {
		return "", 0, true
	}
	nextPage, err := strconv.Atoi(query.Get("page"))
	if err != nil || nextPage <= page {
		return "", 0, true
	}
	return raw, nextPage, true
}

// linkRelation returns the target of one Link relation. An unreadable or
// ambiguous offered relation returns an empty target with present true, never
// the absence that would prove a terminal page.
func linkRelation(header http.Header, relation string) (string, bool) {
	raw, present := "", false
	for _, value := range header.Values("Link") {
		entries, ok := splitLink(value, ',')
		if !ok {
			return "", true
		}
		for _, entry := range entries {
			// The entry scan already proved balanced quotes/brackets and
			// valid bytes for every comma-delimited entry.
			parts, _ := splitLink(entry, ';')
			target := strings.TrimSpace(parts[0])
			if _, parameter, found := strings.Cut(target, ">"); found && strings.TrimSpace(parameter) != "" {
				// Read a relation even when its separator is missing. The
				// malformed target below then rejects it as offered, not absent.
				parts = append(parts, parameter)
			}
			for _, parameter := range parts[1:] {
				key, relations, _ := strings.Cut(strings.TrimSpace(parameter), "=")
				keys := strings.Fields(key)
				if len(keys) == 0 || !strings.EqualFold(keys[0], "rel") {
					continue
				}
				if len(keys) != 1 {
					return "", true
				}
				relations = strings.TrimSpace(relations)
				tokens := relations
				if strings.HasPrefix(relations, `"`) {
					// RFC 8288 quoted pairs represent the following byte,
					// not a Go escape. Keep the raw value for validation below.
					var decoded strings.Builder
					escaped := false
					for _, b := range []byte(relations) {
						if b == '\\' && !escaped {
							escaped = true
							continue
						}
						decoded.WriteByte(b)
						escaped = false
					}
					tokens = decoded.String()
				}
				for _, token := range strings.FieldsFunc(tokens, func(r rune) bool {
					return r == '"' || r == '\'' || r == ' ' || r == '\t'
				}) {
					if !strings.EqualFold(token, relation) {
						continue
					}
					if present || !strings.HasPrefix(target, "<") || !strings.HasSuffix(target, ">") ||
						!strings.HasPrefix(relations, `"`) || !strings.HasSuffix(relations, `"`) || strings.Count(relations, `"`) != 2 {
						return "", true
					}
					raw, present = target[1:len(target)-1], true
				}
			}
		}
	}
	return raw, present
}

// splitLink recognizes separators only outside URI brackets and quoted strings
// (RFC 8288 section 3 and Appendix B). A quoted-pair consumes the next byte,
// including a quote or backslash. Unclosed constructs cannot prove a final page.
func splitLink(value string, separator byte) ([]string, bool) {
	var parts []string
	start := 0
	quoted, escaped, bracketed := false, false, false
	for index := 0; index < len(value); index++ {
		char := value[index]
		if (char < ' ' && char != '\t') || char == 127 {
			return nil, false
		}
		switch {
		case escaped:
			escaped = false
		case quoted:
			switch char {
			case '\\':
				escaped = true
			case '"':
				quoted = false
			}
		case bracketed:
			if char == '>' {
				bracketed = false
			}
		case char == '<':
			bracketed = true
		case char == '"':
			quoted = true
		case char == separator:
			parts = append(parts, value[start:index])
			start = index + 1
		}
	}
	return append(parts, value[start:]), !quoted && !bracketed
}
