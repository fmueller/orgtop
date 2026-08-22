package domain_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

func TestParseRepositoryValid(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantName  string
		wantKey   string
	}{
		{name: "simple", input: "owner/repo", wantOwner: "owner", wantName: "repo", wantKey: "owner/repo"},
		{name: "shortest components", input: "a/b", wantOwner: "a", wantName: "b", wantKey: "a/b"},
		{name: "mixed case is preserved", input: "OrgTop/Repo.Name", wantOwner: "OrgTop", wantName: "Repo.Name", wantKey: "orgtop/repo.name"},
		{name: "owner with inner hyphen", input: "my-org/repo", wantOwner: "my-org", wantName: "repo", wantKey: "my-org/repo"},
		{name: "owner at maximum length", input: strings.Repeat("a", 39) + "/repo", wantOwner: strings.Repeat("a", 39), wantName: "repo", wantKey: strings.Repeat("a", 39) + "/repo"},
		{name: "repository at maximum length", input: "owner/" + strings.Repeat("r", 100), wantOwner: "owner", wantName: strings.Repeat("r", 100), wantKey: "owner/" + strings.Repeat("r", 100)},
		{name: "repository with allowed punctuation", input: "owner/.dot_under-hyphen", wantOwner: "owner", wantName: ".dot_under-hyphen", wantKey: "owner/.dot_under-hyphen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseRepository(tt.input)
			if err != nil {
				t.Fatalf("ParseRepository(%q) returned error: %v", tt.input, err)
			}
			if got.Owner() != tt.wantOwner {
				t.Errorf("Owner() = %q, want %q", got.Owner(), tt.wantOwner)
			}
			if got.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", got.Name(), tt.wantName)
			}
			if got.String() != tt.input {
				t.Errorf("String() = %q, want %q", got.String(), tt.input)
			}
			if got.Key() != tt.wantKey {
				t.Errorf("Key() = %q, want %q", got.Key(), tt.wantKey)
			}
		})
	}
}

func TestParseRepositoryInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "missing separator", input: "owner"},
		{name: "empty repository component", input: "owner/"},
		{name: "empty owner component", input: "/repo"},
		{name: "three components", input: "owner/repo/extra"},
		{name: "empty middle component", input: "owner//repo"},
		{name: "owner with space", input: "own er/repo"},
		{name: "repository with space", input: "owner/re po"},
		{name: "owner starting with hyphen", input: "-owner/repo"},
		{name: "owner ending with hyphen", input: "owner-/repo"},
		{name: "owner too long", input: strings.Repeat("a", 40) + "/repo"},
		{name: "repository too long", input: "owner/" + strings.Repeat("r", 101)},
		{name: "owner with underscore", input: "own_er/repo"},
		{name: "owner with dot", input: "own.er/repo"},
		{name: "non ascii owner", input: "öwner/repo"},
		{name: "non ascii repository", input: "owner/repö"},
		{name: "surrounding whitespace", input: " owner/repo "},
		{name: "trailing newline", input: "owner/repo\n"},
		{name: "glob pattern", input: "owner/*"},
		{name: "url form", input: "https://github.com/owner/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseRepository(tt.input)
			if err == nil {
				t.Fatalf("ParseRepository(%q) = %v, want error", tt.input, got)
			}
			if !errors.Is(err, domain.ErrInvalidRepository) {
				t.Errorf("error %v does not match ErrInvalidRepository", err)
			}
			if !strings.Contains(err.Error(), strconv.Quote(tt.input)) {
				t.Errorf("error %q does not mention the rejected value %q", err.Error(), tt.input)
			}
		})
	}
}

func TestRepositoryEqualityIsCaseInsensitiveByKey(t *testing.T) {
	lower, err := domain.ParseRepository("owner/repo")
	if err != nil {
		t.Fatalf("ParseRepository returned error: %v", err)
	}
	upper, err := domain.ParseRepository("Owner/REPO")
	if err != nil {
		t.Fatalf("ParseRepository returned error: %v", err)
	}
	if lower.Key() != upper.Key() {
		t.Errorf("Key() mismatch: %q vs %q", lower.Key(), upper.Key())
	}
	if lower.String() == upper.String() {
		t.Errorf("String() should retain the requested spelling, both were %q", lower.String())
	}
}
