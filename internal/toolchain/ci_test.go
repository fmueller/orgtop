package toolchain

import (
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestWorkflowPathLanesAreExactMirrors guards the two-lane split: ci.yml ignores
// the doc and planning paths that planning.yml claims, so every changed file
// lands in exactly one lane. If the two lists drift, files fall between the lanes
// and are validated by nothing at all — a failure mode with no other symptom.
func TestWorkflowPathLanesAreExactMirrors(t *testing.T) {
	t.Parallel()

	ci := loadYAML(t, ciWorkflow)
	planning := loadYAML(t, planningWorkflow)

	lanes := map[string][]string{
		"ci.yml pull_request paths-ignore": stringSlice(nodeAt(ci, "on", "pull_request", "paths-ignore")),
		"ci.yml push paths-ignore":         stringSlice(nodeAt(ci, "on", "push", "paths-ignore")),
		"planning.yml pull_request paths":  stringSlice(nodeAt(planning, "on", "pull_request", "paths")),
		"planning.yml push paths":          stringSlice(nodeAt(planning, "on", "push", "paths")),
	}

	want := sorted(lanes["ci.yml pull_request paths-ignore"])
	if len(want) == 0 {
		t.Fatal("ci.yml pull_request must declare a paths-ignore lane")
	}

	for name, got := range lanes {
		if !equalStrings(want, sorted(got)) {
			t.Errorf("%s must mirror the other path lanes exactly; update every list together\n got: %v\nwant: %v",
				name, sorted(got), want)
		}
	}
}

// TestPlanningLaneCoversCommittedPlanningPaths keeps the lane split honest as the
// repository grows: a planning or specification directory that no lane claims is
// gated by ci.yml, which is exactly the expensive matrix the split exists to
// avoid for prose-only changes.
func TestPlanningLaneCoversCommittedPlanningPaths(t *testing.T) {
	t.Parallel()

	lane := stringSlice(nodeAt(loadYAML(t, planningWorkflow), "on", "pull_request", "paths"))

	for _, required := range []string{
		".agents/skills/**",
		".claude/skills/**",
		".taskrail/**",
		"planning/**",
		"specs/**",
		"AGENTS.md",
		"CHANGELOG.md",
		"README.md",
	} {
		if !contains(lane, required) {
			t.Errorf("the planning lane must claim %q", required)
		}
	}
}

// TestCIDoesNotRunMutationTests keeps mutation testing off the pull-request path.
// gremlins re-runs the whole suite once per mutant, so reintroducing it here
// would quietly restore the slowest job in the repository.
func TestCIDoesNotRunMutationTests(t *testing.T) {
	t.Parallel()

	for _, command := range stepValues(loadYAML(t, ciWorkflow), "run") {
		if strings.Contains(command, "test:mutate") || strings.Contains(command, "gremlins") {
			t.Errorf("full mutation testing belongs in mutation.yml (weekly), not the CI lane: %q", command)
		}
	}
}

// TestMutationWorkflowIsScheduledOnly pins the scheduled-only policy from the
// other side: mutation.yml must never grow a push or pull_request trigger.
func TestMutationWorkflowIsScheduledOnly(t *testing.T) {
	t.Parallel()

	triggers := sorted(mappingKeys(nodeAt(loadYAML(t, mutationWorkflow), "on")))
	if !equalStrings(triggers, []string{"schedule", "workflow_dispatch"}) {
		t.Errorf("mutation.yml must run on a schedule and manual dispatch only, got %v", triggers)
	}
}

// TestMutationWorkflowIsWeeklyAndActionable keeps the full baseline cheap and
// ensures a failed scheduled gate leaves both durable evidence and owned work.
func TestMutationWorkflowIsWeeklyAndActionable(t *testing.T) {
	t.Parallel()

	workflow := loadYAML(t, mutationWorkflow)

	schedule := nodeAt(workflow, "on", "schedule")
	if schedule == nil || schedule.Kind != yaml.SequenceNode || len(schedule.Content) != 1 {
		t.Fatal("mutation testing must declare exactly one schedule entry")
	}
	if cron := child(schedule.Content[0], "cron"); cron == nil || cron.Value != "0 3 * * 1" {
		t.Error("full mutation testing must run weekly on Monday")
	}
	if issues := nodeAt(workflow, "permissions", "issues"); issues == nil || issues.Value != "write" {
		t.Error("mutation failures must be tracked through a GitHub issue")
	}

	var retained bool
	for _, uses := range stepValues(workflow, "uses") {
		if strings.HasPrefix(uses, "actions/upload-artifact@") {
			retained = true
		}
	}
	if !retained {
		t.Error("mutation results must be retained as an artifact")
	}
}

// TestCIDelegatesGoToTask keeps one command surface for local and CI runs: every
// build or test step goes through a Taskfile target, so reproducing a CI failure
// locally is always the same command that failed.
func TestCIDelegatesGoToTask(t *testing.T) {
	t.Parallel()

	rawGo := regexp.MustCompile(`(^|[\s;&|])go\s+(build|test|vet|run|install|generate)\b`)

	for _, path := range workflowPaths(t) {
		for _, command := range stepValues(loadYAML(t, path), "run") {
			if rawGo.MatchString(command) {
				t.Errorf("%s must invoke Go through a `task` target, not directly: %q", path, command)
			}
		}
	}
}

// TestWorkflowsProvisionToolchainViaSharedSetup keeps toolchain provisioning and
// the Go module and build caches in one place. A job that calls mise-action
// directly skips the cache and recompiles the dependency tree from scratch.
func TestWorkflowsProvisionToolchainViaSharedSetup(t *testing.T) {
	t.Parallel()

	for _, path := range workflowPaths(t) {
		workflow := loadYAML(t, path)

		for _, uses := range stepValues(workflow, "uses") {
			if strings.Contains(uses, "jdx/mise-action") {
				t.Errorf("%s must provision the toolchain via ./.github/actions/setup", path)
			}
			if strings.Contains(uses, "actions/setup-go") {
				t.Errorf("%s must not pin a second Go version alongside mise.toml", path)
			}
		}
	}
}

// TestActionReferencesArePinnedBySHA pins every third-party action to an
// immutable commit. A moving tag can change what runs in CI, including what
// provisions the toolchain, without any change landing in this repository.
func TestActionReferencesArePinnedBySHA(t *testing.T) {
	t.Parallel()

	pinned := regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)

	references := map[string][]string{}
	for _, path := range workflowPaths(t) {
		references[path] = stepValues(loadYAML(t, path), "uses")
	}
	references[setupAction] = compositeUses(t, loadYAML(t, setupAction))

	for path, uses := range references {
		for _, reference := range uses {
			// Local composite actions are versioned by the commit under test.
			if strings.HasPrefix(reference, "./") {
				continue
			}
			if !pinned.MatchString(reference) {
				t.Errorf("%s must pin %q to a full commit SHA", path, reference)
			}
		}
	}
}

