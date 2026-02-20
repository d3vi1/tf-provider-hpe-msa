package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestResolveCloneSnapshot(t *testing.T) {
	cases := []struct {
		name        string
		snapshot    types.String
		expectErr   error
		expectValue string
	}{
		{name: "unknown", snapshot: types.StringUnknown(), expectErr: errCloneSnapshotUnknown},
		{name: "empty", snapshot: types.StringNull(), expectErr: errCloneSnapshotMissing},
		{name: "valid", snapshot: types.StringValue("snap01"), expectValue: "snap01"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := cloneResourceModel{SourceSnapshot: tc.snapshot}
			value, err := resolveCloneSnapshot(model)
			if tc.expectErr != nil {
				if err == nil {
					t.Fatalf("expected error")
				}
				if err != tc.expectErr {
					t.Fatalf("expected %v, got %v", tc.expectErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != tc.expectValue {
				t.Fatalf("expected %q, got %q", tc.expectValue, value)
			}
		})
	}
}

func TestCloneStateFromModelSCSIWWN(t *testing.T) {
	model := cloneResourceModel{}
	volume := &msa.Volume{
		Name:         "clone01",
		SerialNumber: "SNCLONE1",
		WWN:          "600c0ff0000000000000000000000002",
	}

	state := cloneStateFromModel(model, volume)
	if state.SCSIWWN.IsNull() || state.SCSIWWN.ValueString() != volume.WWN {
		t.Fatalf("expected scsi_wwn to be set from volume wwn")
	}

	volume.WWN = ""
	state = cloneStateFromModel(model, volume)
	if !state.SCSIWWN.IsNull() {
		t.Fatalf("expected scsi_wwn to be null when wwn missing")
	}
}

func TestCloneConflictRetryPlannerETAPath(t *testing.T) {
	planner := cloneConflictRetryPlanner{}
	job := &msa.VolumeCopyJob{
		HasETA: true,
		ETA:    2 * time.Minute,
	}

	for i := 0; i < cloneCopyConflictETAMaxRetries; i++ {
		wait, path, ok := planner.next(job)
		if !ok {
			t.Fatalf("expected retry on eta iteration %d", i)
		}
		if path != cloneRetryPathETA {
			t.Fatalf("expected eta path, got %s", path)
		}
		expectedWait := 2*time.Minute + cloneCopyETASafetyBuffer
		if wait != expectedWait {
			t.Fatalf("expected wait %s, got %s", expectedWait, wait)
		}
	}

	wait, path, ok := planner.next(job)
	if !ok {
		t.Fatalf("expected fallback retry after eta retries")
	}
	if path != cloneRetryPathNoETA {
		t.Fatalf("expected no-eta fallback path, got %s", path)
	}
	if wait != cloneCopyConflictNoETAWaits[0] {
		t.Fatalf("expected first no-eta wait %s, got %s", cloneCopyConflictNoETAWaits[0], wait)
	}

	for i := 1; i < len(cloneCopyConflictNoETAWaits); i++ {
		wait, path, ok = planner.next(nil)
		if !ok {
			t.Fatalf("expected no-eta retry on fallback iteration %d", i)
		}
		if path != cloneRetryPathNoETA {
			t.Fatalf("expected no-eta fallback path, got %s", path)
		}
		if wait != cloneCopyConflictNoETAWaits[i] {
			t.Fatalf("expected fallback wait %s, got %s", cloneCopyConflictNoETAWaits[i], wait)
		}
	}

	_, _, ok = planner.next(nil)
	if ok {
		t.Fatalf("expected retries to stop after eta and no-eta retries are exhausted")
	}
}

func TestCloneConflictRetryPlannerNoETAPath(t *testing.T) {
	planner := cloneConflictRetryPlanner{}

	for i, expected := range cloneCopyConflictNoETAWaits {
		wait, path, ok := planner.next(&msa.VolumeCopyJob{ID: "job-no-eta"})
		if !ok {
			t.Fatalf("expected retry on no-eta iteration %d", i)
		}
		if path != cloneRetryPathNoETA {
			t.Fatalf("expected no-eta path, got %s", path)
		}
		if wait != expected {
			t.Fatalf("expected wait %s, got %s", expected, wait)
		}
	}

	_, _, ok := planner.next(nil)
	if ok {
		t.Fatalf("expected no-eta retries to stop after %d retries", len(cloneCopyConflictNoETAWaits))
	}
}

func TestCloneConflictRetryPlannerSwitchesToETAWhenAvailable(t *testing.T) {
	planner := cloneConflictRetryPlanner{}

	wait, path, ok := planner.next(nil)
	if !ok {
		t.Fatalf("expected initial no-eta retry")
	}
	if path != cloneRetryPathNoETA {
		t.Fatalf("expected initial no-eta path, got %s", path)
	}
	if wait != cloneCopyConflictNoETAWaits[0] {
		t.Fatalf("expected initial wait %s, got %s", cloneCopyConflictNoETAWaits[0], wait)
	}

	job := &msa.VolumeCopyJob{
		HasETA: true,
		ETA:    4 * time.Minute,
	}
	expectedWait := 4*time.Minute + cloneCopyETASafetyBuffer
	for i := 0; i < cloneCopyConflictETAMaxRetries; i++ {
		wait, path, ok = planner.next(job)
		if !ok {
			t.Fatalf("expected eta retry after switch on iteration %d", i)
		}
		if path != cloneRetryPathETA {
			t.Fatalf("expected eta path after switch, got %s", path)
		}
		if wait != expectedWait {
			t.Fatalf("expected wait %s, got %s", expectedWait, wait)
		}
	}

	wait, path, ok = planner.next(job)
	if !ok {
		t.Fatalf("expected no-eta fallback after eta retries")
	}
	if path != cloneRetryPathNoETA {
		t.Fatalf("expected no-eta fallback path, got %s", path)
	}
	if wait != cloneCopyConflictNoETAWaits[1] {
		t.Fatalf("expected resumed no-eta wait %s, got %s", cloneCopyConflictNoETAWaits[1], wait)
	}
}

func TestCloneConflictRetryPlannerFallsBackWhenETAUnavailable(t *testing.T) {
	planner := cloneConflictRetryPlanner{}
	job := &msa.VolumeCopyJob{
		HasETA: true,
		ETA:    30 * time.Second,
	}

	wait, path, ok := planner.next(job)
	if !ok {
		t.Fatalf("expected initial eta retry")
	}
	if path != cloneRetryPathETA {
		t.Fatalf("expected eta path, got %s", path)
	}
	if wait != 30*time.Second+cloneCopyETASafetyBuffer {
		t.Fatalf("expected eta wait %s, got %s", 30*time.Second+cloneCopyETASafetyBuffer, wait)
	}

	wait, path, ok = planner.next(nil)
	if !ok {
		t.Fatalf("expected no-eta fallback when eta unavailable")
	}
	if path != cloneRetryPathNoETA {
		t.Fatalf("expected no-eta fallback path, got %s", path)
	}
	if wait != cloneCopyConflictNoETAWaits[0] {
		t.Fatalf("expected first no-eta wait %s, got %s", cloneCopyConflictNoETAWaits[0], wait)
	}
}

func TestCloneConflictContext(t *testing.T) {
	contextState := newCloneConflictContext("source-initial", "target-initial")
	if got := contextState.String(); !strings.Contains(got, "source=source-initial") || !strings.Contains(got, "target=target-initial") {
		t.Fatalf("unexpected initial context: %s", got)
	}

	contextState.update(&msa.VolumeCopyJob{
		ID:     "job-52",
		Source: "source-job",
		Target: "target-job",
		HasETA: true,
		ETA:    95 * time.Second,
	})

	got := contextState.String()
	if !strings.Contains(got, "job id=job-52") {
		t.Fatalf("expected job id in context, got %s", got)
	}
	if !strings.Contains(got, "source=source-job") {
		t.Fatalf("expected source in context, got %s", got)
	}
	if !strings.Contains(got, "target=target-job") {
		t.Fatalf("expected target in context, got %s", got)
	}
	if !strings.Contains(got, "eta=1m35s") {
		t.Fatalf("expected eta in context, got %s", got)
	}
}

func TestCloneCopyConflictErrorMatching(t *testing.T) {
	conflictErr := msa.APIError{
		Status: msa.Status{
			Response: "The operation cannot be completed because it conflicts with an existing volume copy in progress.",
		},
	}
	if !isCloneCopyConflictError(conflictErr) {
		t.Fatalf("expected conflict error match")
	}

	otherErr := msa.APIError{Status: msa.Status{Response: "Some other command failure"}}
	if isCloneCopyConflictError(otherErr) {
		t.Fatalf("did not expect conflict match")
	}
}

func TestSleepWithContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sleepWithContext(ctx, 5*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
