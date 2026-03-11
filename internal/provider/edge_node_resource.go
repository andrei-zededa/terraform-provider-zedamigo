// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/hypervisor"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
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
	edgeNodesDir = "edge_nodes"
	nic0Fmt      = "user,id=usernet0,ipv6=off,hostfwd=tcp::%d-:22,hostfwd=tcp::%d-:10022,hostfwd=tcp::%d-:10080,model=virtio"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &EdgeNode{}
	_ resource.ResourceWithImportState = &EdgeNode{}
)

func NewEdgeNode() resource.Resource {
	return &EdgeNode{}
}

// EdgeNode defines the resource implementation.
type EdgeNode struct {
	providerConf *ZedAmigoProviderConfig
}

// EdgeNodeModel describes the resource data model.
type EdgeNodeModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Mem              types.String `tfsdk:"mem"`
	CPUs             types.Int64  `tfsdk:"cpus"`
	SerialNo         types.String `tfsdk:"serial_no"`
	Nic0             types.String `tfsdk:"nic0"`
	SerialPortServer types.Bool   `tfsdk:"serial_port_server"`
	SerialPortSocket types.String `tfsdk:"serial_port_socket"`
	DiskImgBase      types.String `tfsdk:"disk_image_base"`
	Disk1ImgBase     types.String `tfsdk:"disk_1_image_base"`
	DiskSizeMB       types.Int64  `tfsdk:"disk_size_mb"`
	DriveIf          types.String `tfsdk:"drive_if"`
	SwTPMSock        types.String `tfsdk:"swtpm_socket"`
	DiskImg          types.String `tfsdk:"disk_image"`
	Disk1Img         types.String `tfsdk:"disk_1_image"`
	SerialConsoleLog types.String `tfsdk:"serial_console_log"`
	OvmfVarsSrc      types.String `tfsdk:"ovmf_vars_src"`
	OvmfVars         types.String `tfsdk:"ovmf_vars"`
	QmpSocket        types.String `tfsdk:"qmp_socket"`
	VMRunning        types.Bool   `tfsdk:"vm_running"`
	SSHPort          types.Int32  `tfsdk:"ssh_port"`
	ExtraArgs        types.List   `tfsdk:"extra_qemu_args"`
	CPUPins          types.List   `tfsdk:"cpu_pins"`
	UseGvproxy       types.Bool   `tfsdk:"use_gvproxy"`
}

func (r *EdgeNode) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, edgeNodesDir, id)
}

func (r *EdgeNode) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_edge_node"
}

