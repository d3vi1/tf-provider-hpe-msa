package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*volumeMappingResource)(nil)
var _ resource.ResourceWithImportState = (*volumeMappingResource)(nil)

func NewVolumeMappingResource() resource.Resource {
	return &volumeMappingResource{}
}

type volumeMappingResource struct {
	client *msa.Client
}

type volumeMappingResourceModel struct {
	ID         types.String `tfsdk:"id"`
	VolumeName types.String `tfsdk:"volume_name"`
	TargetType types.String `tfsdk:"target_type"`
	TargetName types.String `tfsdk:"target_name"`
	Access     types.String `tfsdk:"access"`
	LUN        types.String `tfsdk:"lun"`
	Ports      types.Set    `tfsdk:"ports"`
	Properties types.Map    `tfsdk:"properties"`
}

func (r *volumeMappingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_msa_volume_mapping"
}

func (r *volumeMappingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Mapping identifier.",
				Computed:    true,
			},
			"volume_name": schema.StringAttribute{
				Description: "Volume name to map.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target_type": schema.StringAttribute{
				Description: "Mapping target type: host, host_group, or initiator.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"target_name": schema.StringAttribute{
				Description: "Host name, host group name, or initiator ID/nickname.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access": schema.StringAttribute{
				Description: "Access level: read-write (rw), read-only (ro), or no-access.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"lun": schema.StringAttribute{
				Description: "LUN for the mapping (required for explicit mappings unless access=no-access).",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ports": schema.SetAttribute{
				Description: "Controller ports to use for the mapping (e.g., [\"a1\", \"b1\"]).",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
			"properties": schema.MapAttribute{
				Description: "Raw mapping properties returned by the XML API.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *volumeMappingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *volumeMappingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan volumeMappingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	volume := strings.TrimSpace(plan.VolumeName.ValueString())
	if volume == "" {
		resp.Diagnostics.AddError("Invalid configuration", "volume_name is required")
		return
	}

	targetSpec, diag := buildTargetSpec(plan.TargetType, plan.TargetName)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	access, diag := normalizeAccess(plan.Access)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	ports, diag := setToStrings(ctx, plan.Ports)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	lun := strings.TrimSpace(plan.LUN.ValueString())
	if access != "no-access" {
		if lun == "" {
			resp.Diagnostics.AddError("Invalid configuration", "lun is required for explicit mappings")
			return
		}
	}
	if len(ports) > 0 && lun == "" {
		resp.Diagnostics.AddError("Invalid configuration", "lun is required when ports are specified")
		return
	}

	parts := []string{"map", "volume"}
	if access != "" {
		parts = append(parts, "access", access)
	}
	if len(ports) > 0 {
		parts = append(parts, "ports", strings.Join(ports, ","))
	}
	if lun != "" {
		parts = append(parts, "lun", lun)
	}
	// MSA maps hosts and host groups through the initiator parameter using host.* or hostgroup.*.* syntax.
	parts = append(parts, "initiator", targetSpec, volume)

	_, err := r.client.Execute(ctx, parts...)
	if err != nil {
		resp.Diagnostics.AddError("Unable to map volume", err.Error())
		return
	}

	mapping, err := r.waitForMapping(ctx, volume, targetSpec)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read mapping after create", err.Error())
		return
	}

	state, diag := mappingStateFromModel(ctx, plan, mapping)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(mappingID(volume, targetSpec))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *volumeMappingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state volumeMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	volume := strings.TrimSpace(state.VolumeName.ValueString())
	if volume == "" {
		resp.Diagnostics.AddError("Invalid state", "volume_name is required")
		return
	}

	targetSpec, diag := buildTargetSpec(state.TargetType, state.TargetName)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	mapping, err := r.findMapping(ctx, volume, targetSpec)
	if err != nil {
		if errors.Is(err, errMappingNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read mapping", err.Error())
		return
	}

	newState, diag := mappingStateFromModel(ctx, state, mapping)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	newState.ID = types.StringValue(mappingID(volume, targetSpec))

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *volumeMappingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported", "Change volume_name, target, or mapping parameters by recreating the resource.")
}

func (r *volumeMappingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state volumeMappingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	volume := strings.TrimSpace(state.VolumeName.ValueString())
	if volume == "" {
		resp.Diagnostics.AddError("Invalid state", "volume_name is required")
		return
	}

	targetSpec, diag := buildTargetSpec(state.TargetType, state.TargetName)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.Execute(ctx, "unmap", "volume", "initiator", targetSpec, volume)
	if err != nil {
		resp.Diagnostics.AddError("Unable to unmap volume", err.Error())
		return
	}
}

func (r *volumeMappingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 3)
	if len(parts) != 3 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected volume_name:target_type:target_name")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("volume_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target_type"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target_name"), parts[2])...)
}

