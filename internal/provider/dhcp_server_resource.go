// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
	"github.com/davecgh/go-spew/spew"
	"github.com/gofrs/uuid/v5"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	dhcpSrvsDir        = "dhcp_servers"
	dhcpConfigTemplate = `# CoreDHCP config for simple DHCP v4 server for a specific interface.
server4:
  listen:
    - "%{{ .Interface }}:67"
  plugins:
    - lease_time: 3600s
    - server_id: {{ .ServerID }} 
    - dns: {{ .NameServer }} 
    - router: {{ .Router }} 
    - netmask: {{ .Netmask }} 
    - range: {{ .LeasesFile }} {{ .PoolStart }} {{ .PoolEnd }} 180s
`
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &DHCPServer{}
	_ resource.ResourceWithImportState = &DHCPServer{}
)

func NewDHCPServer() resource.Resource {
	return &DHCPServer{}
}

// DHCPServer defines the resource implementation.
type DHCPServer struct {
	providerConf *ZedAmigoProviderConfig
}

// DHCPServerModel describes the resource data model.
type DHCPServerModel struct {
	ID         types.String `tfsdk:"id"`
	Interface  types.String `tfsdk:"interface"`
	ServerID   types.String `tfsdk:"server_id"`
	NameServer types.String `tfsdk:"nameserver"`
	Router     types.String `tfsdk:"router"`
	Netmask    types.String `tfsdk:"netmask"`
	PoolStart  types.String `tfsdk:"pool_start"`
	PoolEnd    types.String `tfsdk:"pool_end"`
	LeasesFile types.String `tfsdk:"leases_file"`
	ConfigFile types.String `tfsdk:"config_file"`
	PIDFile    types.String `tfsdk:"pid_file"`
}

func (r *DHCPServer) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, dhcpSrvsDir, id)
}

func (r *DHCPServer) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_server"
}

func (r *DHCPServer) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Simple DHCP v4 server for a specific interface",
		MarkdownDescription: undent.Md(`Create and manage a DHCP v4 server instance with a simple configuration
		that listens on a specific interface. Uses an embedded instance of CoreDHCP (https://github.com/coredhcp/coredhcp).
		NOTE: If the host has a firewall configuration that might drop incoming UDP port 67 packets. Double check that.
		This resource DOES NOT manage the host firewall configuration.`),

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "DHCP server resource identifier",
				MarkdownDescription: "DHCP server resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"interface": schema.StringAttribute{
				Description: "Interface on which to run this DHCP v4 server instance",
				Optional:    false,
				Required:    true,
			},
			"server_id": schema.StringAttribute{
				Description: "IPv4 address representing the DHCP server ID",
				Optional:    false,
				Required:    true,
			},
			"nameserver": schema.StringAttribute{
				Description: "Nameserver (DNS) IPv4 address",
				MarkdownDescription: undent.Md(`IPv4 address which will be used as the value for the nameserver/DNS option in the DHCP offer.
				If a fully working setup is needed then this must be an existing & working DNS resolver.
				This resource DOES NOT provide DNS resolving.`),
				Optional: false,
				Required: true,
			},
			"router": schema.StringAttribute{
				Description: "Router (gateway) IPv4 address",
				MarkdownDescription: undent.Md(`IPv4 address which will be used as the value for the router option in the DHCP offer.
				If a fully working setup is needed then the host must be configured to route (do NAT, etc.) correctly.
				This resource DOES NOT configure the host.`),
				Optional: false,
				Required: true,
			},
			"netmask": schema.StringAttribute{
				Description: "Netmask for the DHCP offers",
				Optional:    false,
				Required:    true,
			},
			"pool_start": schema.StringAttribute{
				Description: "DHCP v4 pool first IPv4 address for dynamic allocation",
				Optional:    false,
				Required:    true,
			},
			"pool_end": schema.StringAttribute{
				Description: "DHCP v4 pool last IPv4 address for dynamic allocation",
				Optional:    false,
				Required:    true,
			},
			"leases_file": schema.StringAttribute{
				Computed:    true,
				Description: "The sqlite3 leases file used by this instance of CoreDHCP",
			},
			"config_file": schema.StringAttribute{
				Computed:    true,
				Description: "The auto-generated CoreDHCP configuration file",
			},
			"pid_file": schema.StringAttribute{
				Computed:    true,
				Description: "Process ID file",
			},
		},
	}
}

func (r *DHCPServer) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	traceData := map[string]any{"providerConf": spew.Sprint(r.providerConf)}
	tflog.Trace(ctx, "DHCP server resource configure debugging", traceData)
}

func (r *DHCPServer) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DHCPServerModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	tmpl, err := template.New("config").Parse(dhcpConfigTemplate)
	if err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Unable to parse config template: %s", err))
		return
	}

	leasesPath := filepath.Join(d, "leases.sqlite3")
	confPath := filepath.Join(d, "config.yaml")
	confFile, err := os.Create(confPath)
	if err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Can't create file '%s': %s", confPath, err))
		return
	}
	defer confFile.Close()
	data.ConfigFile = types.StringValue(confPath)
	data.LeasesFile = types.StringValue(leasesPath)

	// If we don't do this mapping and try to directly pass `data` to
	// template.Execute then that will call field.String() which returns
	// the value double-quoted.
	td := struct {
		Interface  string
		ServerID   string
		NameServer string
		Router     string
		Netmask    string
		PoolStart  string
		PoolEnd    string
		LeasesFile string
	}{
		Interface:  data.Interface.ValueString(),
		ServerID:   data.ServerID.ValueString(),
		NameServer: data.NameServer.ValueString(),
		Router:     data.Router.ValueString(),
		Netmask:    data.Netmask.ValueString(),
		PoolStart:  data.PoolStart.ValueString(),
		PoolEnd:    data.PoolEnd.ValueString(),
		LeasesFile: data.LeasesFile.ValueString(),
	}
	if err := tmpl.Execute(confFile, td); err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Error",
			fmt.Sprintf("Can't write templated config file '%s': %s", confPath, err))
		return
	}

	pidFile := filepath.Join(d, "pid")
	data.PIDFile = types.StringValue(pidFile)

	srvCmd := os.Args[0]
	srvArgs := []string{}
	if r.providerConf.UseSudo {
		srvCmd = r.providerConf.Sudo
		srvArgs = []string{os.Args[0]}
	}
	moreArgs := []string{"-pid-file", pidFile, "-dhcp-server", "-ds.config", data.ConfigFile.ValueString()}
	if res, err := cmd.RunDetached(d, srvCmd, append(srvArgs, moreArgs...)...); err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			"Failed to run socket tailer")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	// Read the DHCPServer current state.
	if diags, err := r.readDHCPServer(d, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read DHCPServer state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	tflog.Trace(ctx, "DHCPServer Resource created succesfully")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPServer) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DHCPServerModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Read the DHCPServer current state.
	if diags, err := r.readDHCPServer(d, &data); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Failed to read DHCPServer state", err.Error())
		resp.Diagnostics.Append(diags...)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPServer) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DHCPServerModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: What to do here ?

	resp.Diagnostics.AddError("DHCPServer Resource Update Error", "Update is not supported.")
}

func (r *DHCPServer) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DHCPServerModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("DHCPServer Resource Delete Error",
			fmt.Sprintf("Can't delete DHCPServer resource directory: %v", err))
		return
	}
}

func (r *DHCPServer) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *DHCPServer) readDHCPServer(resPath string, model *DHCPServerModel) (diag.Diagnostics, error) {
	return nil, nil
}
