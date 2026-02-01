package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestResolveVolumeTarget(t *testing.T) {
	testCases := []struct {
		name     string
		pool     string
		vdisk    string
		expected string
		wantErr  bool
	}{
		{name: "pool", pool: "pool-a", expected: "pool-a"},
		{name: "vdisk", vdisk: "A", expected: "A"},
		{name: "both", pool: "pool-a", vdisk: "A", wantErr: true},
		{name: "none", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := volumeResourceModel{}
			model.Pool = stringValueOrNull(tc.pool)
			model.VDisk = stringValueOrNull(tc.vdisk)

			value, err := resolveVolumeTarget(model)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, value)
			}
		})
	}
}

func stringValueOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}
