package provider

import (
	"strconv"
	"testing"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestResolveVolumeTarget(t *testing.T) {
	testCases := []struct {
		name     string
		pool     string
		vdisk    string
		expected string
		wantErr  error
	}{
		{name: "pool", pool: "pool-a", expected: "pool-a"},
		{name: "vdisk", vdisk: "A", expected: "A"},
		{name: "both", pool: "pool-a", vdisk: "A", wantErr: errVolumeTargetConflict},
		{name: "none", wantErr: errVolumeTargetMissing},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			model := volumeResourceModel{}
			model.Pool = stringValueOrNull(tc.pool)
			model.VDisk = stringValueOrNull(tc.vdisk)

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
		{name: "invalid", input: "abc", wantErr: true},
		{name: "missing-unit", input: "100", wantErr: true},
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
