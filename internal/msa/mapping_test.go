package msa

import "testing"

func TestMappingsFromResponse(t *testing.T) {
	fixture := readFixture(t, "show_maps_initiator.xml")
	response, err := parseResponse(fixture)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}

	mappings := MappingsFromResponse(response)
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(mappings))
	}

	mapping := mappings[0]
	if mapping.Volume != "volA" {
		t.Fatalf("unexpected volume %q", mapping.Volume)
	}
	if mapping.VolumeSerial != "00c0ff3cab9c00000000000002010000" {
		t.Fatalf("unexpected serial %q", mapping.VolumeSerial)
	}
	if mapping.LUN != "12" {
		t.Fatalf("unexpected LUN %q", mapping.LUN)
	}
	if mapping.Access != "read-write" {
		t.Fatalf("unexpected access %q", mapping.Access)
	}
	if mapping.Ports != "1,2" {
		t.Fatalf("unexpected ports %q", mapping.Ports)
	}
}
