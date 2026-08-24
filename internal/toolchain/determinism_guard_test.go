package toolchain

import (
	"go/parser"
	"go/token"
	"slices"
	"testing"
)

// TestHostClockReadsInSeesThroughEveryImportForm guards the guard: the walk in
// TestNoTestReadsTheHostClock is only as good as the detection under it, and a
// detector that keys off the literal identifier "time" lets a renamed or
// dot-imported time package reintroduce exactly the nondeterminism NFR-006
// forbids, silently and with no failing test to catch it.
func TestHostClockReadsInSeesThroughEveryImportForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "a plain import",
			source: "package p\nimport \"time\"\nfunc TestX(t *testing.T) { _ = time.Now() }\n",
			want:   []string{"time.Now"},
		},
		{
			name:   "a renamed import",
			source: "package p\nimport clock \"time\"\nfunc TestX(t *testing.T) { _ = clock.Since(anchor) }\n",
			want:   []string{"clock.Since"},
		},
		{
			name:   "a dot import",
			source: "package p\nimport . \"time\"\nfunc TestX(t *testing.T) { _ = Now() }\n",
			want:   []string{"Now"},
		},
		{
			name:   "a read outside any function",
			source: "package p\nimport \"time\"\nvar anchor = time.Now()\n",
			want:   []string{"time.Now"},
		},
		{
			name:   "a benchmark, where elapsed real time is the point",
			source: "package p\nimport \"time\"\nfunc BenchmarkX(b *testing.B) { _ = time.Now() }\n",
			want:   nil,
		},
		{
			name:   "a same-named method on another type",
			source: "package p\nimport \"time\"\nfunc TestX(t *testing.T) { _ = clock.Now() }\n",
			want:   nil,
		},
		{
			name:   "a bare call without the dot import that would bind it",
			source: "package p\nimport \"time\"\nfunc TestX(t *testing.T) { _ = Now() }\n",
			want:   nil,
		},
		{
			name:   "a file that never imports time",
			source: "package p\nfunc TestX(t *testing.T) { _ = time.Now() }\n",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, "guard_probe_test.go", tt.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe failed: %v", err)
			}

			found := hostClockReadsIn(fileSet, file)
			expressions := make([]string, 0, len(found))
			for _, read := range found {
				expressions = append(expressions, read.expression)
			}
			if !slices.Equal(expressions, tt.want) {
				t.Errorf("hostClockReadsIn found %v, want %v", expressions, tt.want)
			}
		})
	}
}
