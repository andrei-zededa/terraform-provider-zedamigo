// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &EveInstallerDS{}

func NewEveInstallerDataSource() datasource.DataSource {
	return &EveInstallerDS{}
}

// EveInstallerDS defines the data source implementation.
type EveInstallerDS struct {
	providerConf *ZedAmigoProviderConfig
}

// EveInstallerDSModel describes the data source data model.
type EveInstallerDSModel struct {
	ID       types.String `tfsdk:"id"`
	Filename types.String `tfsdk:"filename"`
}

func (d *EveInstallerDS) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_eve_installer"
}

func (d *EveInstallerDS) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "EVE-OS Installer data source",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "EVE-OS Installer data source",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "EVE-OS Installer data source identifier",
				MarkdownDescription: "EVE-OS Installer data source identifier",
				Computed:            true,
			},
			"filename": schema.StringAttribute{
				Description:         "Full path/filename of the installer file",
				MarkdownDescription: "Full path/filename of the installer file",
				Optional:            false,
				Required:            true,
			},
		},
	}
}

func (d *EveInstallerDS) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func sum2ID(s string) string {
	hasher := sha256.New()

	hasher.Write([]byte(s))
	hashBytes := hasher.Sum(nil)

	return base64.RawStdEncoding.EncodeToString(hashBytes)
}

func (d *EveInstallerDS) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data EveInstallerDSModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(sum2ID(data.Filename.ValueString()))

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read EVE installer data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
