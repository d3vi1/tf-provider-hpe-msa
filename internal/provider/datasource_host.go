package provider

import (
	"context"
	"strings"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = (*hostDataSource)(nil)

func NewHostDataSource() datasource.DataSource {
	return &hostDataSource{}
}

type hostDataSource struct {
	client *msa.Client
}

type hostDataSourceModel struct {
	Name       types.String `tfsdk:"name"`
	ID         types.String `tfsdk:"id"`
	HostID     types.String `tfsdk:"host_id"`
	Properties types.Map    `tfsdk:"properties"`
}

func (d *hostDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_msa_host"
}

func (d *hostDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Host name to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "Host identifier.",
				Computed:    true,
			},
			"host_id": schema.StringAttribute{
				Description: "Host serial number reported by the array.",
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Raw properties returned by the XML API.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *hostDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*msa.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type", "Expected *msa.Client")
		return
	}

	d.client = client
}

func (d *hostDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data hostDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	if data.Name.IsUnknown() || data.Name.IsNull() || data.Name.ValueString() == "" {
		resp.Diagnostics.AddError("Invalid name", "name must be provided")
		return
	}

	response, err := d.client.Execute(ctx, "show", "host-groups")
	if err != nil {
		resp.Diagnostics.AddError("Unable to query hosts", err.Error())
		return
	}

	hosts := msa.HostsFromResponse(response)
	var host *msa.Host
	for _, candidate := range hosts {
		if strings.EqualFold(candidate.Name, data.Name.ValueString()) {
			host = &candidate
			break
		}
	}
	if host == nil {
		resp.Diagnostics.AddError("Host not found", "No host with the requested name was returned by the array")
		return
	}

	props := host.Properties
	propsValue, diag := types.MapValueFrom(ctx, types.StringType, props)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	hostID := host.SerialNumber
	data.ID = types.StringValue(firstNonEmpty(hostID, host.DurableID, data.Name.ValueString()))
	if hostID != "" {
		data.HostID = types.StringValue(hostID)
	} else {
		data.HostID = types.StringNull()
	}
	data.Properties = propsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
