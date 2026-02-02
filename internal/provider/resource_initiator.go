package provider

import (
	"context"
	"errors"
	"strings"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*initiatorResource)(nil)
var _ resource.ResourceWithImportState = (*initiatorResource)(nil)

func NewInitiatorResource() resource.Resource {
	return &initiatorResource{}
}

type initiatorResource struct {
	client *msa.Client
}

type initiatorResourceModel struct {
	ID           types.String `tfsdk:"id"`
	InitiatorID  types.String `tfsdk:"initiator_id"`
	Nickname     types.String `tfsdk:"nickname"`
	Profile      types.String `tfsdk:"profile"`
	HostID       types.String `tfsdk:"host_id"`
	HostKey      types.String `tfsdk:"host_key"`
	Properties   types.Map    `tfsdk:"properties"`
	AllowDestroy types.Bool   `tfsdk:"allow_destroy"`
}

func (r *initiatorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_msa_initiator"
}

func (r *initiatorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Initiator identifier.",
				Computed:    true,
			},
			"initiator_id": schema.StringAttribute{
				Description: "Initiator ID (WWPN or IQN).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					initiatorIDValidator{},
				},
			},
			"nickname": schema.StringAttribute{
				Description: "Initiator nickname.",
				Required:    true,
			},
			"profile": schema.StringAttribute{
				Description: "Initiator profile (standard, hp-ux, openvms).",
				Optional:    true,
				Computed:    true,
			},
			"host_id": schema.StringAttribute{
				Description: "Host serial number associated with this initiator.",
				Computed:    true,
			},
			"host_key": schema.StringAttribute{
				Description: "Host key associated with this initiator.",
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Raw properties returned by the XML API.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"allow_destroy": schema.BoolAttribute{
				Description: "Require explicit opt-in to delete initiator nicknames.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *initiatorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*msa.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", "Expected *msa.Client")
		return
	}

	r.client = client
}

func (r *initiatorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan initiatorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	initID := strings.TrimSpace(plan.InitiatorID.ValueString())
	nickname := strings.TrimSpace(plan.Nickname.ValueString())
	if initID == "" || nickname == "" {
		resp.Diagnostics.AddError("Invalid configuration", "initiator_id and nickname are required")
		return
	}

	if err := r.setInitiator(ctx, initID, nickname, plan.Profile); err != nil {
		resp.Diagnostics.AddError("Unable to set initiator", err.Error())
		return
	}

	initiator, err := r.findInitiator(ctx, initID, nickname)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read initiator after create", err.Error())
		return
	}

	state, diag := initiatorStateFromModel(ctx, plan, initiator, true)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *initiatorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state initiatorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	initID := initiatorLookupID(state)
	nickname := strings.TrimSpace(state.Nickname.ValueString())
	if initID == "" && nickname == "" {
		resp.Diagnostics.AddError("Invalid state", "initiator_id is required")
		return
	}

	initiator, err := r.findInitiator(ctx, initID, nickname)
	if err != nil {
		if errors.Is(err, errInitiatorNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read initiator", err.Error())
		return
	}

	newState, diag := initiatorStateFromModel(ctx, state, initiator, false)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *initiatorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan initiatorResourceModel
	var state initiatorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	initID := strings.TrimSpace(plan.InitiatorID.ValueString())
	nickname := strings.TrimSpace(plan.Nickname.ValueString())
	if initID == "" || nickname == "" {
		resp.Diagnostics.AddError("Invalid configuration", "initiator_id and nickname are required")
		return
	}

	if err := r.setInitiator(ctx, initID, nickname, plan.Profile); err != nil {
		resp.Diagnostics.AddError("Unable to update initiator", err.Error())
		return
	}

	initiator, err := r.findInitiator(ctx, initID, nickname)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read initiator after update", err.Error())
		return
	}

	newState, diag := initiatorStateFromModel(ctx, plan, initiator, true)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *initiatorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state initiatorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	if state.AllowDestroy.IsNull() || !state.AllowDestroy.ValueBool() {
		resp.Diagnostics.AddError(
			"Initiator deletion not permitted",
			"Set allow_destroy = true to permit initiator nickname deletion.",
		)
		return
	}

	initID := ""
	if !state.ID.IsNull() && !state.ID.IsUnknown() {
		initID = strings.TrimSpace(state.ID.ValueString())
	}
	if initID == "" {
		initID = strings.TrimSpace(state.InitiatorID.ValueString())
	}
	if initID == "" {
		resp.Diagnostics.AddError("Invalid state", "initiator_id is required for deletion")
		return
	}

	_, err := r.client.Execute(ctx, "delete", "initiator-nickname", initID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete initiator nickname", err.Error())
		return
	}
}

