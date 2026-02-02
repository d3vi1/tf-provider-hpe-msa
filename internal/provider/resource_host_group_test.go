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

func TestDiffHostGroupMembersWhitespaceAndCase(t *testing.T) {
	add, remove := diffHostGroupMembers(
		[]string{" hosta ", "HostC"},
		[]string{"HostA", "HostB"},
	)
	if len(add) != 1 || add[0] != "HostC" {
		t.Fatalf("unexpected add list: %v", add)
	}
	if len(remove) != 1 || remove[0] != "HostB" {
		t.Fatalf("unexpected remove list: %v", remove)
	}
}

func TestUniqueHostNames(t *testing.T) {
	unique := uniqueHostNames([]string{" HostA ", "hosta", "HostB", ""})
	if len(unique) != 2 {
		t.Fatalf("unexpected unique list: %v", unique)
	}
	if unique[0] != "HostA" || unique[1] != "HostB" {
		t.Fatalf("unexpected unique list order: %v", unique)
	}
}
