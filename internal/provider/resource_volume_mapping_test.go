package provider

import (
	"context"
	"testing"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBuildTargetSpec(t *testing.T) {
	type testCase struct {
		targetType string
		targetName string
		expected   string
	}

	cases := []testCase{
		{targetType: "host", targetName: "Host1", expected: "Host1.*"},
		{targetType: "host_group", targetName: "Group1", expected: "Group1.*.*"},
		{targetType: "initiator", targetName: "500605b00cf9a660", expected: "500605b00cf9a660"},
	}

	for _, tc := range cases {
		result, diags := buildTargetSpec(stringValueOrNull(tc.targetType), stringValueOrNull(tc.targetName))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics for %v: %v", tc, diags)
		}
		if result != tc.expected {
			t.Fatalf("expected %q, got %q", tc.expected, result)
		}
	}
}

func TestBuildTargetSpecInvalidHostGroupName(t *testing.T) {
	_, diags := buildTargetSpec(stringValueOrNull("host_group"), stringValueOrNull("bad,name"))
	if !diags.HasError() {
		t.Fatalf("expected diagnostics for invalid host_group name")
	}
}

func TestNormalizeAccess(t *testing.T) {
	cases := map[string]string{
		"rw":         "read-write",
		"read-write": "read-write",
		"ro":         "read-only",
		"read-only":  "read-only",
		"no-access":  "no-access",
	}

	for input, expected := range cases {
		value, diags := normalizeAccess(stringValueOrNull(input))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics for %q: %v", input, diags)
		}
		if value != expected {
			t.Fatalf("expected %q, got %q", expected, value)
		}
	}
}

func TestMappingStatePortsNullWhenUnconfigured(t *testing.T) {
	ctx := context.Background()
	model := volumeMappingResourceModel{
		Ports: types.SetNull(types.StringType),
	}
	mapping := &msa.Mapping{
		Volume: "vol1",
		Access: "read-write",
		LUN:    "1",
		Ports:  "1,2,3",
	}

	state, diags := mappingStateFromModel(ctx, model, mapping)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.Ports.IsNull() {
		t.Fatalf("expected ports to be null when not configured")
	}
}

func TestMappingStatePortsFromAPIWhenConfigured(t *testing.T) {
	ctx := context.Background()
	setValue, diag := types.SetValueFrom(ctx, types.StringType, []string{"1", "2"})
	if diag.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diag)
	}
	model := volumeMappingResourceModel{
		Ports: setValue,
	}
	mapping := &msa.Mapping{
		Volume: "vol1",
		Access: "read-write",
		LUN:    "1",
		Ports:  "1,2,3",
	}

	state, diags := mappingStateFromModel(ctx, model, mapping)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Ports.IsNull() {
		t.Fatalf("expected ports to be set when configured")
	}
	var ports []string
	diags = state.Ports.ElementsAs(ctx, &ports, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading ports: %v", diags)
	}
	if len(ports) != 3 {
		t.Fatalf("expected 3 ports from API, got %d", len(ports))
	}
}

func TestCanonicalAccess(t *testing.T) {
	cases := map[string]string{
		"rw":         "read-write",
		"Read-Write": "read-write",
		"ro":         "read-only",
		"read-only":  "read-only",
		"no-access":  "no-access",
	}

	for input, expected := range cases {
		if value := canonicalAccess(input); value != expected {
			t.Fatalf("expected %q, got %q", expected, value)
		}
	}
}
