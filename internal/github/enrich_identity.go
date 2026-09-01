package github

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// responseURL parses a URL a response claimed for itself and reports its path
// components only when every RG-003 invariant holds: the configured API root's
// scheme and host, no query, fragment, or credentials, no trailing slash, and no
// percent escape that could spell an alternate path. The invariants are checked
// against the configured root rather than a literal host so a fixture server and
// the public API prove the same rules.
func responseURL(root, value string) ([]string, bool) {
	parsed, components, ok := parseAPIURL(root, value)
	if !ok {
		return nil, false
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return nil, false
	}
	return components, true
}

// parseAPIURL applies every RG-003 URL invariant except the query rule, which
// differs between a self-describing response URL and a pagination link OrgTop
// itself parameterized.
func parseAPIURL(root, value string) (*url.URL, []string, bool) {
	rootURL, err := url.Parse(root)
	if err != nil {
		return nil, nil, false
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, nil, false
	}
	if parsed.Scheme != rootURL.Scheme || parsed.Host != rootURL.Host {
		return nil, nil, false
	}
	if parsed.User != nil || parsed.Fragment != "" || parsed.RawFragment != "" {
		return nil, nil, false
	}
	path := parsed.EscapedPath()
	if strings.Contains(path, "%") {
		return nil, nil, false
	}
	if !strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return nil, nil, false
	}
	return parsed, strings.Split(strings.TrimPrefix(path, "/"), "/"), true
}

// matchesRepositoryPath reports whether the leading `/repos/{owner}/{repo}`
// components canonically name the requested repository. Only case may differ:
// literal components are compared through the canonical repository key so a
// redirect or a rename cannot silently change evidence identity.
func matchesRepositoryPath(components []string, repository domain.Repository) bool {
	if len(components) < 3 || components[0] != "repos" {
		return false
	}
	returned, err := domain.ParseRepository(components[1] + "/" + components[2])
	if err != nil {
		return false
	}
	return returned.Key() == repository.Key()
}

// matchesCanonicalRepository reports whether a returned `owner/repository` name
// canonically equals the requested repository.
func matchesCanonicalRepository(fullName string, repository domain.Repository) bool {
	returned, err := domain.ParseRepository(strings.TrimSpace(fullName))
	if err != nil {
		return false
	}
	return returned.Key() == repository.Key()
}

// matchesEntityPath reports whether path components name exactly
// `/repos/{owner}/{repo}/{collection}/{entity}` for the requested repository. A
// longer or shorter path is never accepted, so no returned URL can name a
// neighbouring entity of the one that was requested.
func matchesEntityPath(components []string, repository domain.Repository, collection, entity string) bool {
	if len(components) != 5 || !matchesRepositoryPath(components, repository) {
		return false
	}
	return components[3] == collection && components[4] == entity
}

// matchesCompareURL reports whether a compare response's claimed URL is exactly
// `/repos/{owner}/{repo}/compare/{base}...{head}` for the immutable request.
func matchesCompareURL(root, value string, descriptor domain.EvidenceDescriptor) bool {
	components, ok := responseURL(root, value)
	return ok && matchesEntityPath(components, descriptor.Repository(), "compare", descriptor.Base()+"..."+descriptor.Head())
}

// matchesPullRequestURL reports whether a pull request response's claimed URL is
// exactly `/repos/{owner}/{repo}/pulls/{number}` for the requested number.
func matchesPullRequestURL(root, value string, descriptor domain.EvidenceDescriptor) bool {
	components, ok := responseURL(root, value)
	return ok && matchesEntityPath(components, descriptor.Repository(), "pulls", strconv.Itoa(descriptor.Number()))
}
