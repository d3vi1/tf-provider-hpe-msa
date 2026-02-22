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
	"syscall"
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

type destroyLockOwnerMetadata struct {
	Owner string
	PID   int
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

		reclaimed, reclaimErr := tryReapStaleDestroyGlobalLock(ctx, lockDir, wait)
		if reclaimErr != nil {
			return nil, reclaimErr
		}
		if reclaimed {
			continue
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

func tryReapStaleDestroyGlobalLock(ctx context.Context, lockDir string, wait time.Duration) (bool, error) {
	lockInfo, err := os.Stat(lockDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat destroy global lock %q: %w", lockDir, err)
	}

	ownerFile := filepath.Join(lockDir, "owner")
	metadata, _ := readDestroyLockOwnerMetadata(ownerFile)

	reasons := make([]string, 0, 2)
	ownerAlive := false
	if metadata.PID > 0 {
		if processExists(metadata.PID) {
			ownerAlive = true
		} else {
			reasons = append(reasons, fmt.Sprintf("dead_pid=%d", metadata.PID))
		}
	}

	lockAge := time.Since(lockInfo.ModTime())
	if !ownerAlive && lockAge >= wait {
		reasons = append(reasons, fmt.Sprintf("age=%s", lockAge.Round(time.Second)))
	}

	if len(reasons) == 0 {
		return false, nil
	}

	_ = os.Remove(ownerFile)
	if err := os.Remove(lockDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EBUSY) {
			return false, nil
		}
		return false, fmt.Errorf("remove stale destroy global lock %q: %w", lockDir, err)
	}

	tflog.Warn(ctx, "reclaimed stale MSA destroy global lock", map[string]any{
		"lock_dir":   lockDir,
		"lock_owner": metadata.Owner,
		"lock_pid":   metadata.PID,
		"reasons":    strings.Join(reasons, ","),
	})
	return true, nil
}

func readDestroyLockOwnerMetadata(ownerFile string) (destroyLockOwnerMetadata, error) {
	data, err := os.ReadFile(ownerFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return destroyLockOwnerMetadata{}, nil
		}
		return destroyLockOwnerMetadata{}, fmt.Errorf("read destroy lock owner metadata %q: %w", ownerFile, err)
	}

	metadata := destroyLockOwnerMetadata{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "owner":
			metadata.Owner = value
		case "pid":
			pid, parseErr := strconv.Atoi(value)
			if parseErr == nil {
				metadata.PID = pid
			}
		}
	}

	return metadata, nil
}

func processExists(pid int) bool {
	if pid < 1 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
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
