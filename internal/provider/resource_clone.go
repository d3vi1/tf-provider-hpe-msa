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
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*cloneResource)(nil)
var _ resource.ResourceWithImportState = (*cloneResource)(nil)

const (
	cloneCopyConflictETAMaxRetries = 3
	cloneCopyETASafetyBuffer       = 5 * time.Second
	cloneRetryPathETA              = "eta"
	cloneRetryPathNoETA            = "no-eta"
)

var cloneCopyConflictNoETAWaits = []time.Duration{
	15 * time.Second,
	30 * time.Second,
	45 * time.Second,
	180 * time.Second,
	300 * time.Second,
}

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
	SCSIWWN         types.String `tfsdk:"scsi_wwn"`
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
			"scsi_wwn": schema.StringAttribute{
				Description: "Host-visible SCSI WWN/NAA identifier reported by the array.",
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

	err = r.executeCloneCopy(ctx, source, name, parts...)
	if err != nil {
		if isCloneAlreadyExistsError(err) {
			resp.Diagnostics.AddError("Clone already exists", "Import the clone or choose a different name.")
			return
		}
		resp.Diagnostics.AddError("Unable to copy volume", err.Error())
		return
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

type cloneConflictRetryStrategy int

const (
	cloneConflictRetryStrategyUnset cloneConflictRetryStrategy = iota
	cloneConflictRetryStrategyETA
	cloneConflictRetryStrategyNoETA
)

type cloneConflictRetryPlanner struct {
	strategy     cloneConflictRetryStrategy
	etaRetries   int
	noETARetries int
	lastETA      time.Duration
}

func (p *cloneConflictRetryPlanner) next(job *msa.VolumeCopyJob) (time.Duration, string, bool) {
	if p.strategy == cloneConflictRetryStrategyUnset {
		if job != nil && job.HasETA {
			p.strategy = cloneConflictRetryStrategyETA
		} else {
			p.strategy = cloneConflictRetryStrategyNoETA
		}
	}

	if job != nil && job.HasETA {
		p.lastETA = job.ETA
	}

	switch p.strategy {
	case cloneConflictRetryStrategyETA:
		if p.etaRetries >= cloneCopyConflictETAMaxRetries {
			return 0, cloneRetryPathETA, false
		}
		wait := cloneCopyETASafetyBuffer
		if p.lastETA > 0 {
			wait += p.lastETA
		}
		p.etaRetries++
		return wait, cloneRetryPathETA, true
	default:
		if p.noETARetries >= len(cloneCopyConflictNoETAWaits) {
			return 0, cloneRetryPathNoETA, false
		}
		wait := cloneCopyConflictNoETAWaits[p.noETARetries]
		p.noETARetries++
		return wait, cloneRetryPathNoETA, true
	}
}

type cloneConflictContext struct {
	jobID  string
	source string
	target string
	eta    string
}

func newCloneConflictContext(source, target string) cloneConflictContext {
	return cloneConflictContext{
		source: strings.TrimSpace(source),
		target: strings.TrimSpace(target),
	}
}

func (c *cloneConflictContext) update(job *msa.VolumeCopyJob) {
	if job == nil {
		return
	}

	if value := strings.TrimSpace(job.ID); value != "" {
		c.jobID = value
	}
	if value := strings.TrimSpace(job.Source); value != "" {
		c.source = value
	}
	if value := strings.TrimSpace(job.Target); value != "" {
		c.target = value
	}
	if job.HasETA {
		c.eta = job.ETA.String()
	} else if c.eta == "" {
		if value := strings.TrimSpace(job.ETARaw); value != "" {
			c.eta = value
		}
	}
}

func (c cloneConflictContext) fields() map[string]any {
	fields := map[string]any{}
	if c.jobID != "" {
		fields["job_id"] = c.jobID
	}
	if c.source != "" {
		fields["job_source"] = c.source
	}
	if c.target != "" {
		fields["job_target"] = c.target
	}
	if c.eta != "" {
		fields["job_eta"] = c.eta
	}
	return fields
}

func (c cloneConflictContext) String() string {
	jobID := c.jobID
	if jobID == "" {
		jobID = "unknown"
	}

	source := c.source
	if source == "" {
		source = "unknown"
	}

	target := c.target
	if target == "" {
		target = "unknown"
	}

	eta := c.eta
	if eta == "" {
		eta = "unknown"
	}

	return fmt.Sprintf("job id=%s source=%s target=%s eta=%s", jobID, source, target, eta)
}

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

func (r *cloneResource) executeCloneCopy(ctx context.Context, source, target string, parts ...string) error {
	_, err := r.client.Execute(ctx, parts...)
	if err == nil {
		return nil
	}
	if isCloneAlreadyExistsError(err) {
		return err
	}
	if !isCloneCopyConflictError(err) {
		return err
	}

	return r.retryCloneCopyConflict(ctx, source, target, parts, err)
}

func (r *cloneResource) retryCloneCopyConflict(ctx context.Context, source, target string, parts []string, initialErr error) error {
	planner := cloneConflictRetryPlanner{}
	contextState := newCloneConflictContext(source, target)
	lastErr := initialErr
	attempts := 1

	for {
		job, lookupErr := r.client.FindActiveVolumeCopyJob(ctx, source, target)
		if lookupErr != nil {
			tflog.Warn(ctx, "Unable to query active volume-copy job during clone retry", map[string]any{
				"attempt":      attempts,
				"lookup_error": lookupErr.Error(),
			})
		}
		contextState.update(job)

		wait, retryPath, ok := planner.next(job)
		if !ok {
			return fmt.Errorf(
				"copy volume failed after %d attempt(s); conflict context: %s: %w",
				attempts,
				contextState.String(),
				lastErr,
			)
		}

		fields := contextState.fields()
		fields["attempt"] = attempts
		fields["retry_path"] = retryPath
		fields["wait_seconds"] = int(wait / time.Second)
		tflog.Info(ctx, "Clone copy blocked by active volume-copy; waiting before retry", fields)

		if err := sleepWithContext(ctx, wait); err != nil {
			return fmt.Errorf(
				"copy volume retry interrupted after %d attempt(s); conflict context: %s: %w",
				attempts,
				contextState.String(),
				err,
			)
		}

		_, err := r.client.Execute(ctx, parts...)
		attempts++
		if err == nil {
			return nil
		}

		lastErr = err
		if isCloneAlreadyExistsError(err) {
			return err
		}
		if !isCloneCopyConflictError(err) {
			return err
		}
	}
}

func isCloneAlreadyExistsError(err error) bool {
	var apiErr msa.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	msg := strings.ToLower(apiErr.Status.Response)
	return strings.Contains(msg, "name already in use") || strings.Contains(msg, "already exists")
}

func isCloneCopyConflictError(err error) bool {
	var apiErr msa.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	msg := strings.ToLower(apiErr.Status.Response)
	return strings.Contains(msg, "existing volume copy in progress")
}

func sleepWithContext(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
			if err := sleepWithContext(ctx, wait); err != nil {
				return nil, err
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
	if volume.WWN != "" {
		state.SCSIWWN = types.StringValue(volume.WWN)
	} else {
		state.SCSIWWN = types.StringNull()
	}

	return state
}
