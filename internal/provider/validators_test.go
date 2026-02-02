package provider

import "testing"

func TestIsValidInitiatorID(t *testing.T) {
	valid := []string{
		"50:06:01:60:3b:ad:be:ef",
		"500601603badbeef",
		"50-06-01-60-3b-ad-be-ef",
		"iqn.1993-08.org.debian:01:aaa",
		"eui.02004567A425678D",
		"naa.50060160A3B3BEEF",
	}
	for _, value := range valid {
		if !isValidInitiatorID(value) {
			t.Fatalf("expected valid initiator_id for %q", value)
		}
	}

	invalid := []string{
		"",
		"NOTAWWPN",
		"50:06:01:60:3b:ad:be",
		"50:06:01:60:3b:ad:be:eg",
		"iqn.",
		"eui.",
		"iqn.1993-08.org.debian:01: a",
	}
	for _, value := range invalid {
		if isValidInitiatorID(value) {
			t.Fatalf("expected invalid initiator_id for %q", value)
		}
	}
}
