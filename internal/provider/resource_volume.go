package provider

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
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
	resp.TypeName = req.ProviderTypeName + "_msa_volume"
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

	var configPool types.String
	var configVDisk types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("pool"), &configPool)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("vdisk"), &configVDisk)...)
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
		if errors.Is(err, errVolumeTargetMissing) {
			target, err = r.defaultPool(ctx)
			if err != nil {
				resp.Diagnostics.AddError("Missing pool or vdisk", err.Error())
				return
			}
		} else if errors.Is(err, errVolumeTargetUnknown) {
			if configPool.IsNull() && configVDisk.IsNull() {
				target, err = r.defaultPool(ctx)
				if err != nil {
					resp.Diagnostics.AddError("Missing pool or vdisk", err.Error())
					return
				}
			} else {
				resp.Diagnostics.AddError("Pool/vdisk unknown", "pool or vdisk must be known during planning to create a volume")
				return
			}
		} else {
			resp.Diagnostics.AddError("Invalid configuration", err.Error())
			return
		}
	}

	_, err = r.findVolume(ctx, name, "")
	if err == nil {
		resp.Diagnostics.AddError("Volume already exists", "Import the volume or choose a different name.")
		return
	}
	if err != nil && !errors.Is(err, errVolumeNotFound) {
		resp.Diagnostics.AddError("Unable to check existing volumes", err.Error())
		return
	}

	shouldValidate := false
	// MSA XML API expects pool + access parameters for volume creation.
	_, err = r.client.Execute(ctx, "create", "volume", name, "pool", target, "size", size, "access", "no-access")
	if err != nil {
		var apiErr msa.APIError
		if errors.As(err, &apiErr) {
			msg := strings.ToLower(apiErr.Status.Response)
			if strings.Contains(msg, "volume was created") || strings.Contains(msg, "name is already in use") || strings.Contains(msg, "name already in use") {
				// Some firmware revisions report a non-zero response even though the volume exists.
				shouldValidate = true
			} else {
				resp.Diagnostics.AddError("Unable to create volume", err.Error())
				return
			}
		} else {
			resp.Diagnostics.AddError("Unable to create volume", err.Error())
			return
		}
	}

	volume, err := r.waitForVolume(ctx, plan.Name.ValueString(), "")
	if err != nil {
		resp.Diagnostics.AddError("Unable to read volume after create", err.Error())
		return
	}

	if shouldValidate {
		if !volumeMatchesTarget(volume, target) {
			resp.Diagnostics.AddError(
				"Volume name collision",
				fmt.Sprintf("Volume %q exists but does not match pool/vdisk %q.", name, target),
			)
			return
		}

		match, err := volumeSizeMatches(size, volume)
		if err != nil {
			resp.Diagnostics.AddError("Unable to verify existing volume size", err.Error())
			return
		}
		if !match {
			resp.Diagnostics.AddError(
				"Volume name collision",
				fmt.Sprintf("Volume %q exists but does not match requested size %q.", name, size),
			)
			return
		}
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
var errVolumeTargetMissing = errors.New("volume target missing")
var errVolumeTargetConflict = errors.New("volume target conflict")
var errVolumeTargetUnknown = errors.New("volume target unknown")

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
	poolValue := strings.TrimSpace(plan.Pool.ValueString())
	vdiskValue := strings.TrimSpace(plan.VDisk.ValueString())

	poolKnown := !plan.Pool.IsUnknown() && poolValue != ""
	vdiskKnown := !plan.VDisk.IsUnknown() && vdiskValue != ""

	switch {
	case poolKnown && vdiskKnown:
		return "", errVolumeTargetConflict
	case poolKnown:
		return poolValue, nil
	case vdiskKnown:
		return vdiskValue, nil
	case plan.Pool.IsUnknown() || plan.VDisk.IsUnknown():
		return "", errVolumeTargetUnknown
	case poolValue == "" && vdiskValue == "":
		return "", errVolumeTargetMissing
	default:
		return "", errVolumeTargetMissing
	}
}

func (r *volumeResource) defaultPool(ctx context.Context) (string, error) {
	response, err := r.client.Execute(ctx, "show", "pools")
	if err != nil {
		return "", fmt.Errorf("unable to query pools: %w", err)
	}

	names := poolNamesFromResponse(response)
	if len(names) == 1 {
		return names[0], nil
	}
	if len(names) == 0 {
		return "", errors.New("no pools were returned; set pool or vdisk explicitly")
	}
	return "", fmt.Errorf("multiple pools found; set pool or vdisk explicitly (%s)", strings.Join(names, ", "))
}

func poolNamesFromResponse(response msa.Response) []string {
	names := make([]string, 0)
	seen := make(map[string]struct{})
	for _, obj := range response.ObjectsWithoutStatus() {
		if obj.BaseType != "pools" && obj.BaseType != "pool" {
			if _, ok := obj.PropertyValue("pool-name"); !ok {
				continue
			}
		}
		props := obj.PropertyMap()
		if obj.Name == "pools" && props["pool-name"] == "" && props["serial-number"] == "" {
			continue
		}
		name := firstNonEmpty(props["pool-name"], props["name"], obj.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}
	return names
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

func volumeMatchesTarget(volume *msa.Volume, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return true
	}
	if strings.EqualFold(volume.PoolName, target) {
		return true
	}
	if strings.EqualFold(volume.VDiskName, target) {
		return true
	}
	return false
}

func volumeSizeMatches(planSize string, volume *msa.Volume) (bool, error) {
	planBytes, err := parseSizeToBytes(planSize)
	if err != nil {
		return false, err
	}
	if volume.SizeNumeric == "" {
		return false, errors.New("volume size-numeric is missing")
	}
	blocks, err := strconv.ParseInt(volume.SizeNumeric, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid size-numeric %q", volume.SizeNumeric)
	}
	volumeBytes := blocks * 512
	diff := int64(math.Abs(float64(planBytes - volumeBytes)))
	tolerance := sizeTolerance(planBytes)
	return diff <= tolerance, nil
}

func sizeTolerance(planBytes int64) int64 {
	const minTolerance = int64(8 * 1024 * 1024)
	relative := int64(float64(planBytes) * 0.001)
	if relative < minTolerance {
		return minTolerance
	}
	return relative
}

func parseSizeToBytes(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("size is required")
	}

	matches := regexp.MustCompile(`^([0-9]*\.?[0-9]+)\s*([A-Za-z]+)?$`).FindStringSubmatch(raw)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid size %q", raw)
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("invalid size %q", raw)
	}

	unit := strings.ToUpper(strings.TrimSpace(matches[2]))
	if unit == "" {
		return 0, fmt.Errorf("invalid size %q", raw)
	}

	decimalUnits := map[string]float64{
		"B":  1,
		"KB": 1e3,
		"MB": 1e6,
		"GB": 1e9,
		"TB": 1e12,
		"PB": 1e15,
		"K":  1e3,
		"M":  1e6,
		"G":  1e9,
		"T":  1e12,
		"P":  1e15,
	}
	binaryUnits := map[string]float64{
		"KIB": 1024,
		"MIB": 1024 * 1024,
		"GIB": 1024 * 1024 * 1024,
		"TIB": 1024 * 1024 * 1024 * 1024,
		"PIB": 1024 * 1024 * 1024 * 1024 * 1024,
	}

	if multiplier, ok := decimalUnits[unit]; ok {
		return int64(value*multiplier + 0.5), nil
	}
	if multiplier, ok := binaryUnits[unit]; ok {
		return int64(value*multiplier + 0.5), nil
	}

	return 0, fmt.Errorf("invalid size unit %q", unit)
}