var errMappingNotFound = errors.New("mapping not found")

func (r *volumeMappingResource) findMapping(ctx context.Context, volume, targetSpec string) (*msa.Mapping, error) {
	response, err := r.client.Execute(ctx, "show", "maps", "initiator", targetSpec)
	if err != nil {
		return nil, err
	}

	for _, mapping := range msa.MappingsFromResponse(response) {
		if strings.EqualFold(mapping.Volume, volume) {
			return &mapping, nil
		}
	}

	return nil, errMappingNotFound
}

func (r *volumeMappingResource) waitForMapping(ctx context.Context, volume, targetSpec string) (*msa.Mapping, error) {
	waits := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
	for i, wait := range waits {
		mapping, err := r.findMapping(ctx, volume, targetSpec)
		if err == nil {
			return mapping, nil
		}
		if !errors.Is(err, errMappingNotFound) {
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
	return nil, errMappingNotFound
}

func buildTargetSpec(targetType types.String, targetName types.String) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if targetType.IsUnknown() || targetType.IsNull() {
		diags.AddError("Invalid target_type", "target_type is required")
		return "", diags
	}
	if targetName.IsUnknown() || targetName.IsNull() || strings.TrimSpace(targetName.ValueString()) == "" {
		diags.AddError("Invalid target_name", "target_name is required")
		return "", diags
	}

	typeValue := strings.TrimSpace(targetType.ValueString())
	nameValue := strings.TrimSpace(targetName.ValueString())

	switch typeValue {
	case "host":
		return fmt.Sprintf("%s.*", nameValue), diags
	case "host_group":
		return fmt.Sprintf("%s.*.*", nameValue), diags
	case "initiator":
		return nameValue, diags
	default:
		diags.AddError("Invalid target_type", "target_type must be host, host_group, or initiator")
		return "", diags
	}
}

func normalizeAccess(value types.String) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return "read-write", diags
	}

	raw := strings.TrimSpace(value.ValueString())
	if raw == "" {
		return "read-write", diags
	}

	switch strings.ToLower(raw) {
	case "rw", "read-write":
		return "read-write", diags
	case "ro", "read-only":
		return "read-only", diags
	case "no-access":
		return "no-access", diags
	default:
		diags.AddError("Invalid access", "access must be read-write, read-only, no-access, rw, or ro")
		return "", diags
	}
}

func mappingStateFromModel(ctx context.Context, model volumeMappingResourceModel, mapping *msa.Mapping) (volumeMappingResourceModel, diag.Diagnostics) {
	state := model
	var diags diag.Diagnostics

	state.VolumeName = types.StringValue(mapping.Volume)
	if mapping.Access != "" {
		state.Access = types.StringValue(canonicalAccess(mapping.Access))
	} else if !model.Access.IsNull() && !model.Access.IsUnknown() && strings.TrimSpace(model.Access.ValueString()) != "" {
		state.Access = types.StringValue(strings.TrimSpace(model.Access.ValueString()))
	} else {
		state.Access = types.StringNull()
	}
	if mapping.LUN != "" {
		state.LUN = types.StringValue(mapping.LUN)
	} else if !model.LUN.IsNull() && !model.LUN.IsUnknown() && strings.TrimSpace(model.LUN.ValueString()) != "" {
		state.LUN = types.StringValue(strings.TrimSpace(model.LUN.ValueString()))
	} else {
		state.LUN = types.StringNull()
	}

	if !model.Ports.IsNull() && !model.Ports.IsUnknown() {
		ports := strings.TrimSpace(mapping.Ports)
		if ports != "" {
			portItems := strings.Split(ports, ",")
			cleaned := make([]string, 0, len(portItems))
			for _, item := range portItems {
				item = strings.TrimSpace(item)
				if item != "" {
					cleaned = append(cleaned, item)
				}
			}
			setValue, diag := types.SetValueFrom(ctx, types.StringType, cleaned)
			if diag.HasError() {
				diags.Append(diag...)
				return state, diags
			}
			state.Ports = setValue
		} else {
			state.Ports = types.SetNull(types.StringType)
		}
	} else {
		state.Ports = types.SetNull(types.StringType)
	}

	propsValue, diag := types.MapValueFrom(ctx, types.StringType, mapping.Properties)
	if diag.HasError() {
		diags.Append(diag...)
		return state, diags
	}
	state.Properties = propsValue

	return state, diags
}

func canonicalAccess(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "rw", "read-write":
		return "read-write"
	case "ro", "read-only":
		return "read-only"
	case "no-access":
		return "no-access"
	default:
		return strings.TrimSpace(value)
	}
}

func mappingID(volume, targetSpec string) string {
	return volume + ":" + targetSpec
}
