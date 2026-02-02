package msa

import "testing"

func TestHostsFromResponse(t *testing.T) {
	fixture := readFixture(t, "show_host_groups.xml")
	response, err := parseResponse(fixture)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}

	hosts := HostsFromResponse(response)
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	if hosts[0].Name != "HostA" {
		t.Fatalf("expected HostA, got %q", hosts[0].Name)
	}
	if hosts[0].DurableID != "H1" {
		t.Fatalf("expected durable id H1, got %q", hosts[0].DurableID)
	}
	if hosts[0].SerialNumber != "00c0ff3cab9c00000000000001010000" {
		t.Fatalf("unexpected serial number %q", hosts[0].SerialNumber)
	}
	if hosts[0].MemberCount != 2 {
		t.Fatalf("expected member count 2, got %d", hosts[0].MemberCount)
	}
	if hosts[0].HostGroup != "UNGROUPEDHOSTS" {
		t.Fatalf("expected host group UNGROUPEDHOSTS, got %q", hosts[0].HostGroup)
	}
}
