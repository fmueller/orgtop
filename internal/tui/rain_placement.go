package tui

import (
	"encoding/binary"

	"github.com/fmueller/orgtop/internal/domain"
)

// The standard 64-bit FNV-1a parameters RG-006 closes Rain placement on. They
// are written out rather than taken from hash/fnv so the closed constants stay
// readable beside the byte stream they consume.
const (
	fnvOffsetBasis uint64 = 0xcbf29ce484222325
	fnvPrime       uint64 = 0x100000001b3
)

// The ASCII purpose prefixes selecting which placement result a hash of the
// same event and Scope produces. Distinct prefixes keep row and slot
// independent without a second identity (RG-006).
const (
	rowPurpose  = "rain-row-v1"
	slotPurpose = "rain-slot-v1"
)

// The `scope-key-v1` kind bytes and matcher-token markers. `scope-key-v1` names
// the encoding and contributes no literal version bytes of its own (RG-006).
const (
	repositoryKindByte = 'R'
	pathKindByte       = 'P'
	literalTokenByte   = 'L'
	wildcardTokenByte  = 'S'
	recursiveTokenByte = 'D'
	separatorTokenByte = 'X'
)

// rainSlotCells is the width of one logical Rain slot: the shared category
// glyph followed by the current-PR qualifier or a space (RG-006).
const rainSlotCells = 2

// fnv1a returns the standard 64-bit FNV-1a hash of the bytes. Multiplication
// wraps modulo 2^64, which is exactly what unsigned Go arithmetic does.
func fnv1a(data []byte) uint64 {
	hash := fnvOffsetBasis
	for _, octet := range data {
		hash ^= uint64(octet)
		hash *= fnvPrime
	}
	return hash
}

// scopeKeyV1 encodes one Scope's canonical identity as RG-006's `scope-key-v1`:
// the lowercase repository key as a four-byte big-endian length plus its UTF-8
// bytes, the kind byte, and, for a path Scope, its canonical matcher tokens.
// Only canonical identity is read, so a requested spelling never moves an item.
func scopeKeyV1(scope domain.Scope) []byte {
	repository := scope.Repository().Key()
	key := binary.BigEndian.AppendUint32(make([]byte, 0, len(repository)+8), uint32(len(repository)))
	key = append(key, repository...)
	if scope.Kind() != domain.ScopePath {
		return append(key, repositoryKindByte)
	}
	key = append(key, pathKindByte)
	for _, token := range scope.Matcher().CanonicalTokens() {
		switch token.Kind {
		case domain.MatcherTokenLiteral:
			key = append(key, literalTokenByte)
			key = binary.BigEndian.AppendUint32(key, uint32(len(token.Literal)))
			key = append(key, token.Literal...)
		case domain.MatcherTokenWildcard:
			key = append(key, wildcardTokenByte)
		case domain.MatcherTokenRecursive:
			key = append(key, recursiveTokenByte)
		case domain.MatcherTokenSeparator:
			key = append(key, separatorTokenByte)
		}
	}
	return key
}

// placementHash hashes RG-006's exact placement byte stream: the ASCII purpose
// prefix, byte `00`, the normalized event ID, byte `00`, and `scope-key-v1`.
func placementHash(purpose, event string, scope domain.Scope) uint64 {
	key := scopeKeyV1(scope)
	stream := make([]byte, 0, len(purpose)+len(event)+len(key)+2)
	stream = append(stream, purpose...)
	stream = append(stream, 0)
	stream = append(stream, event...)
	stream = append(stream, 0)
	stream = append(stream, key...)
	return fnv1a(stream)
}

// rainRowHash returns the stable initial-row hash of one event/Scope identity.
func rainRowHash(event string, scope domain.Scope) uint64 {
	return placementHash(rowPurpose, event, scope)
}

// rainSlotHash returns the stable desired-slot hash of one event/Scope identity.
func rainSlotHash(event string, scope domain.Scope) uint64 {
	return placementHash(slotPurpose, event, scope)
}

// reduce maps a hash onto a bounded count by unsigned modulo. A count at or
// below zero has no position at all, which RG-006 renders as its explicit
// constrained state rather than as position zero.
func reduce(hash uint64, count int) int {
	if count <= 0 {
		return 0
	}
	return int(hash % uint64(count))
}

// rainSlots returns the exact horizontal slot count of one column interior
// width: none at or below zero, one one-cell slot at exactly one, and
// `floor(C/2)` two-cell slots from two (RG-006).
func rainSlots(interior int) int {
	switch {
	case interior <= 0:
		return 0
	case interior == 1:
		return 1
	default:
		return interior / rainSlotCells
	}
}

// rainClipsQualification reports whether a column of this interior width hides
// the current-PR `~`. Only the exact one-cell column does, and the hidden
// qualification stays counted and reachable rather than replaced by color.
func rainClipsQualification(interior int) bool { return interior == 1 }
