package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestIsValidInitiatorID(t *testing.T) {
	valid := []string{
		"50:06:01:60:3b:ad:be:ef",
		"500601603badbeef",
		"50-06-01-60-3b-ad-be-ef",
		"iqn.1993-08.org.debian:01:aaa",
		"eui.02004567A425678D",
		"naa.50060160A3B3BEEF",
		"naa.50060160A3B3BEEF50060160A3B3BEEF",
	}
	for _, value := range valid {
		if !isValidInitiatorID(value) {
			t.Fatalf("expected valid initiator_id for %q", value)
		}
	}

	invalid := []string{
		"",
		"   ",
		"NOTAWWPN",
		"50:06:01:60:3b:ad:be",
		"50:06:01:60:3b:ad:be:eg",
		"iqn.",
		"iqn.x",
		"iqn.1993-08:missingdomain",
		"iqn.1993-08.org.debian:",
		"eui.",
		"eui.zz",
		"iqn.1993-08.org.debian:01: a",
		"naa.foo",
	}
	for _, value := range invalid {
		if isValidInitiatorID(value) {
			t.Fatalf("expected invalid initiator_id for %q", value)
		}
	}
}

func TestInitiatorIDValidatorRejectsEmpty(t *testing.T) {
	v := initiatorIDValidator{}
	req := validator.StringRequest{
		ConfigValue: types.StringValue(" "),
	}
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected diagnostics for empty initiator_id")
	}
}
