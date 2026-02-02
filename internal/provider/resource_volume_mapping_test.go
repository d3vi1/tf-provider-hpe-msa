package provider

import "testing"

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
