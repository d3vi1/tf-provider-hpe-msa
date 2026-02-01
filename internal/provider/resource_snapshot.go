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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*snapshotResource)(nil)
var _ resource.ResourceWithImportState = (*snapshotResource)(nil)

func NewSnapshotResource() resource.Resource {
	return &snapshotResource{}
}

type snapshotResource struct {
	client *msa.Client
}

type snapshotResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	VolumeName   types.String `tfsdk:"volume_name"`
	SerialNumber types.String `tfsdk:"serial_number"`
	DurableID    types.String `tfsdk:"durable_id"`
	Pool         types.String `tfsdk:"pool"`
	VDisk        types.String `tfsdk:"vdisk"`
	Size         types.String `tfsdk:"size"`
	Properties   types.Map    `tfsdk:"properties"`
	AllowDestroy types.Bool   `tfsdk:"allow_destroy"`
}

func (r *snapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_msa_snapshot"
}

func (r *snapshotResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Snapshot identifier (serial number).",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Snapshot name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"volume_name": schema.StringAttribute{
				Description: "Source volume name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"serial_number": schema.StringAttribute{
				Description: "Snapshot serial number.",
				Computed:    true,
			},
			"durable_id": schema.StringAttribute{
				Description: "Durable ID reported by the array.",
				Computed:    true,
			},
			"pool": schema.StringAttribute{
				Description: "Pool name.",
				Computed:    true,
			},
			"vdisk": schema.StringAttribute{
				Description: "Virtual disk name.",
				Computed:    true,
			},
			"size": schema.StringAttribute{
				Description: "Snapshot size reported by the array.",
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Raw properties returned by the XML API.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"allow_destroy": schema.BoolAttribute{
				Description: "Require explicit opt-in to delete snapshots.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *snapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *snapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan snapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	name := strings.TrimSpace(plan.Name.ValueString())
	volumeName := strings.TrimSpace(plan.VolumeName.ValueString())
	if name == "" || volumeName == "" {
		resp.Diagnostics.AddError("Invalid configuration", "name and volume_name are required")
		return
	}

	_, err := r.findSnapshot(ctx, name, "")
	if err == nil {
		resp.Diagnostics.AddError("Snapshot already exists", "Import the snapshot or choose a different name.")
		return
	}
	if err != nil && !errors.Is(err, errSnapshotNotFound) {
		resp.Diagnostics.AddError("Unable to check existing snapshots", err.Error())
		return
	}

	shouldValidate := false
	_, err = r.client.Execute(ctx, "create", "snapshots", "volumes", volumeName, name)
	if err != nil {
		var apiErr msa.APIError
		if errors.As(err, &apiErr) {
			msg := strings.ToLower(apiErr.Status.Response)
			if strings.Contains(msg, "snapshot(s) were created") {
				shouldValidate = true
			} else if strings.Contains(msg, "name") && strings.Contains(msg, "already") {
				shouldValidate = true
			} else {
				resp.Diagnostics.AddError("Unable to create snapshot", err.Error())
				return
			}
		} else {
			resp.Diagnostics.AddError("Unable to create snapshot", err.Error())
			return
		}
	}

	snapshot, err := r.waitForSnapshot(ctx, name, "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read snapshot after create", err.Error())
		return
	}

	if shouldValidate && !strings.EqualFold(snapshot.BaseVolumeName, volumeName) {
		resp.Diagnostics.AddError(
			"Snapshot name collision",
			fmt.Sprintf("Snapshot %q exists but does not belong to volume %q.", name, volumeName),
		)
		return
	}

	state, diags := snapshotStateFromModel(ctx, plan, snapshot)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *snapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state snapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	id := strings.TrimSpace(state.ID.ValueString())
	snapshot, err := r.findSnapshot(ctx, state.Name.ValueString(), id)
	if err != nil {
		if errors.Is(err, errSnapshotNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read snapshot", err.Error())
		return
	}

	newState, diags := snapshotStateFromModel(ctx, state, snapshot)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *snapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported", "Snapshot updates require replacement")
}

func (r *snapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state snapshotResourceModel
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
			"Set allow_destroy = true to permit snapshot deletion.",
		)
		return
	}

	snapshot, err := r.findSnapshot(ctx, state.Name.ValueString(), state.ID.ValueString())
	if err != nil {
		if errors.Is(err, errSnapshotNotFound) {
			return
		}
		resp.Diagnostics.AddError("Unable to read snapshot for deletion", err.Error())
		return
	}

	if !state.ID.IsNull() && state.ID.ValueString() != "" && snapshot.SerialNumber != state.ID.ValueString() {
		resp.Diagnostics.AddError("Snapshot mismatch", "Snapshot serial number does not match state")
		return
	}
	if !state.VolumeName.IsNull() && state.VolumeName.ValueString() != "" && !strings.EqualFold(snapshot.BaseVolumeName, state.VolumeName.ValueString()) {
		resp.Diagnostics.AddError("Snapshot mismatch", "Snapshot volume does not match state")
		return
	}

	target := strings.TrimSpace(snapshot.Name)
	if target == "" {
		resp.Diagnostics.AddError("Invalid state", "Snapshot name is required for deletion")
		return
	}

	_, err = r.client.Execute(ctx, "delete", "snapshot", target)
	if err != nil {
		resp.Diagnostics.AddError("Unable to delete snapshot", err.Error())
		return
	}
}

func (r *snapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

var errSnapshotNotFound = errors.New("snapshot not found")

func (r *snapshotResource) findSnapshot(ctx context.Context, name, id string) (*msa.Snapshot, error) {
	response, err := r.client.Execute(ctx, "show", "snapshots")
	if err != nil {
		return nil, err
	}

	snapshots := msa.SnapshotsFromResponse(response)
	for _, snapshot := range snapshots {
		if id != "" && snapshot.SerialNumber == id {
			return &snapshot, nil
		}
	}

	for _, snapshot := range snapshots {
		if strings.EqualFold(snapshot.Name, name) {
			return &snapshot, nil
		}
	}

	return nil, errSnapshotNotFound
}

func (r *snapshotResource) waitForSnapshot(ctx context.Context, name, id string) (*msa.Snapshot, error) {
	waits := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second}
	for i, wait := range waits {
		snapshot, err := r.findSnapshot(ctx, name, id)
		if err == nil {
			return snapshot, nil
		}
		if !errors.Is(err, errSnapshotNotFound) {
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
	return nil, errSnapshotNotFound
}

func snapshotStateFromModel(ctx context.Context, model snapshotResourceModel, snapshot *msa.Snapshot) (snapshotResourceModel, diag.Diagnostics) {
	state := model
	state.Name = types.StringValue(snapshot.Name)

	if snapshot.BaseVolumeName != "" {
		state.VolumeName = types.StringValue(snapshot.BaseVolumeName)
	}
	if snapshot.DurableID != "" {
		state.DurableID = types.StringValue(snapshot.DurableID)
	}
	if snapshot.SerialNumber != "" {
		state.SerialNumber = types.StringValue(snapshot.SerialNumber)
		state.ID = types.StringValue(snapshot.SerialNumber)
	}
	if snapshot.PoolName != "" {
		state.Pool = types.StringValue(snapshot.PoolName)
	}
	if snapshot.VDiskName != "" {
		state.VDisk = types.StringValue(snapshot.VDiskName)
	}
	if snapshot.Size != "" {
		state.Size = types.StringValue(snapshot.Size)
	}

	propsValue, diags := types.MapValueFrom(ctx, types.StringType, snapshot.Properties)
	if diags.HasError() {
		return state, diags
	}
	state.Properties = propsValue

	return state, diags
}
