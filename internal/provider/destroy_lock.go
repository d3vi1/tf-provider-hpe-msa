package provider

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	defaultDestroyGlobalLockDir   = "/tmp/xconnector-directlun-destroy-global.lock.d"
	defaultDestroyGlobalLockWait  = 10 * time.Minute
	destroyGlobalLockPollInterval = 1 * time.Second
)

type destroyGlobalLock struct {
	dir        string
	ownerFile  string
	owner      string
	acquiredAt time.Time
}

func acquireDestroyGlobalLock(ctx context.Context, owner string) (*destroyGlobalLock, error) {
	lockDir, wait, err := destroyGlobalLockSettings()
	if err != nil {
		return nil, err
	}
	return acquireDestroyGlobalLockWithOptions(ctx, owner, lockDir, wait)
}

func acquireDestroyGlobalLockWithOptions(ctx context.Context, owner, lockDir string, wait time.Duration) (*destroyGlobalLock, error) {
	lockDir = strings.TrimSpace(lockDir)
	if lockDir == "" {
		return nil, errors.New("destroy global lock directory is empty")
	}
	if wait < time.Second {
		return nil, fmt.Errorf("destroy global lock wait must be at least 1s (got %s)", wait)
	}

	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "unknown"
	}

	if err := os.MkdirAll(filepath.Dir(lockDir), 0o755); err != nil {
		return nil, fmt.Errorf("prepare destroy global lock parent directory: %w", err)
	}

	deadline := time.Now().Add(wait)
	for {
		err := os.Mkdir(lockDir, 0o700)
		if err == nil {
			lock := &destroyGlobalLock{
				dir:        lockDir,
				ownerFile:  filepath.Join(lockDir, "owner"),
				owner:      owner,
				acquiredAt: time.Now().UTC(),
			}
			_ = os.WriteFile(lock.ownerFile, []byte(fmt.Sprintf(
				"owner=%s\nacquired_at=%s\npid=%d\n",
				lock.owner,
				lock.acquiredAt.Format(time.RFC3339),
				os.Getpid(),
			)), 0o600)
			tflog.Info(ctx, "acquired MSA destroy global lock", map[string]any{
				"lock_dir":    lock.dir,
				"lock_owner":  lock.owner,
				"acquired_at": lock.acquiredAt.Format(time.RFC3339),
			})
			return lock, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("create destroy global lock directory %q: %w", lockDir, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout acquiring destroy global lock %q for owner %q after %s", lockDir, owner, wait)
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context canceled while waiting for destroy global lock %q: %w", lockDir, ctx.Err())
		case <-time.After(destroyGlobalLockPollInterval):
		}
	}
}

func (lock *destroyGlobalLock) Release(ctx context.Context) error {
	if lock == nil {
		return nil
	}
	if err := os.Remove(lock.ownerFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove destroy lock owner file %q: %w", lock.ownerFile, err)
	}
	if err := os.Remove(lock.dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove destroy lock directory %q: %w", lock.dir, err)
	}
	tflog.Info(ctx, "released MSA destroy global lock", map[string]any{
		"lock_dir":    lock.dir,
		"lock_owner":  lock.owner,
		"released_at": time.Now().UTC().Format(time.RFC3339),
	})
	return nil
}

func destroyGlobalLockSettings() (string, time.Duration, error) {
	lockDir := strings.TrimSpace(os.Getenv("HPE_MSA_DESTROY_GLOBAL_LOCK_DIR"))
	if lockDir == "" {
		lockDir = defaultDestroyGlobalLockDir
	}

	wait := defaultDestroyGlobalLockWait
	if raw := strings.TrimSpace(os.Getenv("HPE_MSA_DESTROY_GLOBAL_LOCK_WAIT_SECONDS")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 1 {
			return "", 0, fmt.Errorf("invalid HPE_MSA_DESTROY_GLOBAL_LOCK_WAIT_SECONDS=%q (must be integer >= 1)", raw)
		}
		wait = time.Duration(seconds) * time.Second
	}

	return lockDir, wait, nil
}
