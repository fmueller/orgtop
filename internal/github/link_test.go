package github

import (
	"net/http"
	"testing"
)

func TestLinkRelationDelimiters(t *testing.T) {
	const target = "https://api.github.com/repos/acme/api/commits/abc?per_page=100&page=2"
	for _, test := range []struct {
		name, value, want string
		present           bool
	}{
		{"next uri delimiters", `<https://example.com/a,b;c>; rel="next"`, "https://example.com/a,b;c", true},
		{"uri delimiters", `<https://example.com/a,b;c>; rel="last", <` + target + `>; title="two,; pages"; rel="next"`, target, true},
		{"next before last", `<` + target + `>; rel="next"; title="two,; pages", <https://example.com/a>; rel="last"`, target, true},
		{"escaped backslash", `<` + target + `>; title="two\\"; rel="prev next"`, target, true},
		{"title brackets", `<` + target + `>; title="<two>,; pages"; rel="NEXT"`, target, true},
		{"title is not a relation", `<` + target + `>; title="a; rel=\"next\", b"; rel="last"`, "", false},
		{"ambiguous entries", `<` + target + `>; title="two, pages"; rel="next", <` + target + `>; rel="next"`, "", true},
		{"ambiguous parameters", `<` + target + `>; title="two; pages"; rel="next"; rel="next"`, "", true},
		{"control byte", `<` + target + ">; title=\"two\x01 pages\"; rel=\"next\"", "", true},
		{"delete byte", `<` + target + ">; title=\"two\x7f pages\"; rel=\"next\"", "", true},
		{"tab whitespace", "<" + target + ">;\t title=\"two\t pages\";\t rel=\"next\"", target, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, present := linkRelation(http.Header{"Link": {test.value}}, "next")
			if got != test.want || present != test.present {
				t.Fatalf("linkRelation = %q, %v; want %q, %v", got, present, test.want, test.present)
			}
		})
	}
	got, present := linkRelation(http.Header{"Link": {`<` + target + `>; title="two, pages"; rel="next"`, `<` + target + `>; rel="next"`}}, "next")
	if got != "" || !present {
		t.Fatalf("duplicate header fields = %q, %v; want empty, true", got, present)
	}
}
