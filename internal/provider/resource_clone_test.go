package provider

import (
	"testing"

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
