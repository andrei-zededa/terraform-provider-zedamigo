// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/shirou/gopsutil/v4/mem"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &SystemMemory{}

func NewSystemMemoryDataSource() datasource.DataSource {
	return &SystemMemory{}
}

// SystemMemory defines the data source implementation.
type SystemMemory struct {
	providerConf *ZedAmigoProviderConfig
}

// SystemMemoryModel describes the data source data model.
type SystemMemoryModel struct {
	ID          types.String  `tfsdk:"id"`
	Total       types.Int64   `tfsdk:"total"`
	Used        types.Int64   `tfsdk:"used"`
	UsedPercent types.Float64 `tfsdk:"used_percent"`
}

func (d *SystemMemory) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_memory"
}

func (d *SystemMemory) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The system memory data source returns the information about system memory, currently the total and used values.",

		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The system memory data source returns the information about system memory, currently the total and used values.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "System memory data source identifier",
				MarkdownDescription: "System memory data source identifier",
				Computed:            true,
			},
			"total": schema.Int64Attribute{
				Description:         "Total system memory in bytes",
				MarkdownDescription: "Total system memory in bytes",
				Computed:            true,
			},
			"used": schema.Int64Attribute{
				Description:         "Used system memory in bytes",
				MarkdownDescription: "Used system memory in bytes",
				Computed:            true,
			},
			"used_percent": schema.Float64Attribute{
				Description:         "Used system memory as a percentage of total",
				MarkdownDescription: "Used system memory as a percentage of total",
				Computed:            true,
			},
		},
	}
}

func (d *SystemMemory) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	conf, ok := req.ProviderData.(*ZedAmigoProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected string, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.providerConf = conf
}

func (d *SystemMemory) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SystemMemoryModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	h, err := os.Hostname()
	if err != nil {
		resp.Diagnostics.AddError("System Memory Data Source Error", fmt.Sprintf("Unable to get system hostname, got error: %s", err))
		return
	}
	if len(h) < 1 {
		resp.Diagnostics.AddError("System Memory Data Source Error", fmt.Sprintf("Unable to get system hostname, got empty string"))
		return
	}
	data.ID = types.StringValue(h)

	v, err := mem.VirtualMemory()
	if err != nil {
		resp.Diagnostics.AddError("System Memory Data Source Error", fmt.Sprintf("Unable to get system memory info, got error: %s", err))
		return
	}

	// TODO: We should check that the conversions DO NOT overflow.
	data.Total = types.Int64Value(int64(v.Total))
	data.Used = types.Int64Value(int64(v.Used))
	data.UsedPercent = types.Float64Value(v.UsedPercent)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read system memory data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
