package cache

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// ErrContended reports a cache lock another process or refresh holds. A
// contended read is a miss and a contended write is skipped; OrgTop never waits
// in a tight loop and never opens a second cache.
var ErrContended = errors.New("enrichment cache is busy")

// retryInterval polls a contended region without spinning.
const retryInterval = 5 * time.Millisecond

// waits are the lock wait bounds one process runs on. Normal work waits at
// most 250 ms for a busy region, matching the SQLite busy limit; an explicit
// reset may wait two seconds.
type waits struct {
	// busy bounds ordinary lifecycle and admission acquisition, and is the
	// SQLite busy timeout the driver is opened with.
	busy time.Duration
	// reset bounds the explicit `--reset-cache` lifecycle acquisition, which
	// waits longer because the user asked for it.
	reset time.Duration
}

// defaultWaits are RG-005's closed production bounds.
func defaultWaits() waits {
	return waits{busy: 250 * time.Millisecond, reset: 2 * time.Second}
}

// lockWaits is the wait bound every cache lock acquisition in this process
// uses. Production always runs on defaultWaits; the test binary replaces it
// once, before any test runs, so a broken lock or budget fails the suite fast
// instead of holding a test open for the production bound.
var lockWaits = defaultWaits()

// The maintenance lock's two independent byte regions. The lifecycle region
// coordinates setup, rebuild, and reset; the admission region serializes
// mutating transactions across processes while readers hold it shared.
const (
	lifecycleRegion = 0
	admissionRegion = 1
)

// processLocks serializes lock regions inside one process. POSIX record locks
// are owned per process rather than per file description, so two goroutines
// would otherwise silently share one lock and release it for each other.
var processLocks sync.Map // absolute region name -> *sync.RWMutex

// lockFiles holds one descriptor per lock path for the whole process, counted
// by how many stores reference it. POSIX record locks belong to a process and
// an inode, not to a descriptor: closing any descriptor on that inode drops
// every lock the process holds through it. Sharing one counted descriptor is
// what keeps a second store's Close from silently releasing the admission
// region a first store still holds, which another process could then take.
var lockFiles = struct {
	mutex sync.Mutex
	open  map[string]*countedLockFile
}{open: map[string]*countedLockFile{}}

// countedLockFile is one process-wide lock descriptor and its reference count.
type countedLockFile struct {
	file       *os.File
	references int
}

// retainLockFile returns the process-wide descriptor for a lock path, opening it
// once and counting every later reference.
func retainLockFile(root *os.Root, path string) (*os.File, error) {
	lockFiles.mutex.Lock()
	defer lockFiles.mutex.Unlock()

	if shared, ok := lockFiles.open[path]; ok {
		shared.references++
		return shared.file, nil
	}
	file, err := openOwnedFile(root, lockName)
	if err != nil {
		return nil, err
	}
	lockFiles.open[path] = &countedLockFile{file: file, references: 1}
	return file, nil
}

// releaseLockFile drops one reference and closes the descriptor once the last
// store using it is gone, so no database or lock handle outlives the cache.
func releaseLockFile(path string) error {
	lockFiles.mutex.Lock()
	defer lockFiles.mutex.Unlock()

	shared, ok := lockFiles.open[path]
	if !ok {
		return nil
	}
	shared.references--
	if shared.references > 0 {
		return nil
	}
	delete(lockFiles.open, path)
	return shared.file.Close()
}

// maintenanceLock is the private sidecar lock file guarding the cache
// directory. It holds no cache data: it exists only so path mutation, setup,
// and mutating transactions cannot race another OrgTop process.
type maintenanceLock struct {
	file   *os.File
	path   string
	held   map[int]*heldRegion
	closed bool
	mutex  sync.Mutex
}

type heldRegion struct {
	guard     *sync.RWMutex
	exclusive bool
}

// openMaintenanceLock opens the lock file relative to the already validated
// cache directory. It creates the file with restrictive permissions and never
// follows a symlink into another object.
func openMaintenanceLock(root *os.Root, path string) (*maintenanceLock, error) {
	file, err := retainLockFile(root, path)
	if err != nil {
		return nil, err
	}
	return &maintenanceLock{file: file, path: path, held: map[int]*heldRegion{}}, nil
}

// acquire takes one region shared or exclusively, waiting at most the given
// bound. A region already held by this lock is a programming error, not a wait.
func (l *maintenanceLock) acquire(region int, exclusive bool, wait time.Duration) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if _, ok := l.held[region]; ok {
		return fmt.Errorf("cache lock region %d is already held", region)
	}

	guard := l.processGuard(region)
	if !acquireGuard(guard, exclusive, wait) {
		return fmt.Errorf("%w: another refresh holds the cache", ErrContended)
	}
	if err := lockRegion(l.file, region, exclusive, wait); err != nil {
		releaseGuard(guard, exclusive)
		return err
	}
	l.held[region] = &heldRegion{guard: guard, exclusive: exclusive}
	return nil
}

// release drops one held region. Releasing an unheld region is a no-op so
// deferred cleanup stays simple on every error path.
func (l *maintenanceLock) release(region int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	state, ok := l.held[region]
	if !ok {
		return
	}
	delete(l.held, region)
	_ = unlockRegion(l.file, region)
	releaseGuard(state.guard, state.exclusive)
}

// convert exchanges a held exclusive region for shared access. Setup takes the
// lifecycle region exclusively and keeps it shared for the store's lifetime.
func (l *maintenanceLock) convert(region int, wait time.Duration) error {
	l.release(region)
	return l.acquire(region, false, wait)
}

// close releases every held region and the lock file itself. No database handle
// may remain open past this point.
func (l *maintenanceLock) close() error {
	l.mutex.Lock()
	if l.closed {
		l.mutex.Unlock()
		return nil
	}
	l.closed = true
	regions := make([]int, 0, len(l.held))
	for region := range l.held {
		regions = append(regions, region)
	}
	l.mutex.Unlock()

	for _, region := range regions {
		l.release(region)
	}
	return releaseLockFile(l.path)
}

func (l *maintenanceLock) processGuard(region int) *sync.RWMutex {
	name := fmt.Sprintf("%s#%d", l.path, region)
	guard, _ := processLocks.LoadOrStore(name, &sync.RWMutex{})
	return guard.(*sync.RWMutex)
}

// acquireGuard polls the in-process guard until the wait bound expires. It never
// blocks indefinitely, so a stuck holder degrades the cache instead of the UI.
func acquireGuard(guard *sync.RWMutex, exclusive bool, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		if tryGuard(guard, exclusive) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(retryInterval)
	}
}

func tryGuard(guard *sync.RWMutex, exclusive bool) bool {
	if exclusive {
		return guard.TryLock()
	}
	return guard.TryRLock()
}

func releaseGuard(guard *sync.RWMutex, exclusive bool) {
	if exclusive {
		guard.Unlock()
		return
	}
	guard.RUnlock()
}
