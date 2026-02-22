package provider

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAcquireDestroyGlobalLockWithOptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lockDir := filepath.Join(t.TempDir(), "destroy-lock.d")

	lock, err := acquireDestroyGlobalLockWithOptions(ctx, "test-owner", lockDir, 2*time.Second)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}

	ownerRaw, err := os.ReadFile(filepath.Join(lockDir, "owner"))
	if err != nil {
		t.Fatalf("read owner file: %v", err)
	}
	ownerData := string(ownerRaw)
	if !strings.Contains(ownerData, "owner=test-owner") {
		t.Fatalf("owner file missing owner, got: %q", ownerData)
	}

	if err := lock.Release(ctx); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	if _, err := os.Stat(lockDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("lock dir should be removed, stat err=%v", err)
	}
}

func TestAcquireDestroyGlobalLockWithOptionsTimeout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lockDir := filepath.Join(t.TempDir(), "destroy-lock-timeout.d")

	lock, err := acquireDestroyGlobalLockWithOptions(ctx, "first-owner", lockDir, 2*time.Second)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	defer func() {
		_ = lock.Release(ctx)
	}()

	start := time.Now()
	_, err = acquireDestroyGlobalLockWithOptions(ctx, "second-owner", lockDir, 1100*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout acquiring second lock")
	}
	if !strings.Contains(err.Error(), "timeout acquiring destroy global lock") {
		t.Fatalf("unexpected timeout error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Fatalf("expected lock wait to retry for at least 1s, elapsed=%s", elapsed)
	}
}

func TestAcquireDestroyGlobalLockWithOptionsReacquire(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lockDir := filepath.Join(t.TempDir(), "destroy-lock-reacquire.d")

	first, err := acquireDestroyGlobalLockWithOptions(ctx, "owner-1", lockDir, 2*time.Second)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	if err := first.Release(ctx); err != nil {
		t.Fatalf("release first lock: %v", err)
	}

	second, err := acquireDestroyGlobalLockWithOptions(ctx, "owner-2", lockDir, 2*time.Second)
	if err != nil {
		t.Fatalf("acquire second lock: %v", err)
	}
	if err := second.Release(ctx); err != nil {
		t.Fatalf("release second lock: %v", err)
	}
}

func TestAcquireDestroyGlobalLockWithOptionsReclaimsStaleByAge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lockDir := filepath.Join(t.TempDir(), "destroy-lock-stale-age.d")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("create stale lock dir: %v", err)
	}

	ownerFile := filepath.Join(lockDir, "owner")
	staleOwner := "owner=stale-owner\nacquired_at=2026-02-22T00:00:00Z\npid=999999\n"
	if err := os.WriteFile(ownerFile, []byte(staleOwner), 0o600); err != nil {
		t.Fatalf("write stale owner: %v", err)
	}

	old := time.Now().Add(-5 * time.Second)
	if err := os.Chtimes(lockDir, old, old); err != nil {
		t.Fatalf("set stale lock mtime: %v", err)
	}
	if err := os.Chtimes(ownerFile, old, old); err != nil {
		t.Fatalf("set stale owner mtime: %v", err)
	}

	lock, err := acquireDestroyGlobalLockWithOptions(ctx, "new-owner", lockDir, 1*time.Second)
	if err != nil {
		t.Fatalf("acquire lock after stale reclaim: %v", err)
	}
	defer func() {
		_ = lock.Release(ctx)
	}()

	ownerRaw, err := os.ReadFile(ownerFile)
	if err != nil {
		t.Fatalf("read owner after stale reclaim: %v", err)
	}
	if !strings.Contains(string(ownerRaw), "owner=new-owner") {
		t.Fatalf("owner file not replaced after stale reclaim: %q", string(ownerRaw))
	}
}

func TestAcquireDestroyGlobalLockWithOptionsReclaimsStaleDeadPID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	lockDir := filepath.Join(t.TempDir(), "destroy-lock-stale-pid.d")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("create stale lock dir: %v", err)
	}

	ownerFile := filepath.Join(lockDir, "owner")
	nonexistentPID := 999999
	ownerContent := strings.Join([]string{
		"owner=stale-owner",
		"acquired_at=2026-02-22T00:00:00Z",
		"pid=" + strconv.Itoa(nonexistentPID),
		"",
	}, "\n")
	if err := os.WriteFile(ownerFile, []byte(ownerContent), 0o600); err != nil {
		t.Fatalf("write stale owner: %v", err)
	}

	lock, err := acquireDestroyGlobalLockWithOptions(ctx, "fresh-owner", lockDir, 3*time.Second)
	if err != nil {
		t.Fatalf("acquire lock after dead pid reclaim: %v", err)
	}
	defer func() {
		_ = lock.Release(ctx)
	}()

	ownerRaw, err := os.ReadFile(ownerFile)
	if err != nil {
		t.Fatalf("read owner after dead pid reclaim: %v", err)
	}
	if !strings.Contains(string(ownerRaw), "owner=fresh-owner") {
		t.Fatalf("owner file not replaced after dead pid reclaim: %q", string(ownerRaw))
	}
}
