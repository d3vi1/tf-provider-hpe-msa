package provider

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*cloneResource)(nil)
var _ resource.ResourceWithImportState = (*cloneResource)(nil)

func NewCloneResource() resource.Resource {
	return &cloneResource{}
}

type cloneResource struct {
	client *msa.Client
}

type cloneResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	SourceSnapshot  types.String `tfsdk:"source_snapshot"`
	DestinationPool types.String `tfsdk:"destination_pool"`
	Pool            types.String `tfsdk:"pool"`
	VDisk           types.String `tfsdk:"vdisk"`
	DurableID       types.String `tfsdk:"durable_id"`
	SerialNumber    types.String `tfsdk:"serial_number"`
	WWID            types.String `tfsdk:"wwid"`
	AllowDestroy    types.Bool   `tfsdk:"allow_destroy"`
}

func (r *cloneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_msa_clone"
}

func (r *cloneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Clone identifier (serial number).",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Destination volume name for the clone.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source_snapshot": schema.StringAttribute{
				Description: "Source snapshot name or serial number to copy.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"destination_pool": schema.StringAttribute{
				Description: "Optional destination pool name or serial number.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"pool": schema.StringAttribute{
				Description: "Pool name reported for the clone.",
				Computed:    true,
			},
			"vdisk": schema.StringAttribute{
				Description: "Virtual disk name reported for the clone.",
				Computed:    true,
			},
			"durable_id": schema.StringAttribute{
				Description: "Durable ID reported by the array.",
				Computed:    true,
			},
			"serial_number": schema.StringAttribute{
				Description: "Clone serial number.",
				Computed:    true,
			},
			"wwid": schema.StringAttribute{
				Description: "WWID derived from the array (serial number).",
				Computed:    true,
			},
			"allow_destroy": schema.BoolAttribute{
				Description: "Require explicit opt-in to delete clones.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *cloneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *cloneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cloneResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var configSource types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("source_snapshot"), &configSource)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	if configSource.IsNull() {
		resp.Diagnostics.AddError("Invalid configuration", "source_snapshot must be set to create a clone")
		return
	}
	if configSource.IsUnknown() {
		resp.Diagnostics.AddError("Invalid configuration", "source_snapshot must be known to create a clone")
		return
	}

	name := strings.TrimSpace(plan.Name.ValueString())
	if name == "" {
		resp.Diagnostics.AddError("Invalid configuration", "name is required")
		return
	}

	source, err := resolveCloneSnapshot(plan)
	if err != nil {
		switch {
		case errors.Is(err, errCloneSnapshotMissing):
			resp.Diagnostics.AddError("Invalid configuration", "source_snapshot must be set")
		case errors.Is(err, errCloneSnapshotUnknown):
			resp.Diagnostics.AddError("Invalid configuration", "source_snapshot must be known")
		default:
			resp.Diagnostics.AddError("Invalid configuration", err.Error())
		}
		return
	}

	parts := []string{"copy", "volume"}
	if !plan.DestinationPool.IsNull() && !plan.DestinationPool.IsUnknown() {
		pool := strings.TrimSpace(plan.DestinationPool.ValueString())
		if pool != "" {
			parts = append(parts, "destination-pool", pool)
		}
	} else if plan.DestinationPool.IsUnknown() {
		resp.Diagnostics.AddError("Invalid configuration", "destination_pool must be known")
		return
	}
	parts = append(parts, "name", name, source)

	_, err = r.client.Execute(ctx, parts...)
	if err != nil {
		var apiErr msa.APIError
		if errors.As(err, &apiErr) {
			msg := strings.ToLower(apiErr.Status.Response)
			if strings.Contains(msg, "name already in use") || strings.Contains(msg, "already exists") {
				resp.Diagnostics.AddError("Clone already exists", "Import the clone or choose a different name.")
				return
			} else {
				resp.Diagnostics.AddError("Unable to copy volume", err.Error())
				return
			}
		} else {
			resp.Diagnostics.AddError("Unable to copy volume", err.Error())
			return
		}
	}

	volume, err := r.waitForVolume(ctx, name, "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read clone after create", err.Error())
		return
	}

	state := cloneStateFromModel(plan, volume)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *cloneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cloneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	name := strings.TrimSpace(state.Name.ValueString())
	id := strings.TrimSpace(state.ID.ValueString())
	if name == "" && id == "" {
		resp.Diagnostics.AddError("Invalid state", "name or id is required")
		return
	}

	volume, err := r.findVolume(ctx, name, id)
	if err != nil {
		if errors.Is(err, errVolumeNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read clone", err.Error())
		return
	}

	newState := cloneStateFromModel(state, volume)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *cloneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported", "Change clone parameters by recreating the resource.")
}

func (r *cloneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cloneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	if state.AllowDestroy.IsUnknown() || state.AllowDestroy.IsNull() || !state.AllowDestroy.ValueBool() {
		resp.Diagnostics.AddError(
			"Deletion blocked",
			"Set allow_destroy = true to permit clone deletion.",
		)
		return
	}

	id := strings.TrimSpace(state.ID.ValueString())
	target := id
	if target == "" {
		target = state.Name.ValueString()
	}
	if strings.TrimSpace(target) == "" {
		resp.Diagnostics.AddError("Invalid state", "clone ID or name is required for deletion")
		return
	}

	_, err := r.client.Execute(ctx, "delete", "volumes", target)
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete clone", err.Error())
		return
	}
}

func (r *cloneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

var errCloneSnapshotMissing = errors.New("clone snapshot missing")
var errCloneSnapshotUnknown = errors.New("clone snapshot unknown")

func resolveCloneSnapshot(plan cloneResourceModel) (string, error) {
	if plan.SourceSnapshot.IsUnknown() {
		return "", errCloneSnapshotUnknown
	}

	value := strings.TrimSpace(plan.SourceSnapshot.ValueString())
	if value == "" {
		return "", errCloneSnapshotMissing
	}

	return value, nil
}

func (r *cloneResource) findVolume(ctx context.Context, name, id string) (*msa.Volume, error) {
	response, err := r.client.Execute(ctx, "show", "volumes")
	if err != nil {
		return nil, err
	}

	volumes := msa.VolumesFromResponse(response)
	for _, volume := range volumes {
		if id != "" && volume.SerialNumber == id {
			return &volume, nil
		}
	}

	for _, volume := range volumes {
		if strings.EqualFold(volume.Name, name) {
			return &volume, nil
		}
	}

	return nil, errVolumeNotFound
}

func (r *cloneResource) waitForVolume(ctx context.Context, name, id string) (*msa.Volume, error) {
	waits := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second}
	for i, wait := range waits {
		volume, err := r.findVolume(ctx, name, id)
		if err == nil {
			return volume, nil
		}
		if !errors.Is(err, errVolumeNotFound) {
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
	return nil, errVolumeNotFound
}

func cloneStateFromModel(model cloneResourceModel, volume *msa.Volume) cloneResourceModel {
	state := model
	state.Name = types.StringValue(volume.Name)

	if volume.PoolName != "" {
		state.Pool = types.StringValue(volume.PoolName)
	}
	if volume.VDiskName != "" {
		state.VDisk = types.StringValue(volume.VDiskName)
	}
	if volume.DurableID != "" {
		state.DurableID = types.StringValue(volume.DurableID)
	}
	if volume.SerialNumber != "" {
		state.SerialNumber = types.StringValue(volume.SerialNumber)
		state.ID = types.StringValue(volume.SerialNumber)
		state.WWID = types.StringValue(volume.SerialNumber)
	}

	return state
}
