package provider

import (
	"context"
	"errors"
	"fmt"
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

var _ resource.Resource = (*volumeResource)(nil)

func NewVolumeResource() resource.Resource {
	return &volumeResource{}
}

type volumeResource struct {
	client *msa.Client
}

type volumeResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Size         types.String `tfsdk:"size"`
	Pool         types.String `tfsdk:"pool"`
	VDisk        types.String `tfsdk:"vdisk"`
	DurableID    types.String `tfsdk:"durable_id"`
	SerialNumber types.String `tfsdk:"serial_number"`
	WWID         types.String `tfsdk:"wwid"`
	AllowDestroy types.Bool   `tfsdk:"allow_destroy"`
}

func (r *volumeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_volume"
}

func (r *volumeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Volume identifier (serial number).",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Volume name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"size": schema.StringAttribute{
				Description: "Volume size (e.g., 100GB).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"pool": schema.StringAttribute{
				Description: "Pool/virtual disk name for volume placement.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vdisk": schema.StringAttribute{
				Description: "Virtual disk name (alias of pool).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"durable_id": schema.StringAttribute{
				Description: "Durable ID reported by the array.",
				Computed:    true,
			},
			"serial_number": schema.StringAttribute{
				Description: "Volume serial number.",
				Computed:    true,
			},
			"wwid": schema.StringAttribute{
				Description: "WWID derived from the array (serial number).",
				Computed:    true,
			},
			"allow_destroy": schema.BoolAttribute{
				Description: "Require explicit opt-in to delete volumes.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *volumeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *volumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan volumeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	name := strings.TrimSpace(plan.Name.ValueString())
	size := strings.TrimSpace(plan.Size.ValueString())
	if name == "" || size == "" {
		resp.Diagnostics.AddError("Invalid configuration", "name and size are required")
		return
	}

	target, err := resolveVolumeTarget(plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}

	// MSA XML API expects pool + access parameters for volume creation.
	_, err = r.client.Execute(ctx, "create", "volume", name, "pool", target, "size", size, "access", "no-access")
	if err != nil {
		resp.Diagnostics.AddError("Unable to create volume", err.Error())
		return
	}

	volume, err := r.waitForVolume(ctx, plan.Name.ValueString(), "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read volume after create", err.Error())
		return
	}

	state := volumeStateFromModel(plan, volume)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *volumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state volumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	id := strings.TrimSpace(state.ID.ValueString())
	volume, err := r.findVolume(ctx, state.Name.ValueString(), id)
	if err != nil {
		if errors.Is(err, errVolumeNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read volume", err.Error())
		return
	}

	newState := volumeStateFromModel(state, volume)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *volumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported", "Volume updates require replacement")
}

func (r *volumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state volumeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	if state.AllowDestroy.IsUnknown() || !state.AllowDestroy.ValueBool() {
		resp.Diagnostics.AddError(
			"Deletion blocked",
			"Set allow_destroy = true to permit volume deletion.",
		)
		return
	}

	id := strings.TrimSpace(state.ID.ValueString())
	target := id
	if target == "" {
		target = state.Name.ValueString()
	}
	if strings.TrimSpace(target) == "" {
		resp.Diagnostics.AddError("Invalid state", "Volume ID or name is required for deletion")
		return
	}

	_, err := r.client.Execute(ctx, "delete", "volumes", target)
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete volume", err.Error())
		return
	}
}

func (r *volumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

var errVolumeNotFound = errors.New("volume not found")

func (r *volumeResource) findVolume(ctx context.Context, name, id string) (*msa.Volume, error) {
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

func (r *volumeResource) waitForVolume(ctx context.Context, name, id string) (*msa.Volume, error) {
	waits := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
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

func resolveVolumeTarget(plan volumeResourceModel) (string, error) {
	pool := strings.TrimSpace(plan.Pool.ValueString())
	vdisk := strings.TrimSpace(plan.VDisk.ValueString())

	switch {
	case pool != "" && vdisk != "":
		return "", fmt.Errorf("only one of pool or vdisk can be set")
	case pool == "" && vdisk == "":
		return "", fmt.Errorf("either pool or vdisk must be set")
	case pool != "":
		return pool, nil
	default:
		return vdisk, nil
	}
}

func volumeStateFromModel(model volumeResourceModel, volume *msa.Volume) volumeResourceModel {
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
