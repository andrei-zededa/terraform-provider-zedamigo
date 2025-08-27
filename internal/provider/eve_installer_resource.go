// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/gofrs/uuid/v5"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	eveInstallersDir = "eve_installers"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &EveInstaller{}
	_ resource.ResourceWithImportState = &EveInstaller{}
)

func NewEveInstaller() resource.Resource {
	return &EveInstaller{}
}

// EveInstaller defines the resource implementation.
type EveInstaller struct {
	providerConf *ZedAmigoProviderConfig
}

// EveInstallerModel describes the resource data model.
type EveInstallerModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Tag      types.String `tfsdk:"tag"`
	Cluster  types.String `tfsdk:"cluster"`
	TLSCA    types.String `tfsdk:"tls_ca"`
	ObjCA    types.String `tfsdk:"object_signing_ca"`
	Hosts    types.String `tfsdk:"additional_hosts"`
	SSHKey   types.String `tfsdk:"authorized_keys"`
	GrubCfg  types.String `tfsdk:"grub_cfg"`
	DPCOver  types.String `tfsdk:"device_port_config_override"`
	Filename types.String `tfsdk:"filename"`
}

func (r *EveInstaller) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, eveInstallersDir, id)
}

func (r *EveInstaller) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_eve_installer"
}

func (r *EveInstaller) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "EVE-OS Installer",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "EVE-OS Installer",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "EVE-OS Installer identifier",
				MarkdownDescription: "EVE-OS Installer identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "EVE-OS Installer name (also the file-name)",
				MarkdownDescription: "EVE-OS Installer name (also the file-name)",
				Optional:            false,
				Required:            true,
			},
			"tag": schema.StringAttribute{
				Description:         "lfedge/eve container image tag to use for generating the EVE-OS Installer",
				MarkdownDescription: "lfedge/eve container image tag to use for generating the EVE-OS Installer",
				Optional:            false,
				Required:            true,
			},
			"cluster": schema.StringAttribute{
				Description:         "Zedcloud cluster hostname",
				MarkdownDescription: "Zedcloud cluster hostname",
				Optional:            false,
				Required:            true,
			},
			"tls_ca": schema.StringAttribute{
				Description:         "The CA that signed the TLS server certificate (PEM), see `v2tlsbaseroot-certificates.pem` in https://github.com/lf-edge/eve/blob/master/docs/REGISTRATION.md",
				MarkdownDescription: "The CA that signed the TLS server certificate (PEM), see `v2tlsbaseroot-certificates.pem` in https://github.com/lf-edge/eve/blob/master/docs/REGISTRATION.md",
				Optional:            true,
				Required:            false,
			},
			"object_signing_ca": schema.StringAttribute{
				Description:         "The CA that signed the internal controller certificate, see `root-certificate.pem` in https://github.com/lf-edge/eve/blob/master/docs/REGISTRATION.md",
				MarkdownDescription: "The CA that signed the internal controller certificate, see `root-certificate.pem` in https://github.com/lf-edge/eve/blob/master/docs/REGISTRATION.md",
				Optional:            true,
				Required:            false,
			},
			"additional_hosts": schema.StringAttribute{
				Description:         "Additional entries that will be appended to /etc/hosts of the installed edge-node",
				MarkdownDescription: "Additional entries that will be appended to /etc/hosts of the installed edge-node",
				Optional:            true,
				Required:            false,
			},
			"authorized_keys": schema.StringAttribute{
				Description:         "SSH public key (unsure if multiple are supported) that is configured on the installed edge-node for SSH prior to onboarding",
				MarkdownDescription: "SSH public key (unsure if multiple are supported) that is configured on the installed edge-node for SSH prior to onboarding",
				Optional:            true,
				Required:            false,
			},
			"grub_cfg": schema.StringAttribute{
				Description:         "grub.cfg",
				MarkdownDescription: "grub.cfg",
				Optional:            true,
				Required:            false,
			},
			"device_port_config_override": schema.StringAttribute{
				Description:         "DevicePortConfig/override.json",
				MarkdownDescription: "DevicePortConfig/override.json",
				Optional:            true,
				Required:            false,
			},
			"filename": schema.StringAttribute{
				Description:         "Full path/filename of the resulting installer file",
				MarkdownDescription: "Full path/filename of the resulting installer file",
				Computed:            true,
			},
		},
	}
}

func (r *EveInstaller) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	conf, ok := req.ProviderData.(*ZedAmigoProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected string, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.providerConf = conf
}

