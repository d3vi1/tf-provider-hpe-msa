package provider

import "testing"

func TestDiffHostGroupMembers(t *testing.T) {
	add, remove := diffHostGroupMembers(
		[]string{"HostA", "HostC"},
		[]string{"HostA", "HostB"},
	)

	if len(add) != 1 || add[0] != "HostC" {
		t.Fatalf("unexpected add list: %v", add)
	}
	if len(remove) != 1 || remove[0] != "HostB" {
		t.Fatalf("unexpected remove list: %v", remove)
	}
}

func TestDiffHostGroupMembersCaseInsensitive(t *testing.T) {
	add, remove := diffHostGroupMembers(
		[]string{"hosta", "HostB"},
		[]string{"HostA"},
	)
	if len(add) != 1 || add[0] != "HostB" {
		t.Fatalf("unexpected add list: %v", add)
	}
	if len(remove) != 0 {
		t.Fatalf("unexpected remove list: %v", remove)
	}
}

func TestDiffHostGroupMembersDedupes(t *testing.T) {
	add, remove := diffHostGroupMembers(
		[]string{"HostA", "HostA", "HostB"},
		[]string{"HostB"},
	)
	if len(add) != 1 || add[0] != "HostA" {
		t.Fatalf("unexpected add list: %v", add)
	}
	if len(remove) != 0 {
		t.Fatalf("unexpected remove list: %v", remove)
	}
}
