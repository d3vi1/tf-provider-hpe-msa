package provider

import (
	"context"
	"testing"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestInitiatorStateFromModelPreservePlan(t *testing.T) {
	ctx := context.Background()
	model := initiatorResourceModel{
		InitiatorID: types.StringValue("50:aa:bb:cc:dd:ee:ff:00"),
		Nickname:    types.StringValue("init1"),
		Profile:     types.StringValue("standard"),
	}
	initiator := &msa.Initiator{
		ID:       "50aabbccddeeff00",
		Nickname: "INIT1",
		Profile:  "Standard",
	}

	state, diags := initiatorStateFromModel(ctx, model, initiator, true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.InitiatorID.ValueString() != "50:aa:bb:cc:dd:ee:ff:00" {
		t.Fatalf("unexpected initiator_id: %s", state.InitiatorID.ValueString())
	}
	if state.Nickname.ValueString() != "init1" {
		t.Fatalf("unexpected nickname: %s", state.Nickname.ValueString())
	}
	if state.Profile.ValueString() != "standard" {
		t.Fatalf("unexpected profile: %s", state.Profile.ValueString())
	}
	if state.ID.ValueString() != "50aabbccddeeff00" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
}

func TestInitiatorStateFromModelReadUsesAPI(t *testing.T) {
	ctx := context.Background()
	model := initiatorResourceModel{
		InitiatorID: types.StringValue("50:aa:bb:cc:dd:ee:ff:00"),
		Nickname:    types.StringValue("init1"),
		Profile:     types.StringValue("standard"),
	}
	initiator := &msa.Initiator{
		ID:       "50aabbccddeeff00",
		Nickname: "INIT1",
		Profile:  "Standard",
	}

	state, diags := initiatorStateFromModel(ctx, model, initiator, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.InitiatorID.ValueString() != "50:aa:bb:cc:dd:ee:ff:00" {
		t.Fatalf("unexpected initiator_id: %s", state.InitiatorID.ValueString())
	}
	if state.Nickname.ValueString() != "INIT1" {
		t.Fatalf("unexpected nickname: %s", state.Nickname.ValueString())
	}
	if state.Profile.ValueString() != "standard" {
		t.Fatalf("unexpected profile: %s", state.Profile.ValueString())
	}
	if state.ID.ValueString() != "50aabbccddeeff00" {
		t.Fatalf("unexpected id: %s", state.ID.ValueString())
	}
}

func TestInitiatorStateFromModelReadPreservesProfileCaseWhenEqual(t *testing.T) {
	ctx := context.Background()
	model := initiatorResourceModel{
		InitiatorID: types.StringValue("50:aa:bb:cc:dd:ee:ff:00"),
		Nickname:    types.StringValue("init1"),
		Profile:     types.StringValue("Standard"),
	}
	initiator := &msa.Initiator{
		ID:       "50aabbccddeeff00",
		Nickname: "init1",
		Profile:  "Standard",
	}

	state, diags := initiatorStateFromModel(ctx, model, initiator, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Profile.ValueString() != "Standard" {
		t.Fatalf("unexpected profile: %s", state.Profile.ValueString())
	}
}

func TestInitiatorStateFromModelReadUpdatesProfileOnDrift(t *testing.T) {
	ctx := context.Background()
	model := initiatorResourceModel{
		InitiatorID: types.StringValue("50:aa:bb:cc:dd:ee:ff:00"),
		Nickname:    types.StringValue("init1"),
		Profile:     types.StringValue("standard"),
	}
	initiator := &msa.Initiator{
		ID:       "50aabbccddeeff00",
		Nickname: "init1",
		Profile:  "hp-ux",
	}

	state, diags := initiatorStateFromModel(ctx, model, initiator, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Profile.ValueString() != "hp-ux" {
		t.Fatalf("unexpected profile: %s", state.Profile.ValueString())
	}
}

func TestInitiatorLookupIDPrefersCanonicalID(t *testing.T) {
	state := initiatorResourceModel{
		ID:          types.StringValue("50aabbccddeeff00"),
		InitiatorID: types.StringValue("50:aa:bb:cc:dd:ee:ff:00"),
	}
	if got := initiatorLookupID(state); got != "50aabbccddeeff00" {
		t.Fatalf("unexpected lookup id: %s", got)
	}

	state.ID = types.StringNull()
	if got := initiatorLookupID(state); got != "50:aa:bb:cc:dd:ee:ff:00" {
		t.Fatalf("unexpected fallback id: %s", got)
	}
}
