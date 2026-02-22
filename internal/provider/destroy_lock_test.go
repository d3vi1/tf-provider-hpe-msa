package provider

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