func (r *EdgeNode) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Edge Node / VM",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Edge Node / VM in the general case",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Edge Node (or VM) identifier",
				MarkdownDescription: "Edge Node (or VM) identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "Edge Node (or VM) name",
				MarkdownDescription: "Edge Node (or VM) name",
				Optional:            true,
				Required:            false,
			},
			"mem": schema.StringAttribute{
				Description:         "Amount of memory that the VM running the edge node will have. Default: 4G. Valid options: `4096`, `4096M`, `4G`.",
				MarkdownDescription: "Amount of memory that the VM running the edge node will have. Default: 4G. Valid options: `4096`, `4096M`, `4G`.",
				Optional:            true,
				Required:            false,
			},
			"cpus": schema.Int64Attribute{
				Description:         "Number of CPUs that the VM running the edge node will have. Default: 4. See the QEMU `-smp` option.",
				MarkdownDescription: "Number of CPUs that the VM running the edge node will have. Default: 4. See the QEMU `-smp` option.",
				Optional:            true,
				Required:            false,
			},
			"serial_no": schema.StringAttribute{
				Description:         "Edge Node (or VM) serial number",
				MarkdownDescription: "Edge Node (or VM) serial number",
				Optional:            false,
				Required:            true,
			},
			"nic0": schema.StringAttribute{
				Description: "QEMU `-nic` options for the first (#0) NIC of the edge node VM. Default: `" + nic0Fmt + "`",
				MarkdownDescription: undent.Md(`
				By default the first NIC (#0) of the edge node VM will use QEMU "user mode networking", which means that QEMU
				will run an internal DHCP server and internal NAT/router to provide the VM with the same connectivity that
				the QEMU process has on the host. This is convenient because it allows the VM to have external (external to the
				host, possibly full Internet access) connectivity without having to configure any firewall or NAT rules on the
				host. However this also means that the IPv4/v6 address allocated to the VM is not directly accesible from the
				host and port forwards need to be configured. By default a random port is allocated and that is setup as a port
				forward to the VM port 22. Two addtional ports forwards are set up: $random + 1 to 10022 and $random + 2 to 10080
				of the VM. These might be useful if the an edge-app-instance is configured with an inbound rule that maps ports
				10022 or 10080 of the edge node (EVE-OS) to ports of the edge-app-instance. Note that in this case to access an
				edge-app-instance from the host 2 levels of port forwards are involved.`),
				Optional: true,
				Required: false,
			},
			"serial_port_server": schema.BoolAttribute{
				Description: `Configure the edge-node serial port as a UNIX socket server. ` +
					`On Linux (QEMU), when true the serial port is exposed as a UNIX socket server and a socket tailer process ` +
					`is launched to also log serial output to a file. When false, serial output is written directly to a log file. ` +
					`On macOS (vfkit), this setting is ignored — serial output is always written directly to a log file because ` +
					`vfkit does not support socket-based serial devices. The serial_console_log attribute will always be populated.`,
				MarkdownDescription: "Configure the edge-node serial port as a UNIX socket server.\n\n" +
					"**Linux (QEMU):** When `true`, the serial port is exposed as a UNIX socket server and a socket tailer " +
					"process is launched to also log serial output to a file. When `false`, serial output is written directly " +
					"to a log file.\n\n" +
					"**macOS (vfkit):** This setting is ignored — serial output is always written directly to a log file because " +
					"vfkit does not support socket-based serial devices. The `serial_console_log` attribute will always be populated.",
				Optional: true,
				Required: false,
			},
			"serial_port_socket": schema.StringAttribute{
				Description: `File path of the UNIX socket for the serial port server. ` +
					`On Linux (QEMU), this is populated when serial_port_server is true. ` +
					`On macOS (vfkit), this is always empty because vfkit does not support socket-based serial devices.`,
				MarkdownDescription: "File path of the UNIX socket for the serial port server.\n\n" +
					"**Linux (QEMU):** Populated when `serial_port_server` is `true`.\n\n" +
					"**macOS (vfkit):** Always empty because vfkit does not support socket-based serial devices.",
				Computed: true,
			},
			"serial_console_log": schema.StringAttribute{
				Description: `Edge Node log file of serial console output. ` +
					`On Linux (QEMU), populated when serial_port_server is false, or when serial_port_server is true (the socket tailer writes output to this file). ` +
					`On macOS (vfkit), always populated — serial output is written directly to this file regardless of the serial_port_server setting.`,
				MarkdownDescription: "Edge Node log file of serial console output.\n\n" +
					"**Linux (QEMU):** Populated when `serial_port_server` is `false`, or when `serial_port_server` is `true` " +
					"(the socket tailer also writes output to this file).\n\n" +
					"**macOS (vfkit):** Always populated — serial output is written directly to this file regardless of the " +
					"`serial_port_server` setting.",
				Computed: true,
			},
			"disk_image_base": schema.StringAttribute{
				Description:         "Disk image base from which the actual disk image used for this node will be created (qemu-img backing file)",
				MarkdownDescription: "Disk image base from which the actual disk image used for this node will be created (qemu-img backing file)",
				Optional:            false,
				Required:            true,
			},
			"disk_1_image_base": schema.StringAttribute{
				Description:         "Disk image base from which the 2nd disk actual disk image used for this node will be created (qemu-img backing file)",
				MarkdownDescription: "Disk image base from which the 2nd disk actual disk image used for this node will be created (qemu-img backing file)",
				Optional:            true,
				Required:            false,
			},
			"disk_size_mb": schema.Int64Attribute{
				Description:         "Disk image size in MB (megabytes, old-style power of 2). If not specified then the size of the base image will be preserved.",
				MarkdownDescription: "Disk image size in MB (megabytes, old-style power of 2). If not specified then the size of the base image will be preserved.",
				Optional:            true,
				Required:            false,
			},
			"drive_if": schema.StringAttribute{
				Description:         "The value of the interface (if) option for the QEMU `-drive` flag. This defines how the disk is presented to the VM. The default value is empty which for current versions of QEMU translates to `ide` which is a good option for running EVE-OS. Other valid options: ide, scsi, sd, mtd, floppy, pflash, virtio, none. See also the help for QEMU `-drive`.",
				MarkdownDescription: "The value of the interface (if) option for the QEMU `-drive` flag. This defines how the disk is presented to the VM. The default value is empty which for current versions of QEMU translates to `ide` which is a good option for running EVE-OS. Other valid options: ide, scsi, sd, mtd, floppy, pflash, virtio, none. See also the help for QEMU `-drive`.",
				Optional:            true,
				Required:            false,
			},
			"swtpm_socket": schema.StringAttribute{
				Description:         "swtpm process unix socket",
				MarkdownDescription: "swtpm process unix socket",
				Optional:            true,
				Required:            false,
			},
			"disk_image": schema.StringAttribute{
				Description:         "Edge Node disk image",
				MarkdownDescription: "Edge Node disk image",
				Computed:            true,
			},
			"disk_1_image": schema.StringAttribute{
				Description:         "Edge Node 2nd disk disk image",
				MarkdownDescription: "Edge Node 2nd disk disk image",
				Computed:            true,
			},
			"ovmf_vars_src": schema.StringAttribute{
				Description:         "UEFI OVMF vars source file (likely from the corresponding installed edge node)",
				MarkdownDescription: "UEFI OVMF vars source file (likely from the corresponding installed edge node)",
				Optional:            true,
				Required:            false,
			},
			"ovmf_vars": schema.StringAttribute{
				Description:         "UEFI OVMF vars file specific for this edge node",
				MarkdownDescription: "UEFI OVMF vars file specific for this edge node",
				Computed:            true,
			},
			"qmp_socket": schema.StringAttribute{
				Description:         "UNIX socket for QEMU QMP for this edge node VM",
				MarkdownDescription: "UNIX socket for QEMU QMP for this edge node VM",
				Computed:            true,
			},
			"vm_running": schema.BoolAttribute{
				Description:         "Running state of the QEMU VM for this edge node",
				MarkdownDescription: "Running state of the QEMU VM for this edge node",
				Computed:            true,
			},
			"ssh_port": schema.Int32Attribute{
				Description:         "Randomly selected port on localhost on which the EVE-OS TCP port 22 can be accessed",
				MarkdownDescription: "Randomly selected port on localhost on which the EVE-OS TCP port 22 can be accessed",
				Computed:            true,
			},
			"extra_qemu_args": schema.ListAttribute{
				Description: "Extra CLI arguments for the QEMU command used to start the edge node VM. Passed verbatim to QEMU.",
				MarkdownDescription: undent.Md(`
				Extra CLI arguments for the QEMU command used to start the edge node VM. Passed verbatim to QEMU.
				For example this can be used to create additional NICs for the edge node VM:
				      extra_qemu_args = [
				        "-nic", "tap,id=vmnet1,ifname=${zedamigo_tap.TAP_101.name},script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:01",
				        "-nic", "tap,id=vmnet2,ifname=${zedamigo_tap.TAP_102.name},script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:02",
				        "-nic", "tap,id=vmnet3,ifname=${zedamigo_tap.TAP_103.name},script=no,downscript=no,model=e1000,mac=8c:84:74:11:01:03",
    				      ]
				Considering that the respective TAP interfaces are created with the |zedamigo_tap| resource.`),
				ElementType: types.StringType,
				Optional:    true,
				Required:    false,
			},
			"use_gvproxy": schema.BoolAttribute{
				Description:         "Use embedded gvproxy for networking instead of QEMU SLIRP. Requires QEMU 7.2+. Default: `false` on Linux but `true` on MacOS since there we have to use it since we use `vfkit` instead of `qemu`.",
				MarkdownDescription: "Use embedded gvproxy for networking instead of QEMU SLIRP. Requires QEMU 7.2+. Default: `false` on Linux but `true` on MacOS since there we have to use it since we use `vfkit` instead of `qemu`.",
				Optional:            true,
			},
			"cpu_pins": schema.ListAttribute{
				Description: "List of host CPU IDs to pin VM vCPUs to. Must match CPU count.",
				MarkdownDescription: undent.Md(`
				List of host CPU core IDs to pin the VM's vCPUs to. When specified,
				the length must equal the number of CPUs. For example, with 4 CPUs
				and cpu_pins = [0, 2, 4, 6], vCPU 0 pins to host core 0, vCPU 1 to
				core 2, etc. Requires QEMU to start with debug-threads enabled.`),
				ElementType: types.Int64Type,
				Optional:    true,
				Required:    false,
			},
		},
	}
}