func (r *initiatorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("initiator_id"), req.ID)...)
}

var errInitiatorNotFound = errors.New("initiator not found")

func (r *initiatorResource) findInitiator(ctx context.Context, id, nickname string) (*msa.Initiator, error) {
	response, err := r.client.Execute(ctx, "show", "initiators")
	if err != nil {
		return nil, err
	}

	initiators := msa.InitiatorsFromResponse(response)
	for _, initiator := range initiators {
		if id != "" && strings.EqualFold(initiator.ID, id) {
			return &initiator, nil
		}
	}
	for _, initiator := range initiators {
		if nickname != "" && strings.EqualFold(initiator.Nickname, nickname) {
			return &initiator, nil
		}
	}
	return nil, errInitiatorNotFound
}

func (r *initiatorResource) setInitiator(ctx context.Context, id, nickname string, profile types.String) error {
	parts := []string{"set", "initiator", "id", id, "nickname", nickname}
	if !profile.IsNull() && !profile.IsUnknown() && strings.TrimSpace(profile.ValueString()) != "" {
		parts = append(parts, "profile", profile.ValueString())
	}

	_, err := r.client.Execute(ctx, parts...)
	return err
}

func initiatorStateFromModel(ctx context.Context, model initiatorResourceModel, initiator *msa.Initiator, preservePlan bool) (initiatorResourceModel, diag.Diagnostics) {
	state := model
	var diags diag.Diagnostics

	if !model.InitiatorID.IsNull() && !model.InitiatorID.IsUnknown() && strings.TrimSpace(model.InitiatorID.ValueString()) != "" {
		state.InitiatorID = types.StringValue(strings.TrimSpace(model.InitiatorID.ValueString()))
	} else if initiator.ID != "" {
		state.InitiatorID = types.StringValue(initiator.ID)
	}
	if initiator.ID != "" {
		state.ID = types.StringValue(initiator.ID)
	}
	if preservePlan && !model.Nickname.IsNull() && !model.Nickname.IsUnknown() && strings.TrimSpace(model.Nickname.ValueString()) != "" {
		state.Nickname = types.StringValue(strings.TrimSpace(model.Nickname.ValueString()))
	} else if initiator.Nickname != "" {
		state.Nickname = types.StringValue(initiator.Nickname)
	}
	if preservePlan && !model.Profile.IsNull() && !model.Profile.IsUnknown() && strings.TrimSpace(model.Profile.ValueString()) != "" {
		state.Profile = types.StringValue(strings.TrimSpace(model.Profile.ValueString()))
	} else if initiator.Profile != "" {
		apiProfile := strings.TrimSpace(initiator.Profile)
		if !model.Profile.IsNull() && !model.Profile.IsUnknown() && strings.TrimSpace(model.Profile.ValueString()) != "" &&
			strings.EqualFold(model.Profile.ValueString(), apiProfile) {
			state.Profile = types.StringValue(strings.TrimSpace(model.Profile.ValueString()))
		} else {
			state.Profile = types.StringValue(strings.ToLower(apiProfile))
		}
	}
	if initiator.HostID != "" {
		state.HostID = types.StringValue(initiator.HostID)
	}
	if initiator.HostKey != "" {
		state.HostKey = types.StringValue(initiator.HostKey)
	}

	propsValue, diag := types.MapValueFrom(ctx, types.StringType, initiator.Properties)
	if diag.HasError() {
		diags.Append(diag...)
		return state, diags
	}
	state.Properties = propsValue

	return state, diags
}

func initiatorLookupID(state initiatorResourceModel) string {
	if !state.ID.IsNull() && !state.ID.IsUnknown() {
		if value := strings.TrimSpace(state.ID.ValueString()); value != "" {
			return value
		}
	}
	return strings.TrimSpace(state.InitiatorID.ValueString())
}
