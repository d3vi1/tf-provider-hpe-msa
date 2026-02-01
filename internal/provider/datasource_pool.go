package provider

import (
	"context"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = (*poolDataSource)(nil)

func NewPoolDataSource() datasource.DataSource {
	return &poolDataSource{}
}

type poolDataSource struct {
	client *msa.Client
}

type poolDataSourceModel struct {
	Name       types.String `tfsdk:"name"`
	ID         types.String `tfsdk:"id"`
	Properties types.Map    `tfsdk:"properties"`
}

func (d *poolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (d *poolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Pool name to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "Pool identifier.",
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

func (d *poolDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *poolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data poolDataSourceModel
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

	response, err := d.client.Execute(ctx, "show", "pools")
	if err != nil {
		resp.Diagnostics.AddError("Unable to query pools", err.Error())
		return
	}

	obj, diags := findObjectByName(response, data.Name.ValueString(), []string{"name", "pool-name"}, "pool")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	props := obj.PropertyMap()
	propsValue, diag := types.MapValueFrom(ctx, types.StringType, props)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	data.ID = types.StringValue(firstNonEmpty(props["serial-number"], obj.OID, data.Name.ValueString()))
	data.Properties = propsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