func (r *EdgeNode) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *EdgeNode) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data EdgeNodeModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	u, err := uuid.NewV4()
	if err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			fmt.Sprintf("Unable to generate a new UUID: %s", err))
		return
	}
	data.ID = types.StringValue(u.String())
	if resp.Diagnostics.HasError() {
		return
	}

	data.SSHPort = types.Int32Value(10000 + int32(rand.Uint32N(55534)))

	d := r.getResourceDir(data.ID.ValueString())
	if err := os.MkdirAll(d, 0o700); err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			fmt.Sprintf("Unable to create resource specific directory: %s", err))
		return
	}
	if err := createTFBackPointer(d); err != nil {
		resp.Diagnostics.AddError("Disk Image Resource Error",
			fmt.Sprintf("Unable to create resource specific file: %s", err))
		return
	}

	// Build VMConfig for the hypervisor.
	nic0 := fmt.Sprintf(nic0Fmt, data.SSHPort.ValueInt32(),
		data.SSHPort.ValueInt32()+1, data.SSHPort.ValueInt32()+2)
	if !data.Nic0.IsNull() && len(data.Nic0.ValueString()) > 0 {
		nic0 = data.Nic0.ValueString()
	}

	var extraArgs []string
	if !data.ExtraArgs.IsNull() {
		diags := data.ExtraArgs.ElementsAs(ctx, &extraArgs, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	var cpuPins []int64
	if !data.CPUPins.IsNull() && !data.CPUPins.IsUnknown() {
		diags := data.CPUPins.ElementsAs(ctx, &cpuPins, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		cpus := int64(4)
		if !data.CPUs.IsNull() {
			cpus = data.CPUs.ValueInt64()
		}
		if len(cpuPins) != int(cpus) {
			resp.Diagnostics.AddError("CPU Pins Validation Error",
				fmt.Sprintf("cpu_pins length (%d) must match cpus (%d)", len(cpuPins), cpus))
			return
		}
	}

	vmConf := hypervisor.VMConfig{
		Name:           data.Name.ValueString(),
		ID:             data.ID.ValueString(),
		SerialNo:       data.SerialNo.ValueString(),
		ResourceDir:    d,
		MemoryMB:       data.Mem.ValueString(),
		CPUs:           data.CPUs.ValueInt64(),
		DiskImageBase:  data.DiskImgBase.ValueString(),
		Disk1ImageBase: data.Disk1ImgBase.ValueString(),
		DiskSizeMB:     data.DiskSizeMB.ValueInt64(),
		HasDiskSize:    !data.DiskSizeMB.IsNull(),
		DriveIf:        data.DriveIf.ValueString(),
		OVMFVarsSrc:    data.OvmfVarsSrc.ValueString(),
		Nic0:           nic0,
		SSHPort:        data.SSHPort.ValueInt32(),
		SwTPMSocket:    data.SwTPMSock.ValueString(),
		ExtraArgs:      extraArgs,
		CPUPins:        cpuPins,
		UseGvproxy:     !data.UseGvproxy.IsNull() && data.UseGvproxy.ValueBool(),
	}

	// Handle serial console config.
	if data.SerialPortServer.ValueBool() {
		vmConf.SerialToSocket = filepath.Join(d, "serial_port.socket")
	} else {
		vmConf.SerialToFile = filepath.Join(d, "serial_console_run.log")
	}

	// Prepare disks.
	paths, err := r.providerConf.Hypervisor.PrepareDisks(ctx, vmConf)
	if err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			fmt.Sprintf("Failed to prepare disks: %v", err))
		return
	}

	data.DiskImg = types.StringValue(paths.DiskImage)
	data.Disk1Img = types.StringValue(paths.Disk1Image)
	data.OvmfVars = types.StringValue(paths.OVMFVars)
	data.QmpSocket = types.StringValue(fmt.Sprintf("unix:%s,server,nowait", paths.QMPSocket))

	// Set serial output paths.
	if data.SerialPortServer.ValueBool() {
		data.SerialPortSocket = types.StringValue(vmConf.SerialToSocket)
		data.SerialConsoleLog = types.StringValue(filepath.Join(d, "serial_console_run.log"))
		paths.SerialPortSocket = vmConf.SerialToSocket
		paths.SerialConsoleLog = data.SerialConsoleLog.ValueString()
	} else {
		data.SerialPortSocket = types.StringValue("")
		data.SerialConsoleLog = types.StringValue(vmConf.SerialToFile)
		paths.SerialConsoleLog = vmConf.SerialToFile
	}

	// Start VM.
	if err := r.providerConf.Hypervisor.Start(ctx, vmConf, paths); err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			fmt.Sprintf("Failed to start VM: %v", err))
		return
	}

	tflog.Trace(ctx, "Edge Node Resource created succesfully")

	// Launch socket tailer if serial_port_server is true.
	// On MacOS (vfkit), serial output goes directly to a file — no socket tailer needed.
	if data.SerialPortServer.ValueBool() && runtime.GOOS != "darwin" {
		res, err := cmd.RunDetached(d, os.Args[0], []string{"-socket-tailer", "-st.connect", data.SerialPortSocket.ValueString(), "-st.out", data.SerialConsoleLog.ValueString()}...)
		if err != nil {
			resp.Diagnostics.AddError("Edge Node Resource Error",
				"Failed to run socket tailer")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	x, err := r.providerConf.Hypervisor.Status(ctx, d)
	if err != nil {
		resp.Diagnostics.AddWarning("Edge Node Resource Read Error",
			fmt.Sprintf("Can't read VM status: %v", err))
		return
	}
	data.VMRunning = types.BoolValue(x)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeNode) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EdgeNodeModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	x, err := r.providerConf.Hypervisor.Status(ctx, r.getResourceDir(data.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddWarning("Edge Node Resource Read Warning",
			fmt.Sprintf("Treating this as a warning since most likely"+
				" the corresponding VM instance is not running anymore."+
				" Can't read VM status: %v", err))
		data.VMRunning = types.BoolValue(false)
	}

	data.VMRunning = types.BoolValue(x)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *EdgeNode) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data EdgeNodeModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.AddError("Edge Node Resource Update Error", "Update is not supported.")
}

func (r *EdgeNode) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data EdgeNodeModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d := r.getResourceDir(data.ID.ValueString())

	// Try to shutdown the running VM.
	if data.VMRunning.ValueBool() {
		if err := r.providerConf.Hypervisor.Stop(ctx, d); err != nil {
			resp.Diagnostics.AddWarning("Edge Node Resource Delete Warning",
				fmt.Sprintf("Error stopping VM (may already be stopped): %v", err))
		}
	}

	if err := os.RemoveAll(d); err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Delete Error",
			fmt.Sprintf("Can't delete resource directory: %v", err))
		return
	}
}

func (r *EdgeNode) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Alias resource types.

type VMResource struct {
	EdgeNode // Embed the shared implementation.
}

func (r VMResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func NewVM() resource.Resource {
	return &VMResource{}
}

type VirtualMachineResource struct {
	EdgeNode // Embed the shared implementation.
}

func (r VirtualMachineResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_virtual_machine"
}

func NewVirtualMachine() resource.Resource {
	return &VirtualMachineResource{}
}
