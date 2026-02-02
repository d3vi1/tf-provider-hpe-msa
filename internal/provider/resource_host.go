package provider

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*hostResource)(nil)
var _ resource.ResourceWithImportState = (*hostResource)(nil)

func NewHostResource() resource.Resource {
	return &hostResource{}
}

type hostResource struct {
	client *msa.Client
}

type hostResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Initiators   types.Set    `tfsdk:"initiators"`
	HostGroup    types.String `tfsdk:"host_group"`
	Profile      types.String `tfsdk:"profile"`
	DurableID    types.String `tfsdk:"durable_id"`
	SerialNumber types.String `tfsdk:"serial_number"`
	GroupKey     types.String `tfsdk:"group_key"`
	MemberCount  types.Int64  `tfsdk:"member_count"`
	Properties   types.Map    `tfsdk:"properties"`
	AllowDestroy types.Bool   `tfsdk:"allow_destroy"`
}

func (r *hostResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_msa_host"
}

func (r *hostResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Host identifier (serial number).",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Host name.",
				Required:    true,
			},
			"initiators": schema.SetAttribute{
				Description: "Initiator IDs or nicknames to seed the host (comma-free values).",
				Required:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
			"host_group": schema.StringAttribute{
				Description: "Optional host group name to add the host to.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"profile": schema.StringAttribute{
				Description: "Host profile (standard, hp-ux, openvms).",
				Optional:    true,
				Computed:    true,
			},
			"durable_id": schema.StringAttribute{
				Description: "Durable ID reported by the array.",
				Computed:    true,
			},
			"serial_number": schema.StringAttribute{
				Description: "Host serial number reported by the array.",
				Computed:    true,
			},
			"group_key": schema.StringAttribute{
				Description: "Host group key reported by the array.",
				Computed:    true,
			},
			"member_count": schema.Int64Attribute{
				Description: "Number of initiators in the host.",
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Raw properties returned by the XML API.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"allow_destroy": schema.BoolAttribute{
				Description: "Require explicit opt-in to delete hosts.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *hostResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *hostResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	name := strings.TrimSpace(plan.Name.ValueString())
	if name == "" {
		resp.Diagnostics.AddError("Invalid name", "name must be provided")
		return
	}

	initiators, diag := setToStrings(ctx, plan.Initiators)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(initiators) == 0 {
		resp.Diagnostics.AddError("Invalid initiators", "at least one initiator is required to create a host")
		return
	}

	parts := []string{"create", "host"}
	if !plan.HostGroup.IsNull() && !plan.HostGroup.IsUnknown() && plan.HostGroup.ValueString() != "" {
		parts = append(parts, "host-group", plan.HostGroup.ValueString())
	}
	parts = append(parts, "initiators", strings.Join(initiators, ","))
	if !plan.Profile.IsNull() && !plan.Profile.IsUnknown() && plan.Profile.ValueString() != "" {
		parts = append(parts, "profile", plan.Profile.ValueString())
	}
	parts = append(parts, name)

	_, err := r.client.Execute(ctx, parts...)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create host", err.Error())
		return
	}

	host, err := r.waitForHost(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read host after create", err.Error())
		return
	}

	state, diag := hostStateFromModel(ctx, plan, host)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *hostResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	name := strings.TrimSpace(state.Name.ValueString())
	if name == "" {
		resp.Diagnostics.AddError("Invalid state", "name is required")
		return
	}

	host, err := r.findHost(ctx, name)
	if err != nil {
		if errors.Is(err, errHostNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read host", err.Error())
		return
	}

	newState, diag := hostStateFromModel(ctx, state, host)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *hostResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan hostResourceModel
	var state hostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	currentName := strings.TrimSpace(state.Name.ValueString())
	newName := strings.TrimSpace(plan.Name.ValueString())
	if currentName == "" || newName == "" {
		resp.Diagnostics.AddError("Invalid name", "name must be provided")
		return
	}

	profile := ""
	if !plan.Profile.IsNull() && !plan.Profile.IsUnknown() {
		profile = strings.TrimSpace(plan.Profile.ValueString())
	}

	updateParts := []string{"set", "host"}
	changed := false
	if currentName != newName {
		updateParts = append(updateParts, "name", newName)
		changed = true
	}
	if profile != "" {
		updateParts = append(updateParts, "profile", profile)
		changed = true
	}
	updateParts = append(updateParts, currentName)

	if changed {
		if _, err := r.client.Execute(ctx, updateParts...); err != nil {
			resp.Diagnostics.AddError("Unable to update host", err.Error())
			return
		}
	}

	host, err := r.findHost(ctx, newName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read host after update", err.Error())
		return
	}

	newState, diag := hostStateFromModel(ctx, plan, host)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *hostResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state hostResourceModel
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
			"Host deletion not permitted",
			"Set allow_destroy = true to permit host deletion.",
		)
		return
	}

	name := strings.TrimSpace(state.Name.ValueString())
	if name == "" {
		resp.Diagnostics.AddError("Invalid state", "name is required for deletion")
		return
	}

	_, err := r.client.Execute(ctx, "delete", "hosts", name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete host", err.Error())
		return
	}
}

func (r *hostResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

var errHostNotFound = errors.New("host not found")

func (r *hostResource) findHost(ctx context.Context, name string) (*msa.Host, error) {
	response, err := r.client.Execute(ctx, "show", "host-groups")
	if err != nil {
		return nil, err
	}

	hosts := msa.HostsFromResponse(response)
	for _, host := range hosts {
		if strings.EqualFold(host.Name, name) {
			return &host, nil
		}
	}

	return nil, errHostNotFound
}

func (r *hostResource) waitForHost(ctx context.Context, name string) (*msa.Host, error) {
	waits := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
	for i, wait := range waits {
		host, err := r.findHost(ctx, name)
		if err == nil {
			return host, nil
		}
		if !errors.Is(err, errHostNotFound) {
			return nil, err
		}
		if i < len(waits)-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	return nil, errHostNotFound
}

func hostStateFromModel(ctx context.Context, model hostResourceModel, host *msa.Host) (hostResourceModel, diag.Diagnostics) {
	state := model
	var diags diag.Diagnostics

	state.Name = types.StringValue(host.Name)
	if host.SerialNumber != "" {
		state.SerialNumber = types.StringValue(host.SerialNumber)
		state.ID = types.StringValue(host.SerialNumber)
	} else if host.DurableID != "" {
		state.ID = types.StringValue(host.DurableID)
	}
	if host.DurableID != "" {
		state.DurableID = types.StringValue(host.DurableID)
	}
	if host.HostGroup != "" {
		state.HostGroup = types.StringValue(host.HostGroup)
	}
	if host.GroupKey != "" {
		state.GroupKey = types.StringValue(host.GroupKey)
	}
	state.MemberCount = types.Int64Value(int64(host.MemberCount))

	propsValue, diag := types.MapValueFrom(ctx, types.StringType, host.Properties)
	if diag.HasError() {
		diags.Append(diag...)
		return state, diags
	}
	state.Properties = propsValue

	return state, diags
}

func setToStrings(ctx context.Context, value types.Set) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return nil, diags
	}
	var items []string
	diags.Append(value.ElementsAs(ctx, &items, false)...)
	if diags.HasError() {
		return nil, diags
	}

	cleaned := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		cleaned = append(cleaned, item)
	}
	return cleaned, diags
}
