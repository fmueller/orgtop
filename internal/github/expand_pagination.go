package github

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// listingURL is the documented listing request for one page.
func listingURL(root, organization string, page int) string {
	return fmt.Sprintf("%s/orgs/%s/repos?%s", root, url.PathEscape(organization), listingQuery(page))
}

// listingQuery is the exact query every listing request and every valid next
// link carries.
func listingQuery(page int) string {
	return fmt.Sprintf("type=all&sort=full_name&direction=asc&per_page=%s&page=%d", perPage, page)
}

// nextListingPage returns the next page link the selector may follow, or an
// empty target once the list ended. A link that is not exactly the same
// organization listing advanced by one page fails the whole attempt.
func (s Source) nextListingPage(header http.Header, entry *listing) (string, error) {
	raw, offered, err := nextListingLink(header)
	if err != nil || !offered {
		return "", err
	}
	parsed, components, ok := parseAPIURL(s.baseURL(), raw)
	if !ok {
		return "", fmt.Errorf("%w: the next page link is not a github api url", ErrInvalidPagination)
	}
	if len(components) != 3 || components[0] != "orgs" || components[2] != "repos" ||
		!strings.EqualFold(components[1], entry.organization) {
		return "", fmt.Errorf("%w: the next page link does not name the requested organization listing", ErrInvalidPagination)
	}
	if !advancesListingPage(parsed.Query(), entry.page+1) {
		return "", fmt.Errorf("%w: the next page link does not advance the requested listing query", ErrInvalidPagination)
	}
	return raw, nil
}

// nextListingLink returns the rel="next" target of a listing response. A Link
// header that offers a next relation OrgTop cannot read is unexpected
// pagination rather than an ended list.
func nextListingLink(header http.Header) (string, bool, error) {
	if raw, present := linkRelation(header, "next"); present {
		return raw, true, nil
	}
	for _, value := range header.Values("Link") {
		if strings.Contains(strings.ToLower(value), `rel="next"`) {
			return "", false, fmt.Errorf("%w: the response offered an unreadable next page link", ErrInvalidPagination)
		}
	}
	return "", false, nil
}

// advancesListingPage reports whether a next link carries exactly the requested
// listing query with the page advanced from N to N+1. The expectation is parsed
// from the request the adapter itself builds, so the query contract keeps one
// definition and a link cannot pass against a stale copy of it.
func advancesListingPage(query url.Values, page int) bool {
	expected, err := url.ParseQuery(listingQuery(page))
	if err != nil || len(query) != len(expected) {
		return false
	}
	for key := range expected {
		if query.Get(key) != expected.Get(key) {
			return false
		}
	}
	return true
}