// TestEveryJobDeclaresATimeout bounds the blast radius of a hung step: without a
// timeout a wedged job burns the default six hours of runner time before GitHub
// reclaims it.
func TestEveryJobDeclaresATimeout(t *testing.T) {
	t.Parallel()

	for _, path := range workflowPaths(t) {
		workflow := loadYAML(t, path)
		for _, name := range jobNames(workflow) {
			if child(jobAt(workflow, name), "timeout-minutes") == nil {
				t.Errorf("%s job %q must declare timeout-minutes", path, name)
			}
		}
	}
}

// TestMatrixJobsDoNotFailFast keeps a failing platform from cancelling its peers.
// Cross-platform breaks are usually platform-specific, and cancelling the other
// legs hides whether the failure is isolated or general.
func TestMatrixJobsDoNotFailFast(t *testing.T) {
	t.Parallel()

	for _, path := range workflowPaths(t) {
		workflow := loadYAML(t, path)
		for _, name := range jobNames(workflow) {
			strategy := child(jobAt(workflow, name), "strategy")
			if strategy == nil {
				continue
			}
			failFast := child(strategy, "fail-fast")
			if failFast == nil || failFast.Value != "false" {
				t.Errorf("%s job %q declares a matrix and must set fail-fast: false", path, name)
			}
		}
	}
}

