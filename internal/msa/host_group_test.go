package msa

import "testing"

func TestHostGroupsFromResponse(t *testing.T) {
	fixture := readFixture(t, "show_host_groups.xml")
	response, err := parseResponse(fixture)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}

	groups := HostGroupsFromResponse(response)
	if len(groups) != 1 {
		t.Fatalf("expected 1 host group, got %d", len(groups))
	}

	group := groups[0]
	if group.Name != "UNGROUPED" {
		t.Fatalf("expected UNGROUPED, got %q", group.Name)
	}
	if group.DurableID != "HG0" {
		t.Fatalf("expected durable id HG0, got %q", group.DurableID)
	}
	if group.SerialNumber != "UNGROUPEDHOSTS" {
		t.Fatalf("expected serial number UNGROUPEDHOSTS, got %q", group.SerialNumber)
	}
	if group.MemberCount != 2 {
		t.Fatalf("expected member count 2, got %d", group.MemberCount)
	}
	if len(group.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(group.Hosts))
	}
	if group.Hosts[0].Name != "HostA" || group.Hosts[1].Name != "HostB" {
		t.Fatalf("unexpected hosts: %v", group.Hosts)
	}
}
