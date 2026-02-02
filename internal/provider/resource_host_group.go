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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*hostGroupResource)(nil)
var _ resource.ResourceWithImportState = (*hostGroupResource)(nil)

func NewHostGroupResource() resource.Resource {
	return &hostGroupResource{}
}

type hostGroupResource struct {
	client *msa.Client
}

type hostGroupResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Hosts        types.Set    `tfsdk:"hosts"`
	DurableID    types.String `tfsdk:"durable_id"`
	SerialNumber types.String `tfsdk:"serial_number"`
	MemberCount  types.Int64  `tfsdk:"member_count"`
	Properties   types.Map    `tfsdk:"properties"`
	AllowDestroy types.Bool   `tfsdk:"allow_destroy"`
}

func (r *hostGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_msa_host_group"
}

func (r *hostGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Host group identifier (serial number if available).",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Host group name (case-sensitive, max 32 bytes).",
				Required:    true,
				Validators: []validator.String{
					hostGroupNameValidator{},
				},
			},
			"hosts": schema.SetAttribute{
				Description: "Host names to include in the host group.",
				Required:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"durable_id": schema.StringAttribute{
				Description: "Durable ID reported by the array.",
				Computed:    true,
			},
			"serial_number": schema.StringAttribute{
				Description: "Host group serial number reported by the array.",
				Computed:    true,
			},
			"member_count": schema.Int64Attribute{
				Description: "Number of hosts in the group.",
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Raw host group properties returned by the XML API.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"allow_destroy": schema.BoolAttribute{
				Description: "Require explicit opt-in to delete host groups.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *hostGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *hostGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostGroupResourceModel
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

	hosts, diag := setToStrings(ctx, plan.Hosts)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	hosts = uniqueHostNames(hosts)
	if len(hosts) == 0 {
		resp.Diagnostics.AddError("Invalid hosts", "at least one host is required to create a host group")
		return
	}

	if existing, err := r.findHostGroup(ctx, name); err == nil {
		resp.Diagnostics.AddError("Host group already exists", "Import the host group or choose a different name.")
		_ = existing
		return
	} else if !errors.Is(err, errHostGroupNotFound) {
		resp.Diagnostics.AddError("Unable to check existing host groups", err.Error())
		return
	}

	parts := []string{"create", "host-group", "hosts", strings.Join(hosts, ","), name}
	if _, err := r.client.Execute(ctx, parts...); err != nil {
		resp.Diagnostics.AddError("Unable to create host group", err.Error())
		return
	}

	group, err := r.waitForHostGroup(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read host group after create", err.Error())
		return
	}

	state, diag := hostGroupStateFromModel(ctx, plan, group)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *hostGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostGroupResourceModel
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

	group, err := r.findHostGroup(ctx, name)
	if err != nil {
		if errors.Is(err, errHostGroupNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read host group", err.Error())
		return
	}

	newState, diag := hostGroupStateFromModel(ctx, state, group)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *hostGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan hostGroupResourceModel
	var state hostGroupResourceModel
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
	desiredName := strings.TrimSpace(plan.Name.ValueString())
	if currentName == "" || desiredName == "" {
		resp.Diagnostics.AddError("Invalid name", "name must be provided")
		return
	}

	desiredHosts, diag := setToStrings(ctx, plan.Hosts)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(desiredHosts) == 0 {
		resp.Diagnostics.AddError("Invalid hosts", "at least one host must remain in a host group")
		return
	}

	if currentName != desiredName {
		if _, err := r.client.Execute(ctx, "set", "host-group", "name", desiredName, currentName); err != nil {
			resp.Diagnostics.AddError("Unable to rename host group", err.Error())
			return
		}
		currentName = desiredName
	}

	group, err := r.findHostGroup(ctx, currentName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read host group", err.Error())
		return
	}

	addHosts, removeHosts := diffHostGroupMembers(desiredHosts, hostNames(group.Hosts))
	if len(addHosts) > 0 {
		parts := []string{"add", "host-group-members", "hosts", strings.Join(addHosts, ","), currentName}
		if _, err := r.client.Execute(ctx, parts...); err != nil {
			resp.Diagnostics.AddError("Unable to add host group members", err.Error())
			return
		}
		group, err = r.findHostGroup(ctx, currentName)
		if err != nil {
			resp.Diagnostics.AddError("Unable to read host group after update", err.Error())
			return
		}
		_, removeHosts = diffHostGroupMembers(desiredHosts, hostNames(group.Hosts))
	}

	if len(removeHosts) > 0 {
		if len(removeHosts) >= len(group.Hosts) {
			resp.Diagnostics.AddError(
				"Cannot remove all hosts",
				"At least one host must remain in a host group. Delete the host group instead.",
			)
			return
		}
		parts := []string{"remove", "host-group-members", "hosts", strings.Join(removeHosts, ","), currentName}
		if _, err := r.client.Execute(ctx, parts...); err != nil {
			resp.Diagnostics.AddError("Unable to remove host group members", err.Error())
			return
		}
	}

	group, err = r.findHostGroup(ctx, currentName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read host group after update", err.Error())
		return
	}

	newState, diag := hostGroupStateFromModel(ctx, plan, group)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *hostGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state hostGroupResourceModel
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
			"Host group deletion not permitted",
			"Set allow_destroy = true to permit host group deletion.",
		)
		return
	}

	name := strings.TrimSpace(state.Name.ValueString())
	if name == "" {
		resp.Diagnostics.AddError("Invalid state", "name is required for deletion")
		return
	}

	if _, err := r.client.Execute(ctx, "delete", "host-groups", name); err != nil {
		resp.Diagnostics.AddError("Unable to delete host group", err.Error())
		return
	}
}

func (r *hostGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

var errHostGroupNotFound = errors.New("host group not found")

func (r *hostGroupResource) findHostGroup(ctx context.Context, name string) (*msa.HostGroup, error) {
	response, err := r.client.Execute(ctx, "show", "host-groups")
	if err != nil {
		return nil, err
	}

	groups := msa.HostGroupsFromResponse(response)
	for _, group := range groups {
		if strings.EqualFold(group.Name, name) {
			return &group, nil
		}
	}

	return nil, errHostGroupNotFound
}

func (r *hostGroupResource) waitForHostGroup(ctx context.Context, name string) (*msa.HostGroup, error) {
	waits := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
	for i, wait := range waits {
		group, err := r.findHostGroup(ctx, name)
		if err == nil {
			return group, nil
		}
		if !errors.Is(err, errHostGroupNotFound) {
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
	return nil, errHostGroupNotFound
}

func hostGroupStateFromModel(ctx context.Context, model hostGroupResourceModel, group *msa.HostGroup) (hostGroupResourceModel, diag.Diagnostics) {
	state := model
	var diags diag.Diagnostics

	state.Name = types.StringValue(group.Name)
	if group.SerialNumber != "" {
		state.SerialNumber = types.StringValue(group.SerialNumber)
		state.ID = types.StringValue(group.SerialNumber)
	} else if group.DurableID != "" {
		state.ID = types.StringValue(group.DurableID)
	} else if group.Name != "" {
		state.ID = types.StringValue(group.Name)
	}
	if group.DurableID != "" {
		state.DurableID = types.StringValue(group.DurableID)
	}
	state.MemberCount = types.Int64Value(int64(group.MemberCount))

	setValue, diag := types.SetValueFrom(ctx, types.StringType, hostNames(group.Hosts))
	if diag.HasError() {
		diags.Append(diag...)
		return state, diags
	}
	state.Hosts = setValue

	propsValue, diag := types.MapValueFrom(ctx, types.StringType, group.Properties)
	if diag.HasError() {
		diags.Append(diag...)
		return state, diags
	}
	state.Properties = propsValue

	return state, diags
}

func hostNames(hosts []msa.Host) []string {
	values := make([]string, 0, len(hosts))
	for _, host := range hosts {
		name := strings.TrimSpace(host.Name)
		if name == "" {
			continue
		}
		values = append(values, name)
	}
	return values
}

func diffHostGroupMembers(desired []string, actual []string) ([]string, []string) {
	desiredMap, desiredOrder := normalizedNameMap(desired)
	actualMap, actualOrder := normalizedNameMap(actual)

	add := make([]string, 0)
	for _, key := range desiredOrder {
		if _, ok := actualMap[key]; !ok {
			add = append(add, desiredMap[key])
		}
	}

	remove := make([]string, 0)
	for _, key := range actualOrder {
		if _, ok := desiredMap[key]; !ok {
			remove = append(remove, actualMap[key])
		}
	}

	return add, remove
}

func normalizedNameMap(values []string) (map[string]string, []string) {
	m := make(map[string]string)
	order := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := normalizeName(trimmed)
		if key == "" {
			continue
		}
		if _, ok := m[key]; ok {
			continue
		}
		m[key] = trimmed
		order = append(order, key)
	}
	return m, order
}

func uniqueHostNames(values []string) []string {
	normalized, order := normalizedNameMap(values)
	if len(order) == 0 {
		return nil
	}
	unique := make([]string, 0, len(order))
	for _, key := range order {
		unique = append(unique, normalized[key])
	}
	return unique
}