// TestCIMatrixCoversEveryReleasePlatform keeps the tested platforms and the
// published platforms in step. A GOOS that ships without ever running its tests
// is discovered by users, not by CI — Windows most of all, where the terminal
// handling behind the TUI diverges most from the Linux development host.
func TestCIMatrixCoversEveryReleasePlatform(t *testing.T) {
	t.Parallel()

	matrix := nodeAt(loadYAML(t, ciWorkflow), "jobs", "build-test", "strategy", "matrix", "runner")
	if matrix == nil {
		t.Fatal("the build-test job must declare a runner matrix")
	}

	for _, runner := range []string{"ubuntu-latest", "ubuntu-24.04-arm", "windows-latest", "macos-latest"} {
		if !strings.Contains(matrix.Value, runner) {
			t.Errorf("the build-test matrix must cover %q", runner)
		}
	}
}

// TestCrossCompileCoversEveryReleaseTarget catches a GOOS/GOARCH pair that
// .goreleaser.yml publishes but nothing ever compiles between releases, which
// turns a routine tag push into a failed release.
func TestCrossCompileCoversEveryReleaseTarget(t *testing.T) {
	t.Parallel()

	release := loadYAML(t, goreleaserConfig)
	builds := child(release, "builds")
	if builds == nil || len(builds.Content) == 0 {
		t.Fatal(".goreleaser.yml must declare at least one build")
	}

	tasks := readFile(t, taskfile)
	for _, build := range builds.Content {
		for _, goos := range stringSlice(child(build, "goos")) {
			for _, goarch := range stringSlice(child(build, "goarch")) {
				pair := "GOOS=" + goos + " GOARCH=" + goarch
				if !strings.Contains(tasks, pair) {
					t.Errorf("task build:cross must compile %s, which .goreleaser.yml publishes", pair)
				}
			}
		}
	}
}

// TestReleaseWorkflowGuardsChangelogNotes keeps the publish path from shipping a
// release with an empty body. GoReleaser has its own changelog disabled, so the
// CHANGELOG section is the only source of release notes.
func TestReleaseWorkflowGuardsChangelogNotes(t *testing.T) {
	t.Parallel()

	commands := strings.Join(stepValues(loadYAML(t, releaseWorkflow), "run"), "\n")

	for _, guard := range []string{
		"scripts/check-changelog-version.sh",
		"scripts/changelog-release-notes.sh",
		"--release-notes=",
	} {
		if !strings.Contains(commands, guard) {
			t.Errorf("the release workflow must run %q before publishing", guard)
		}
	}

	if changelog := nodeAt(loadYAML(t, goreleaserConfig), "changelog", "disable"); changelog == nil || changelog.Value != "true" {
		t.Error("GoReleaser's own changelog must stay disabled while CHANGELOG.md is the source of release notes")
	}
}

// TestLocalCheckMirrorsTheCIChecksJob keeps `task check` a faithful local rehearsal
// of the CI checks job. A gate that only exists in the workflow is one a
// contributor first meets after pushing.
func TestLocalCheckMirrorsTheCIChecksJob(t *testing.T) {
	t.Parallel()

	tasks := readFile(t, taskfile)
	start := strings.Index(tasks, "\n  check:\n")
	if start < 0 {
		t.Fatal("Taskfile.yml must declare a `check` target")
	}
	body := tasks[start:]
	if end := strings.Index(body[1:], "\n  clean:"); end >= 0 {
		body = body[:end+1]
	}

	for _, target := range jobStepValues(loadYAML(t, ciWorkflow), "checks", "run") {
		target = strings.TrimSpace(strings.TrimPrefix(target, "task "))
		if target == "" {
			continue
		}
		if !strings.Contains(body, "task: "+target) {
			t.Errorf("`task check` must run %q so the local gate mirrors CI", target)
		}
	}
}

// TestGremlinsIsInstalledOnDemand prevents an occasional mutation tool from
// increasing every local setup and every CI job's shared provisioning time.
func TestGremlinsIsInstalledOnDemand(t *testing.T) {
	t.Parallel()

	if strings.Contains(readFile(t, miseConfig), "go-gremlins") {
		t.Error("gremlins must not be installed by the shared mise toolchain")
	}

	tasks := readFile(t, taskfile)
	for _, required := range []string{
		"go run github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0 unleash",
		"--workers 2",
		"go clean -testcache",
	} {
		if !strings.Contains(tasks, required) {
			t.Errorf("the mutation tasks must keep %q", required)
		}
	}
}
