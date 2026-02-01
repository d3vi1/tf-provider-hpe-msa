package msa

import "testing"

func TestSnapshotsFromResponse(t *testing.T) {
	fixture := readFixture(t, "show_snapshots.xml")
	response, err := parseResponse(fixture)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}

	snapshots := SnapshotsFromResponse(response)
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}

	snapshot := snapshots[0]
	if snapshot.Name != "snap-test" {
		t.Fatalf("unexpected name: %s", snapshot.Name)
	}
	if snapshot.SerialNumber != "00deadbeef0000000011223344556677" {
		t.Fatalf("unexpected serial number: %s", snapshot.SerialNumber)
	}
	if snapshot.DurableID != "V10" {
		t.Fatalf("unexpected durable id: %s", snapshot.DurableID)
	}
	if snapshot.BaseVolumeName != "vol-test" {
		t.Fatalf("unexpected base volume: %s", snapshot.BaseVolumeName)
	}
	if snapshot.PoolName != "A" {
		t.Fatalf("unexpected pool name: %s", snapshot.PoolName)
	}
	if snapshot.VDiskName != "A" {
		t.Fatalf("unexpected vdisk name: %s", snapshot.VDiskName)
	}
	if snapshot.Size != "1.0GB" {
		t.Fatalf("unexpected size: %s", snapshot.Size)
	}
	if snapshot.SizeNumeric != "1953125" {
		t.Fatalf("unexpected size numeric: %s", snapshot.SizeNumeric)
	}
}
