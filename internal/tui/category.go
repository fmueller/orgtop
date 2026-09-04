package tui

import (
	"strings"

	"github.com/fmueller/orgtop/internal/domain"
)

// charset names the character repertoire a category glyph is drawn from. It is
// an injected capability rather than a probe of the live terminal, so the same
// inputs render the same glyph in a test and in a session (RG-008).
type charset int

// The supported repertoires. ASCII is the zero value, because it is the answer
// a terminal that says nothing about its locale gets.
const (
	charsetASCII charset = iota
	charsetUTF8
)

// categoryRegister names how much room a view has for the category text. Both
// registers spell every category; the compact one only spends fewer cells.
type categoryRegister int

// The text registers, richest first.
const (
	registerRich categoryRegister = iota
	registerCompact
)

// categorySemantics is RG-008's closed presentation of one normalized category:
// the two text registers and the one-cell glyph of each repertoire. A glyph
// encodes the category alone. In particular `↗` does not mean growth, `◇` does
// not mean approval, `○` does not mean status, and none of them means success,
// failure, deployment, anomaly, priority, or importance.
type categorySemantics struct {
	rich    string
	compact string
	utf8    string
	ascii   string
}

// categoryVocabulary is the closed shared mapping every view renders a category
// through. It preserves the v0.1.0 text inventory and adds the shared glyphs, so
// a category is understandable without color and no renderer invents a spelling
// or a glyph of its own (RG-008).
var categoryVocabulary = map[domain.Category]categorySemantics{
	domain.CategoryPush:        {rich: "push", compact: "push", utf8: "↑", ascii: "P"},
	domain.CategoryPullRequest: {rich: "pull request", compact: "pull", utf8: "↗", ascii: "R"},
	domain.CategoryReview:      {rich: "review", compact: "review", utf8: "◇", ascii: "V"},
	domain.CategoryComment:     {rich: "comment", compact: "comment", utf8: "○", ascii: "C"},
	domain.CategoryOther:       {rich: "other", compact: "other", utf8: "·", ascii: "?"},
}

// categoryOrder is the closed vocabulary in its published order, so a legend
// and a diagnostic enumerate the categories identically on every run rather
// than in the map's iteration order.
var categoryOrder = []domain.Category{
	domain.CategoryPush,
	domain.CategoryPullRequest,
	domain.CategoryReview,
	domain.CategoryComment,
	domain.CategoryOther,
}

// normalizeCategory maps a source category onto the closed vocabulary. An
// absent, malformed, newly introduced, or otherwise unsupported value becomes
// the catch-all one rather than reaching a renderer unmapped (RG-008).
func normalizeCategory(category domain.Category) domain.Category {
	if _, supported := categoryVocabulary[category]; supported {
		return category
	}
	return domain.CategoryOther
}

// semanticsOf returns the prepared presentation of a source category.
func semanticsOf(category domain.Category) categorySemantics {
	return categoryVocabulary[normalizeCategory(category)]
}

// categoryText spells a source category in the given register.
func categoryText(category domain.Category, register categoryRegister) string {
	semantics := semanticsOf(category)
	if register == registerCompact {
		return semantics.compact
	}
	return semantics.rich
}

// categoryGlyph draws a source category in the given repertoire.
func categoryGlyph(category domain.Category, set charset) string {
	semantics := semanticsOf(category)
	if set == charsetUTF8 {
		return semantics.utf8
	}
	return semantics.ascii
}

// localeVariables are the locale environment variables in precedence order. The
// first non-empty one decides, exactly as POSIX resolves LC_CTYPE.
var localeVariables = []string{"LC_ALL", "LC_CTYPE", "LANG"}

// dumbTerminal is the TERM value of a terminal that renders no more than ASCII.
const dumbTerminal = "dumb"

// resolveCharset picks the glyph repertoire from the environment: UTF-8 only
// when the effective locale names a UTF-8 codeset and the terminal is not dumb.
// NO_COLOR is deliberately not consulted, because it changes color alone and
// never the already selected character set (RG-008).
func resolveCharset(lookup func(string) string) charset {
	if lookup("TERM") == dumbTerminal {
		return charsetASCII
	}
	for _, variable := range localeVariables {
		locale := lookup(variable)
		if locale == "" {
			continue
		}
		if isUTF8Codeset(locale) {
			return charsetUTF8
		}
		return charsetASCII
	}
	return charsetASCII
}

// isUTF8Codeset reports whether a locale names a UTF-8 codeset. The modifier is
// dropped first, and the codeset is then what follows the final `.` of what is
// left, or that whole remainder when the locale carries no codeset at all. So
// `C.UTF-8`, `en_US.UTF8@modifier`, and `en_US.UTF-8@currency.euro` all qualify,
// while a `.` inside a modifier never passes for a codeset separator.
func isUTF8Codeset(locale string) bool {
	codeset := locale
	if modifier := strings.Index(codeset, "@"); modifier >= 0 {
		codeset = codeset[:modifier]
	}
	if dot := strings.LastIndex(codeset, "."); dot >= 0 {
		codeset = codeset[dot+1:]
	}
	return strings.EqualFold(codeset, "UTF-8") || strings.EqualFold(codeset, "UTF8")
}
