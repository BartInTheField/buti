package review

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	lockTimeout = 5 * time.Second
	lockPoll    = 5 * time.Millisecond
)

// lock takes an exclusive OS lock on the lock file next to path, creating the directory if needed, and returns the
// function that releases it. The OS drops the lock when a writer dies, so a crash never leaves the store locked.
func lock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	name := path + ".lock"
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(lockTimeout)
	for {
		ok, err := tryLock(f)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("locking %s: %w", name, err)
		}
		if ok {
			return func() {
				unlockFile(f)
				_ = f.Close()
			}, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("timed out waiting for the lock on %s", name)
		}
		time.Sleep(lockPoll)
	}
}