func (r *EveInstaller) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EveInstallerModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}
	if err := os.MkdirAll(filepath.Join(d, "config"), 0o700); err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := os.WriteFile(filepath.Join(d, "config", "server"), []byte(data.Cluster.ValueString()), 0o600); err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
			fmt.Sprintf("Can't write /config/server file: %s", err))
		return
	}
	tlsCA := data.TLSCA.ValueString()
	if len(tlsCA) > 0 {
		if err := os.WriteFile(filepath.Join(d, "config", "v2tlsbaseroot-certificates.pem"), []byte(tlsCA), 0o600); err != nil {
			resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
				fmt.Sprintf("Can't write /config/v2tlsbaseroot-certificates.pem file: %s", err))
			return
		}
	}
	objCA := data.ObjCA.ValueString()
	if len(objCA) > 0 {
		if err := os.WriteFile(filepath.Join(d, "config", "root-certificate.pem"), []byte(objCA), 0o600); err != nil {
			resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
				fmt.Sprintf("Can't write /config/root-certificate.pem file: %s", err))
			return
		}
	}
	hosts := data.Hosts.ValueString()
	if len(hosts) > 0 {
		if err := os.WriteFile(filepath.Join(d, "config", "hosts"), []byte(hosts), 0o600); err != nil {
			resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
				fmt.Sprintf("Can't write /config/hosts file: %s", err))
			return
		}
	}
	sshKey := data.SSHKey.ValueString()
	if len(sshKey) > 0 {
		// Ensure that the result file has at least one newline, otherwise
		// it is not processed correctly.
		b := []byte(sshKey + "\n")
		if err := os.WriteFile(filepath.Join(d, "config", "authorized_keys"), b, 0o600); err != nil {
			resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
				fmt.Sprintf("Can't write /config/authorized_keys file: %s", err))
			return
		}
	}
	grubCfg := data.GrubCfg.ValueString()
	if len(grubCfg) > 0 {
		// Ensure that the result file has at least one newline.
		b := []byte(grubCfg + "\n")
		if err := os.WriteFile(filepath.Join(d, "config", "grub.cfg"), b, 0o600); err != nil {
			resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
				fmt.Sprintf("Can't write /config/grub.cfg file: %s", err))
			return
		}
	}
	dpcOver := data.DPCOver.ValueString()
	if len(dpcOver) > 0 {
		if err := os.MkdirAll(filepath.Join(d, "config", "DevicePortConfig"), 0o700); err != nil {
			resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
				fmt.Sprintf("Unable to create resource specific directory: %s", err))
			return
		}
		// Ensure that the result file has at least one newline.
		b := []byte(dpcOver + "\n")
		if err := os.WriteFile(filepath.Join(d, "config", "DevicePortConfig", "override.json"), b, 0o600); err != nil {
			resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
				fmt.Sprintf("Can't write /config/DevicePortConfig/override.json file: %s", err))
			return
		}
	}
	if err := os.MkdirAll(filepath.Join(d, "out"), 0o700); err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	res, err := cmd.Run(d, r.providerConf.Docker, "run", "--rm",
		"-v", fmt.Sprintf("%s:/in", filepath.Join(d, "config")),
		"-v", fmt.Sprintf("%s:/out", filepath.Join(d, "out")),
		fmt.Sprintf("docker.io/lfedge/eve:%s", data.Tag.ValueString()),
		"-f", "raw", "installer_iso")
	if err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
			"Unable to create a new installer iso")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	i := fmt.Sprintf("%s.custom_installer.iso", filepath.Join(d, data.Name.ValueString()))
	if err := os.Rename(filepath.Join(d, "out", "installer.iso"), i); err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Error",
			fmt.Sprintf("Unable to move installer file: %v", err))
		return
	}

	tflog.Trace(ctx, "EVE-OS Installer Resource created succesfully")

	j, err := readEveInstaller(r.providerConf, d, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Read Error",
			fmt.Sprintf("Can't read back installer iso resource: %v", err))
		return
	}
	data.Filename = types.StringValue(j)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func readEveInstaller(_ *ZedAmigoProviderConfig, path, name string) (string, error) {
	i := fmt.Sprintf("%s.custom_installer.iso", filepath.Join(path, name))

	return i, nil
}

func (r *EveInstaller) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EveInstallerModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	i, err := readEveInstaller(r.providerConf, r.getResourceDir(data.ID.ValueString()),
		data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Read Error",
			fmt.Sprintf("Can't read back installer iso resource: %v", err))
		return
	}
	data.Filename = types.StringValue(i)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EveInstaller) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EveInstallerModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddError("EVE-OS Installer Resource Update Error", "Update is not supported.")
}

func (r *EveInstaller) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EveInstallerModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("EVE-OS Installer Resource Delete Error",
			fmt.Sprintf("Can't delete resource directory: %v", err))
		return
	}
}

func (r *EveInstaller) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
