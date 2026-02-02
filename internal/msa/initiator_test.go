package msa

import "testing"

func TestInitiatorsFromResponse(t *testing.T) {
	fixture := readFixture(t, "show_initiators.xml")
	response, err := parseResponse(fixture)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}

	initiators := InitiatorsFromResponse(response)
	if len(initiators) != 2 {
		t.Fatalf("expected 2 initiators, got %d", len(initiators))
	}

	if initiators[0].ID != "20000000000000c1" {
		t.Fatalf("unexpected initiator id %q", initiators[0].ID)
	}
	if initiators[0].Nickname != "InitA" {
		t.Fatalf("unexpected nickname %q", initiators[0].Nickname)
	}
	if initiators[0].HostKey != "H1" {
		t.Fatalf("unexpected host key %q", initiators[0].HostKey)
	}
	if initiators[0].Profile != "Standard" {
		t.Fatalf("unexpected profile %q", initiators[0].Profile)
	}
}
