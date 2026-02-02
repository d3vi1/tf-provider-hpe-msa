package provider

import (
	"context"
	"strings"
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
		"IQN.1993-08.org.example:foo",
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

func TestHostNameValidatorLength(t *testing.T) {
	v := hostNameValidator{}

	valid := strings.Repeat("h", maxHostNameLength)
	req := validator.StringRequest{ConfigValue: types.StringValue(valid)}
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics for valid host_name: %v", resp.Diagnostics)
	}

	invalid := strings.Repeat("h", maxHostNameLength+1)
	req = validator.StringRequest{ConfigValue: types.StringValue(invalid)}
	resp = &validator.StringResponse{}
	v.ValidateString(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected diagnostics for oversized host_name")
	}
}

func TestHostGroupNameValidator(t *testing.T) {
	v := hostGroupNameValidator{}

	valid := []string{
		"GroupA",
		"Group 1",
		strings.Repeat("g", maxHostGroupNameBytes),
	}
	for _, value := range valid {
		req := validator.StringRequest{ConfigValue: types.StringValue(value)}
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), req, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics for valid host group name %q: %v", value, resp.Diagnostics)
		}
	}

	invalid := []string{
		"",
		"bad,name",
		"bad.name",
		"bad<name",
		`bad\\name`,
		strings.Repeat("g", maxHostGroupNameBytes+1),
	}
	for _, value := range invalid {
		req := validator.StringRequest{ConfigValue: types.StringValue(value)}
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), req, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("expected diagnostics for invalid host group name %q", value)
		}
	}
}

func TestHostNamesSetValidator(t *testing.T) {
	v := hostNamesSetValidator{}

	valid := []string{"HostA", "HostB"}
	setValue, diag := types.SetValueFrom(context.Background(), types.StringType, valid)
	if diag.HasError() {
		t.Fatalf("unexpected diagnostics building set: %v", diag)
	}
	req := validator.SetRequest{ConfigValue: setValue}
	resp := &validator.SetResponse{}
	v.ValidateSet(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics for valid hosts: %v", resp.Diagnostics)
	}

	invalid := []string{"HostA", "   "}
	setValue, diag = types.SetValueFrom(context.Background(), types.StringType, invalid)
	if diag.HasError() {
		t.Fatalf("unexpected diagnostics building set: %v", diag)
	}
	req = validator.SetRequest{ConfigValue: setValue}
	resp = &validator.SetResponse{}
	v.ValidateSet(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected diagnostics for invalid hosts")
	}
}
