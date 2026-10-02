package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LockProfile serializes credential writers and maintenance processes. OS locks
// are released on process exit; no stale PID file can own a profile forever.
func LockProfile(ctx context.Context, home, name string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name != "credentials" && name != "renewal" && name != "keepalive" && name != "browser" {
		return nil, fmt.Errorf("invalid profile lock")
	}
	if err := os.MkdirAll(home, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(home, name+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		ok, err := tryFileLock(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if ok {
			return func() { _ = f.Close() }, nil
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = f.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func credentialLock(home string) (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return LockProfile(ctx, home, "credentials")
}
