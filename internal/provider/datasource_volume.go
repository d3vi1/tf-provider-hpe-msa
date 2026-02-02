package provider

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/d3vi1/tf-provider-hpe-msa/internal/msa"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = (*volumeDataSource)(nil)

func NewVolumeDataSource() datasource.DataSource {
	return &volumeDataSource{}
}

type volumeDataSource struct {
	client *msa.Client
}

type volumeDataSourceModel struct {
	Name         types.String `tfsdk:"name"`
	NameRegex    types.String `tfsdk:"name_regex"`
	ID           types.String `tfsdk:"id"`
	SerialNumber types.String `tfsdk:"serial_number"`
	DurableID    types.String `tfsdk:"durable_id"`
	WWID         types.String `tfsdk:"wwid"`
	SCSIWWN      types.String `tfsdk:"scsi_wwn"`
	Pool         types.String `tfsdk:"pool"`
	VDisk        types.String `tfsdk:"vdisk"`
	Size         types.String `tfsdk:"size"`
	Properties   types.Map    `tfsdk:"properties"`
}

func (d *volumeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_msa_volume"
}

func (d *volumeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Exact volume name to look up.",
				Optional:    true,
			},
			"name_regex": schema.StringAttribute{
				Description: "Regex to match a volume name (first match wins after sorting by name).",
				Optional:    true,
			},
			"id": schema.StringAttribute{
				Description: "Volume identifier (serial number).",
				Computed:    true,
			},
			"serial_number": schema.StringAttribute{
				Description: "Volume serial number.",
				Computed:    true,
			},
			"durable_id": schema.StringAttribute{
				Description: "Durable ID reported by the array.",
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
			"pool": schema.StringAttribute{
				Description: "Pool name.",
				Computed:    true,
			},
			"vdisk": schema.StringAttribute{
				Description: "Virtual disk name.",
				Computed:    true,
			},
			"size": schema.StringAttribute{
				Description: "Volume size reported by the array.",
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

func (d *volumeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *volumeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data volumeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError("Provider not configured", "Missing MSA client")
		return
	}

	name := strings.TrimSpace(data.Name.ValueString())
	regex := strings.TrimSpace(data.NameRegex.ValueString())
	if name == "" && regex == "" {
		resp.Diagnostics.AddError("Invalid configuration", "either name or name_regex must be provided")
		return
	}
	if name != "" && regex != "" {
		resp.Diagnostics.AddError("Invalid configuration", "only one of name or name_regex can be provided")
		return
	}

	var matcher *regexp.Regexp
	if regex != "" {
		compiled, err := regexp.Compile(regex)
		if err != nil {
			resp.Diagnostics.AddError("Invalid name_regex", fmt.Sprintf("%q is not a valid regex", regex))
			return
		}
		matcher = compiled
	}

	response, err := d.client.Execute(ctx, "show", "volumes")
	if err != nil {
		resp.Diagnostics.AddError("Unable to query volumes", err.Error())
		return
	}

	volumes := msa.VolumesFromResponse(response)
	candidates := make([]msa.Volume, 0, len(volumes))
	for _, volume := range volumes {
		if name != "" && strings.EqualFold(volume.Name, name) {
			candidates = append(candidates, volume)
			break
		}
		if matcher != nil && matcher.MatchString(volume.Name) {
			candidates = append(candidates, volume)
		}
	}

	if len(candidates) == 0 {
		resp.Diagnostics.AddError("Volume not found", "no volume matched the supplied criteria")
		return
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Name == candidates[j].Name {
			return candidates[i].SerialNumber < candidates[j].SerialNumber
		}
		return candidates[i].Name < candidates[j].Name
	})
	volume := candidates[0]

	propsValue, diag := types.MapValueFrom(ctx, types.StringType, volume.Properties)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	data.Name = types.StringValue(volume.Name)
	data.ID = types.StringValue(volume.SerialNumber)
	data.SerialNumber = types.StringValue(volume.SerialNumber)
	data.DurableID = types.StringValue(volume.DurableID)
	data.WWID = types.StringValue(volume.SerialNumber)
	if volume.WWN != "" {
		data.SCSIWWN = types.StringValue(volume.WWN)
	} else {
		data.SCSIWWN = types.StringNull()
	}
	data.Pool = types.StringValue(volume.PoolName)
	data.VDisk = types.StringValue(volume.VDiskName)
	data.Size = types.StringValue(volume.Size)
	data.Properties = propsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
