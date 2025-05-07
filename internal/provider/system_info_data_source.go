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

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &SystemInfo{}

func NewSystemInfoDataSource() datasource.DataSource {
	return &SystemInfo{}
}

// SystemInfo defines the data source implementation.
type SystemInfo struct {
	providerConf *ZedAmigoProviderConfig
}

// SystemInfoModel describes the data source data model.
type SystemInfoModel struct {
	ID   types.String `tfsdk:"id"`
	CPUs types.Int32  `tfsdk:"cpus"`
	// MemTotalBytes is the total system memory in bytes.
	MemTotalBytes types.Int64 `tfsdk:"mem_total_bytes"`
	// MemUsed used system memory in bytes.
	MemUsedBytes   types.Int64   `tfsdk:"mem_used_bytes"`
	MemUsedPercent types.Float64 `tfsdk:"mem_used_percent"`
}

func (d *SystemInfo) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_system_info"
}

func (d *SystemInfo) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The system info data source returns the information about number of CPUs and system memory, currently the total and used values.",

		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The system info data source returns the information about number of CPUs and system memory, currently the total and used values.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "System info data source identifier",
				MarkdownDescription: "System info data source identifier",
				Computed:            true,
			},
			"cpus": schema.Int32Attribute{
				Description:         "Total number of logical CPUs of the system",
				MarkdownDescription: "Total number of logical CPUs of the system",
				Computed:            true,
			},
			"mem_total_bytes": schema.Int64Attribute{
				Description:         "Total system memory in bytes",
				MarkdownDescription: "Total system memory in bytes",
				Computed:            true,
			},
			"mem_used_bytes": schema.Int64Attribute{
				Description:         "Used system memory in bytes",
				MarkdownDescription: "Used system memory in bytes",
				Computed:            true,
			},
			"mem_used_percent": schema.Float64Attribute{
				Description:         "Used system memory as a percentage of total",
				MarkdownDescription: "Used system memory as a percentage of total",
				Computed:            true,
			},
		},
	}
}

func (d *SystemInfo) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SystemInfo) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data SystemInfoModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	h, err := os.Hostname()
	if err != nil {
		resp.Diagnostics.AddError("System Info Data Source Error",
			fmt.Sprintf("Unable to get system hostname, got error: %s", err))
		return
	}
	if len(h) < 1 {
		resp.Diagnostics.AddError("System Info Data Source Error",
			fmt.Sprintf("Unable to get system hostname, got empty string"))
		return
	}
	data.ID = types.StringValue(h)

	x, err := cpu.Counts(true)
	if err != nil {
		resp.Diagnostics.AddError("System Info Data Source Error",
			fmt.Sprintf("Unable to get system CPUs info, got error: %s", err))
		return
	}
	data.CPUs = types.Int32Value(int32(x))

	v, err := mem.VirtualMemory()
	if err != nil {
		resp.Diagnostics.AddError("System Info Data Source Error",
			fmt.Sprintf("Unable to get system memory info, got error: %s", err))
		return
	}

	// TODO: We should check that the conversions DO NOT overflow.
	data.MemTotalBytes = types.Int64Value(int64(v.Total))
	data.MemUsedBytes = types.Int64Value(int64(v.Used))
	data.MemUsedPercent = types.Float64Value(v.UsedPercent)

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read system info data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
