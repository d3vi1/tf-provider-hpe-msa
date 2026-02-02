package msa

import "testing"

func TestVolumesFromResponse(t *testing.T) {
	fixture := readFixture(t, "show_volumes.xml")
	response, err := parseResponse(fixture)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	volumes := VolumesFromResponse(response)
	if len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(volumes))
	}

	volume := volumes[0]
	if volume.Name != "vol01" {
		t.Fatalf("unexpected name: %s", volume.Name)
	}
	if volume.SerialNumber != "SN123" {
		t.Fatalf("unexpected serial number: %s", volume.SerialNumber)
	}
	if volume.DurableID != "V1" {
		t.Fatalf("unexpected durable id: %s", volume.DurableID)
	}
	if volume.WWN != "600c0ff0000000000000000000000001" {
		t.Fatalf("unexpected wwn: %s", volume.WWN)
	}
	if volume.PoolName != "pool-a" {
		t.Fatalf("unexpected pool name: %s", volume.PoolName)
	}
	if volume.VDiskName != "pool-a" {
		t.Fatalf("unexpected vdisk name: %s", volume.VDiskName)
	}
}
