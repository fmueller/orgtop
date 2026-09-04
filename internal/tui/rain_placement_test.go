package tui

import (
	"encoding/hex"
	"testing"

	"github.com/fmueller/orgtop/internal/domain"
)

// placementEvent is the A-050 fixture event ID every closed placement vector
// below is stated for.
const placementEvent = "12345"

// placementRepository is the A-050 fixture repository.
const placementRepository = "acme/api"

// placementFixtureScopes returns A-050's two Scopes: the repository Scope of
// `acme/api` and its all-path Scope `**`.
func placementFixtureScopes(t *testing.T) (repository, path domain.Scope) {
	t.Helper()
	parsed, err := domain.ParseRepository(placementRepository)
	if err != nil {
		t.Fatalf("parse repository %q: %v", placementRepository, err)
	}
	matcher, err := domain.NewPathMatcher([]domain.MatcherToken{domain.RecursiveToken()})
	if err != nil {
		t.Fatalf("build `**` matcher: %v", err)
	}
	pathScope, err := domain.NewPathScope(parsed, matcher)
	if err != nil {
		t.Fatalf("build path Scope: %v", err)
	}
	return domain.NewRepositoryScope(parsed), pathScope
}

// TestScopeKeyV1MatchesTheClosedEncoding guards RG-006's `scope-key-v1`: a
// four-byte big-endian length plus the lowercase repository key, the kind byte,
// and, for a path Scope, its canonical matcher tokens. The expected bytes are
// A-050's, transcribed from the spec rather than from the implementation.
func TestScopeKeyV1MatchesTheClosedEncoding(t *testing.T) {
	repository, path := placementFixtureScopes(t)
	cases := []struct {
		name  string
		scope domain.Scope
		want  string
	}{
		{name: "repository", scope: repository, want: "0000000861636d652f61706952"},
		{name: "all paths", scope: path, want: "0000000861636d652f6170695044"},
	}
	for _, want := range cases {
		if got := hex.EncodeToString(scopeKeyV1(want.scope)); got != want.want {
			t.Errorf("scope-key-v1 of the %s Scope is %s, want %s", want.name, got, want.want)
		}
	}
}

// TestPlacementHashesMatchTheClosedVectors guards RG-006's FNV-1a placement
// stream: an ASCII purpose prefix, byte `00`, the event ID, byte `00`, and
// `scope-key-v1`. Every hash and its reduced row/slot is A-050's.
func TestPlacementHashesMatchTheClosedVectors(t *testing.T) {
	repository, path := placementFixtureScopes(t)
	cases := []struct {
		name     string
		scope    domain.Scope
		row      string
		slot     string
		wantRow  int
		wantSlot int
	}{
		{
			name:     "repository",
			scope:    repository,
			row:      "bd347311c94a8e2e",
			slot:     "66197054b8515a46",
			wantRow:  4,
			wantSlot: 4,
		},
		{
			name:     "all paths",
			scope:    path,
			row:      "caa6ad3909a96be4",
			slot:     "ce8d1df5323638ac",
			wantRow:  3,
			wantSlot: 2,
		},
	}
	for _, want := range cases {
		row := rainRowHash(placementEvent, want.scope)
		if got := hex.EncodeToString(uint64Bytes(row)); got != want.row {
			t.Errorf("%s row hash is %s, want %s", want.name, got, want.row)
		}
		slot := rainSlotHash(placementEvent, want.scope)
		if got := hex.EncodeToString(uint64Bytes(slot)); got != want.slot {
			t.Errorf("%s slot hash is %s, want %s", want.name, got, want.slot)
		}
		if got := reduce(row, 7); got != want.wantRow {
			t.Errorf("%s row of 7 is %d, want %d", want.name, got, want.wantRow)
		}
		if got := reduce(slot, 6); got != want.wantSlot {
			t.Errorf("%s slot of 6 is %d, want %d", want.name, got, want.wantSlot)
		}
	}
}

// TestRainSlotsMatchTheClosedSlotFunction guards RG-006's exact horizontal slot
// count: none at or below zero interior cells, exactly one one-cell slot at one,
// and `floor(C/2)` two-cell slots from two.
func TestRainSlotsMatchTheClosedSlotFunction(t *testing.T) {
	cases := []struct{ width, slots int }{
		{width: -3, slots: 0},
		{width: 0, slots: 0},
		{width: 1, slots: 1},
		{width: 2, slots: 1},
		{width: 3, slots: 1},
		{width: 4, slots: 2},
		{width: 12, slots: 6},
		{width: 13, slots: 6},
	}
	for _, want := range cases {
		if got := rainSlots(want.width); got != want.slots {
			t.Errorf("interior width %d has %d slots, want %d", want.width, got, want.slots)
		}
	}
}

// TestRainClipsQualificationOnlyInOneCellColumns guards RG-006's single narrow
// exception: only the exact one-cell column drops the `~` qualifier.
func TestRainClipsQualificationOnlyInOneCellColumns(t *testing.T) {
	for width, want := range map[int]bool{0: false, 1: true, 2: false, 12: false} {
		if got := rainClipsQualification(width); got != want {
			t.Errorf("interior width %d clips qualification %t, want %t", width, got, want)
		}
	}
}

// TestPlacementIsIndependentOfRequestedSpelling guards that placement reads
// canonical identity: a differently spelled but identically canonical Scope
// hashes to the same row and slot.
func TestPlacementIsIndependentOfRequestedSpelling(t *testing.T) {
	upper, err := domain.ParseRepository("ACME/API")
	if err != nil {
		t.Fatalf("parse repository: %v", err)
	}
	repository, _ := placementFixtureScopes(t)
	other := domain.NewRepositoryScope(upper)
	if got, want := rainRowHash(placementEvent, other), rainRowHash(placementEvent, repository); got != want {
		t.Errorf("row hash of the differently spelled Scope is %x, want %x", got, want)
	}
}

// uint64Bytes renders a hash big-endian so a test can compare it to the closed
// hexadecimal vector exactly as the spec writes it.
func uint64Bytes(value uint64) []byte {
	encoded := make([]byte, 8)
	for index := 7; index >= 0; index-- {
		encoded[index] = byte(value)
		value >>= 8
	}
	return encoded
}
