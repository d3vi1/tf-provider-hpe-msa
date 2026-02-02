package msa

import "testing"

func TestMappingsFromResponse(t *testing.T) {
	fixture := readFixture(t, "show_maps_initiator.xml")
	response, err := parseResponse(fixture)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}

	mappings := MappingsFromResponse(response)
	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}

	if mappings[0].Volume != "volA" {
		t.Fatalf("unexpected volume %q", mappings[0].Volume)
	}
	if mappings[0].VolumeSerial != "00c0ff3cab9c00000000000002010000" {
		t.Fatalf("unexpected serial %q", mappings[0].VolumeSerial)
	}
	if mappings[0].LUN != "12" {
		t.Fatalf("unexpected LUN %q", mappings[0].LUN)
	}
	if mappings[0].Access != "read-write" {
		t.Fatalf("unexpected access %q", mappings[0].Access)
	}
	if mappings[0].Ports != "1,2" {
		t.Fatalf("unexpected ports %q", mappings[0].Ports)
	}

	if mappings[1].Volume != "volB" {
		t.Fatalf("unexpected volume %q", mappings[1].Volume)
	}
	if mappings[1].Access != "no-access" {
		t.Fatalf("unexpected access %q", mappings[1].Access)
	}
	if mappings[1].LUN != "" {
		t.Fatalf("expected empty LUN for no-access, got %q", mappings[1].LUN)
	}
}
