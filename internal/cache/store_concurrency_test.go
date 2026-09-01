package cache

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

// holdEnvironment names the environment variable that turns this test binary
// into the second process of the cross-process locking proof.
const (
	holdEnvironment = "ORGTOP_CACHE_LOCK_HOLDER"
	holdDuration    = 2 * time.Second
)

// TestMain lets the concurrency proof re-execute this binary as an independent
// process. POSIX record locks are owned per process, so only a real second
// process can prove that two OrgTop launches cannot both mutate the cache.
func TestMain(m *testing.M) {
	if root := os.Getenv(holdEnvironment); root != "" {
		os.Exit(holdExclusiveAdmission(root))
	}
	if root := os.Getenv(contendEnvironment); root != "" {
		os.Exit(attemptMutation(root))
	}
	os.Exit(m.Run())
}

// holdExclusiveAdmission opens the cache at root and holds exclusive admission
// for a fixed interval, then exits.
func holdExclusiveAdmission(root string) int {
	store, err := Open(LocationIn(root))
	if err != nil {
		return 2
	}
	defer func() { _ = store.Close() }()

	if err := store.lock.acquire(admissionRegion, true, busyWait); err != nil {
		return 3
	}
	defer store.lock.release(admissionRegion)

	// Signal readiness by creating a marker the parent polls for.
	if err := os.WriteFile(root+"/holding", nil, 0o600); err != nil {
		return 4
	}
	time.Sleep(holdDuration)
	return 0
}

// TestConcurrentProcessesCannotBothMutate proves the exclusive admission region
// serializes mutating transactions across processes: the second process is
// contended, stays interactive, and never opens a second cache.
func TestConcurrentProcessesCannotBothMutate(t *testing.T) {
	root := t.TempDir()

	holder := exec.Command(os.Args[0], "-test.run", "TestMain")
	holder.Env = append(os.Environ(), holdEnvironment+"="+root)
	if err := holder.Start(); err != nil {
		t.Fatalf("start holder error = %v", err)
	}
	t.Cleanup(func() {
		_ = holder.Process.Kill()
		_ = holder.Wait()
	})
	waitForFile(t, root+"/holding")

	store, err := Open(LocationIn(root))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	// The write must give up well before the holder releases the region, so a
	// contended process stays interactive instead of blocking on another one.
	err = awaitBounded(t, func() error { return store.Save(context.Background(), compareEntry(t)) })
	if !errors.Is(err, ErrContended) {
		t.Fatalf("Save() error = %v, want ErrContended", err)
	}
}

// awaitBounded runs one cache operation and fails the test unless it returns
// well inside the interval the holder keeps the region. It measures nothing, so
// the proof does not depend on how fast the host runs.
func awaitBounded(t *testing.T, operation func() error) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- operation() }()
	select {
	case err := <-done:
		return err
	case <-time.After(holdDuration / 2):
		t.Fatal("the cache operation did not give up within its busy bound")
		return nil
	}
}

// TestContendedAdmissionMakesAReadAMiss proves a contended lock never turns into
// an unbounded wait or a partial hit: the read is simply a miss for this
// refresh.
func TestContendedAdmissionMakesAReadAMiss(t *testing.T) {
	t.Parallel()

	store, location := fixedClockStore(t)
	entry := compareEntry(t, "src/main.go")
	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	other, err := Open(location)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer func() { _ = other.Close() }()

	if err := other.lock.acquire(admissionRegion, true, busyWait); err != nil {
		t.Fatalf("acquire admission error = %v", err)
	}
	defer other.lock.release(admissionRegion)

	var ok bool
	err = awaitBounded(t, func() error {
		var lookupErr error
		_, ok, lookupErr = store.Lookup(context.Background(), entry.Key)
		return lookupErr
	})
	if ok {
		t.Errorf("Lookup() ok = true, want a contended miss")
	}
	if !errors.Is(err, ErrContended) {
		t.Errorf("Lookup() error = %v, want ErrContended", err)
	}
}

// TestStoreRecoversAnUncleanShutdown proves a database left with a live WAL by a
// killed process is recovered on the next launch and its committed records are
// still complete evidence.
func TestStoreRecoversAnUncleanShutdown(t *testing.T) {
	t.Parallel()

	location := LocationIn(t.TempDir())
	store, err := Open(location)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	store = store.WithClock(func() time.Time { return referenceTime })
	entry := compareEntry(t, "src/main.go")
	if err := store.Save(context.Background(), entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	// Drop every handle without a clean close, as a killed process would.
	if err := store.lock.close(); err != nil {
		t.Fatalf("lock close error = %v", err)
	}

	recovered, err := Open(location)
	if err != nil {
		t.Fatalf("Open(after crash) error = %v", err)
	}
	defer func() { _ = recovered.Close() }()
	recovered = recovered.WithClock(func() time.Time { return referenceTime })

	got, ok, err := recovered.Lookup(context.Background(), entry.Key)
	if err != nil || !ok {
		t.Fatalf("Lookup() ok = %v, error = %v, want the committed record", ok, err)
	}
	if len(got.Paths) != 1 || got.Paths[0].String() != "src/main.go" {
		t.Errorf("Paths = %v, want [src/main.go]", got.Paths)
	}
}

// waitForFile polls for the holder process's readiness marker a bounded number
// of times, so the proof never reads the host clock to decide when to give up.
func waitForFile(t *testing.T, path string) {
	t.Helper()

	const attempts = 1000
	for attempt := 0; attempt < attempts; attempt++ {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the holder process never signalled readiness at %q", path)
}
