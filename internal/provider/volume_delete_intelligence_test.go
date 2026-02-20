package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
)

type fakeVolumeDeleteProbeClient struct {
	results map[string]fakeVolumeDeleteProbeResult
}

type fakeVolumeDeleteProbeResult struct {
	response msa.Response
	err      error
}

func (f fakeVolumeDeleteProbeClient) Execute(_ context.Context, parts ...string) (msa.Response, error) {
	key := strings.Join(parts, " ")
	if result, ok := f.results[key]; ok {
		return result.response, result.err
	}

	return msa.Response{}, msa.APIError{
		Status: msa.Status{
			Response: "Invalid command",
		},
	}
}

func TestPreDeleteVolumeUsageGuardrailMapped(t *testing.T) {
	client := fakeVolumeDeleteProbeClient{
		results: map[string]fakeVolumeDeleteProbeResult{
			"show maps volume vol-data-01": {
				response: msa.Response{
					Objects: []msa.Object{
						{
							BaseType: "host-view-mappings",
							Name:     "volume-view",
							Properties: []msa.Property{
								{Name: "volume", Value: "vol-data-01"},
								{Name: "volume-serial", Value: "00c0ff3cab9c00000000000002010000"},
								{Name: "access", Value: "read-write"},
								{Name: "lun", Value: "12"},
							},
						},
					},
				},
			},
		},
	}

	guardrail, ok := preDeleteVolumeUsageGuardrail(context.Background(), client, "volume", "vol-data-01")
	if !ok {
		t.Fatalf("expected mapped guardrail")
	}
	if guardrail.summary != "Volume deletion blocked: mapped" {
		t.Fatalf("unexpected summary: %s", guardrail.summary)
	}
	if guardrail.retryable {
		t.Fatalf("expected mapped guardrail to be terminal")
	}
	if !strings.Contains(guardrail.detail, "Classification: terminal") {
		t.Fatalf("expected terminal classification, got %s", guardrail.detail)
	}
	if !strings.Contains(guardrail.detail, "`show maps volume vol-data-01`") {
		t.Fatalf("expected mapping command in detail, got %s", guardrail.detail)
	}
}

func TestPreDeleteVolumeUsageGuardrailActiveCopy(t *testing.T) {
	client := fakeVolumeDeleteProbeClient{
		results: map[string]fakeVolumeDeleteProbeResult{
			"show volume-copy": {
				response: msa.Response{
					Objects: []msa.Object{
						{
							BaseType: "volume-copy",
							Name:     "volume-copy",
							Properties: []msa.Property{
								{Name: "copy-job-id", Value: "job-52"},
								{Name: "source-volume-name", Value: "vol-data-01"},
								{Name: "destination-volume-name", Value: "clone-01"},
								{Name: "copy-status", Value: "In Progress"},
								{Name: "estimated-time-remaining", Value: "120"},
							},
						},
					},
				},
			},
		},
	}

	guardrail, ok := preDeleteVolumeUsageGuardrail(context.Background(), client, "volume", "vol-data-01")
	if !ok {
		t.Fatalf("expected active copy guardrail")
	}
	if guardrail.summary != "Volume deletion blocked: active copy" {
		t.Fatalf("unexpected summary: %s", guardrail.summary)
	}
	if !guardrail.retryable {
		t.Fatalf("expected active copy guardrail to be retryable")
	}
	if !strings.Contains(guardrail.detail, "Classification: retryable") {
		t.Fatalf("expected retryable classification, got %s", guardrail.detail)
	}
	if !strings.Contains(guardrail.detail, "job id=job-52") {
		t.Fatalf("expected job id in detail, got %s", guardrail.detail)
	}
}

func TestPreDeleteVolumeUsageGuardrailActiveConnection(t *testing.T) {
	client := fakeVolumeDeleteProbeClient{
		results: map[string]fakeVolumeDeleteProbeResult{
			"show volume-copy": {
				response: msa.Response{},
			},
			"show connections volume vol-data-01": {
				response: msa.Response{
					Objects: []msa.Object{
						{
							BaseType: "connection",
							Name:     "connection",
							Properties: []msa.Property{
								{Name: "volume-name", Value: "vol-data-01"},
								{Name: "connection-status", Value: "Connected"},
								{Name: "host-name", Value: "app-host-01"},
								{Name: "session-id", Value: "session-9"},
							},
						},
					},
				},
			},
		},
	}

	guardrail, ok := preDeleteVolumeUsageGuardrail(context.Background(), client, "volume", "vol-data-01")
	if !ok {
		t.Fatalf("expected active session guardrail")
	}
	if guardrail.summary != "Volume deletion blocked: active sessions" {
		t.Fatalf("unexpected summary: %s", guardrail.summary)
	}
	if !guardrail.retryable {
		t.Fatalf("expected active sessions guardrail to be retryable")
	}
	if !strings.Contains(guardrail.detail, "Classification: retryable") {
		t.Fatalf("expected retryable classification, got %s", guardrail.detail)
	}
}

func TestPreDeleteVolumeUsageGuardrailFallbackOnProbeError(t *testing.T) {
	client := fakeVolumeDeleteProbeClient{
		results: map[string]fakeVolumeDeleteProbeResult{
			"show maps volume vol-data-01": {
				err: errors.New("dial tcp timeout"),
			},
		},
	}

	if guardrail, ok := preDeleteVolumeUsageGuardrail(context.Background(), client, "volume", "vol-data-01"); ok {
		t.Fatalf("expected fallback without guardrail, got %s: %s", guardrail.summary, guardrail.detail)
	}
}

func TestClassifyVolumeDeleteErrorActiveCopyRetryable(t *testing.T) {
	err := msa.APIError{
		Status: msa.Status{
			Response: "Delete cannot proceed because an existing volume copy is in progress.",
		},
	}

	guardrail, ok := classifyVolumeDeleteError("volume", "vol-data-01", err)
	if !ok {
		t.Fatalf("expected copy guardrail")
	}
	if guardrail.summary != "Volume deletion blocked: active copy" {
		t.Fatalf("unexpected summary: %s", guardrail.summary)
	}
	if !guardrail.retryable {
		t.Fatalf("expected retryable guardrail")
	}
	if !strings.Contains(guardrail.detail, "Classification: retryable") {
		t.Fatalf("expected retryable classification, got %s", guardrail.detail)
	}
}

func TestClassifyVolumeDeleteErrorActiveSessionsRetryable(t *testing.T) {
	err := msa.APIError{
		Status: msa.Status{
			Response: "The volume has active sessions and cannot be deleted while hosts are connected.",
		},
	}

	guardrail, ok := classifyVolumeDeleteError("clone", "clone-app-01", err)
	if !ok {
		t.Fatalf("expected session guardrail")
	}
	if guardrail.summary != "Clone deletion blocked: active sessions" {
		t.Fatalf("unexpected summary: %s", guardrail.summary)
	}
	if !guardrail.retryable {
		t.Fatalf("expected retryable guardrail")
	}
	if !strings.Contains(guardrail.detail, "Classification: retryable") {
		t.Fatalf("expected retryable classification, got %s", guardrail.detail)
	}
}
