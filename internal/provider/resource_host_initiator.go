package provider

import (
	"context"
	"errors"
	"strings"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*hostInitiatorResource)(nil)
var _ resource.ResourceWithImportState = (*hostInitiatorResource)(nil)

func NewHostInitiatorResource() resource.Resource {
	return &hostInitiatorResource{}
}

type hostInitiatorResource struct {
	client *msa.Client
}

type hostInitiatorResourceModel struct {
	ID          types.String `tfsdk:"id"`
	HostName    types.String `tfsdk:"host_name"`
	InitiatorID types.String `tfsdk:"initiator_id"`
}

func (r *hostInitiatorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_msa_host_initiator"
}

func (r *hostInitiatorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Host/initiator association identifier.",
				Computed:    true,
			},
			"host_name": schema.StringAttribute{
				Description: "Host name.",
				Required:    true,
				Validators: []validator.String{
					hostNameValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"initiator_id": schema.StringAttribute{
				Description: "Initiator ID or nickname to attach to the host.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *hostInitiatorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *hostInitiatorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostInitiatorResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	hostName := strings.TrimSpace(plan.HostName.ValueString())
	initID := strings.TrimSpace(plan.InitiatorID.ValueString())
	if hostName == "" || initID == "" {
		resp.Diagnostics.AddError("Invalid configuration", "host_name and initiator_id are required")
		return
	}

	_, err := r.client.Execute(ctx, "add", "host-members", "initiators", initID, hostName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to add host member", err.Error())
		return
	}

	plan.ID = types.StringValue(hostInitiatorID(hostName, initID))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostInitiatorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostInitiatorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	hostName := strings.TrimSpace(state.HostName.ValueString())
	initID := strings.TrimSpace(state.InitiatorID.ValueString())
	if hostName == "" || initID == "" {
		resp.Diagnostics.AddError("Invalid state", "host_name and initiator_id are required")
		return
	}

	hosts, err := r.fetchHosts(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to query hosts", err.Error())
		return
	}

	host, ok := hosts[normalizeName(hostName)]
	if !ok {
		resp.State.RemoveResource(ctx)
		return
	}

	initiator, err := r.fetchInitiator(ctx, initID)
	if err != nil {
		if errors.Is(err, errInitiatorNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to query initiators", err.Error())
		return
	}

	if !initiatorMatchesHost(initiator, host) {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(hostInitiatorID(hostName, initID))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *hostInitiatorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported", "Change host_name or initiator_id by recreating the resource.")
}

func (r *hostInitiatorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state hostInitiatorResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	hostName := strings.TrimSpace(state.HostName.ValueString())
	initID := strings.TrimSpace(state.InitiatorID.ValueString())
	if hostName == "" || initID == "" {
		resp.Diagnostics.AddError("Invalid state", "host_name and initiator_id are required")
		return
	}

	_, err := r.client.Execute(ctx, "remove", "host-members", "initiators", initID, hostName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to remove host member", err.Error())
		return
	}
}

func (r *hostInitiatorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected host_name:initiator_id")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("initiator_id"), parts[1])...)
}

func (r *hostInitiatorResource) fetchHosts(ctx context.Context) (map[string]msa.Host, error) {
	response, err := r.client.Execute(ctx, "show", "host-groups")
	if err != nil {
		return nil, err
	}

	hosts := make(map[string]msa.Host)
	for _, host := range msa.HostsFromResponse(response) {
		if host.Name == "" {
			continue
		}
		hosts[normalizeName(host.Name)] = host
	}
	return hosts, nil
}

func (r *hostInitiatorResource) fetchInitiator(ctx context.Context, id string) (*msa.Initiator, error) {
	response, err := r.client.Execute(ctx, "show", "initiators")
	if err != nil {
		return nil, err
	}

	for _, initiator := range msa.InitiatorsFromResponse(response) {
		if strings.EqualFold(initiator.ID, id) || strings.EqualFold(initiator.Nickname, id) {
			return &initiator, nil
		}
	}
	return nil, errInitiatorNotFound
}

func initiatorMatchesHost(initiator *msa.Initiator, host msa.Host) bool {
	if initiator == nil {
		return false
	}
	if host.DurableID != "" && strings.EqualFold(initiator.HostKey, host.DurableID) {
		return true
	}
	if host.SerialNumber != "" && strings.EqualFold(initiator.HostID, host.SerialNumber) {
		return true
	}
	return false
}

func hostInitiatorID(hostName, initiatorID string) string {
	return hostName + ":" + initiatorID
}
