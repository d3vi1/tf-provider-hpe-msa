package provider

import (
	"strconv"
	"testing"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestResolveVolumeTarget(t *testing.T) {
	testCases := []struct {
		name         string
		pool         string
		vdisk        string
		poolUnknown  bool
		vdiskUnknown bool
		expected     string
		wantErr      error
	}{
		{name: "pool", pool: "pool-a", expected: "pool-a"},
		{name: "vdisk", vdisk: "A", expected: "A"},
		{name: "both", pool: "pool-a", vdisk: "A", wantErr: errVolumeTargetConflict},
		{name: "none", wantErr: errVolumeTargetMissing},
		{name: "pool-unknown", poolUnknown: true, wantErr: errVolumeTargetUnknown},
		{name: "vdisk-unknown", vdiskUnknown: true, wantErr: errVolumeTargetUnknown},
		{name: "vdisk-known-pool-unknown", vdisk: "A", poolUnknown: true, expected: "A"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := volumeResourceModel{}
			if tc.poolUnknown {
				model.Pool = types.StringUnknown()
			} else {
				model.Pool = stringValueOrNull(tc.pool)
			}
			if tc.vdiskUnknown {
				model.VDisk = types.StringUnknown()
			} else {
				model.VDisk = stringValueOrNull(tc.vdisk)
			}

			value, err := resolveVolumeTarget(model)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error")
				}
				if err != tc.wantErr {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
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

func TestParseSizeToBytes(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{name: "gb", input: "1GB", want: 1_000_000_000},
		{name: "gib", input: "1GiB", want: 1_073_741_824},
		{name: "float", input: "2.5TB", want: 2_500_000_000_000},
		{name: "lowercase", input: "1gib", want: 1_073_741_824},
		{name: "space", input: "1 GB", want: 1_000_000_000},
		{name: "trim", input: " 1GB ", want: 1_000_000_000},
		{name: "invalid", input: "abc", wantErr: true},
		{name: "missing-unit", input: "100", wantErr: true},
		{name: "negative", input: "-1GB", wantErr: true},
		{name: "zero", input: "0GB", wantErr: true},
		{name: "malformed", input: "1..2GB", wantErr: true},
		{name: "invalid-unit", input: "1GBB", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			value, err := parseSizeToBytes(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, value)
			}
		})
	}
}

func TestParseSizeToBytesStressInputs(t *testing.T) {
	inputs := map[string]bool{
		"1GB":       false,
		"1GiB":      false,
		"1gib":      false,
		"1 G":       false,
		"1 gB":      false,
		"1G B":      true,
		"1.000GB":   false,
		"1.5GiB":    false,
		"9999999TB": true,
		"":          true,
		" ":         true,
		"1":         true,
		"1e3GB":     true,
		"1_000GB":   true,
		"GB":        true,
		"1.2.3GB":   true,
		"1GB ":      false,
		" 1GB":      false,
		"\t2TB":     false,
	}

	for input, wantErr := range inputs {
		value, err := parseSizeToBytes(input)
		if wantErr {
			if err == nil {
				t.Fatalf("expected error for %q, got %d", input, value)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", input, err)
		}
		if value <= 0 {
			t.Fatalf("expected positive value for %q, got %d", input, value)
		}
	}
}

func TestVolumeSizeMatches(t *testing.T) {
	planSize := "2GB"
	planBytes := int64(2_000_000_000)

	withinToleranceBytes := planBytes - 4*1024*1024
	volume := &msa.Volume{SizeNumeric: strconv.FormatInt(withinToleranceBytes/512, 10)}
	match, err := volumeSizeMatches(planSize, volume)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !match {
		t.Fatalf("expected match within tolerance")
	}

	outsideToleranceBytes := planBytes - 20*1024*1024
	volume = &msa.Volume{SizeNumeric: strconv.FormatInt(outsideToleranceBytes/512, 10)}
	match, err = volumeSizeMatches(planSize, volume)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match {
		t.Fatalf("expected mismatch outside tolerance")
	}
}

func TestPoolNamesFromResponse(t *testing.T) {
	response := msa.Response{
		Objects: []msa.Object{
			{
				BaseType: "pools",
				Name:     "poolA",
				Properties: []msa.Property{
					{Name: "pool-name", Value: "poolA"},
				},
			},
			{
				BaseType: "pools",
				Name:     "pools",
			},
			{
				BaseType: "pools",
				Name:     "poolB",
				Properties: []msa.Property{
					{Name: "name", Value: "poolB"},
				},
			},
			{
				BaseType: "status",
				Name:     "status",
				Properties: []msa.Property{
					{Name: "response-type", Value: "Success"},
				},
			},
			{
				BaseType: "tiers",
				Name:     "tier1",
				Properties: []msa.Property{
					{Name: "name", Value: "tier1"},
				},
			},
		},
	}

	names := poolNamesFromResponse(response)
	if len(names) != 2 {
		t.Fatalf("expected 2 pools, got %d", len(names))
	}
	if names[0] != "poolA" || names[1] != "poolB" {
		t.Fatalf("unexpected pool names: %v", names)
	}
}

func TestVolumeStateFromModelSCSIWWN(t *testing.T) {
	model := volumeResourceModel{}
	volume := &msa.Volume{
		Name:         "vol01",
		SerialNumber: "SN123",
		WWN:          "600c0ff0000000000000000000000001",
	}

	state := volumeStateFromModel(model, volume)
	if state.SCSIWWN.IsNull() || state.SCSIWWN.ValueString() != volume.WWN {
		t.Fatalf("expected scsi_wwn to be set from volume wwn")
	}

	volume.WWN = ""
	state = volumeStateFromModel(model, volume)
	if !state.SCSIWWN.IsNull() {
		t.Fatalf("expected scsi_wwn to be null when wwn missing")
	}
}
