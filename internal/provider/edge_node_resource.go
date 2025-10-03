// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/qmp"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/undent"
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
	nic0Fmt      = "user,id=usernet0,hostfwd=tcp::%d-:22,hostfwd=tcp::%d-:10022,hostfwd=tcp::%d-:10080,model=virtio"
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
	CPUs             types.String `tfsdk:"cpus"`
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
			"cpus": schema.StringAttribute{
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
				Description:         "Configure the edge-node serial port as a telnet server; if false then serial port output is logged to a file",
				MarkdownDescription: "Configure the edge-node serial port as a telnet server; if false then serial port output is logged to a file",
				Optional:            true,
				Required:            false,
			},
			"serial_port_socket": schema.StringAttribute{
				Description:         "If serial_port_server is true then this will contain the file path of the UNIX socket for the serial port server",
				MarkdownDescription: "If serial_port_server is true then this will contain the file path of the UNIX socket for the serial port server",
				Computed:            true,
			},
			"serial_console_log": schema.StringAttribute{
				Description:         "Edge Node log file of serial console output if serial_port_server is false",
				MarkdownDescription: "Edge Node log file of serial console output if serial_port_server is false",
				Computed:            true,
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

	// Read Terraform plan data into the model/
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sid := shortID()
	data.ID = types.StringValue(sid)
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
	data.DiskImg = types.StringValue(filepath.Join(d, "disk0.disk_img.qcow2"))
	qemuImgArgs := []string{
		"create", "-f", "qcow2",
		"-b", data.DiskImgBase.ValueString(), "-F", "qcow2",
		data.DiskImg.ValueString(),
	}
	if !data.DiskSizeMB.IsNull() {
		qemuImgArgs = append(qemuImgArgs, fmt.Sprintf("%sM", data.DiskSizeMB.String()))
	}
	res, err := cmd.Run(d, r.providerConf.QemuImg, qemuImgArgs...)
	if err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			"Unable to create a new disk image")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}
	data.Disk1Img = types.StringValue("")
	if !data.Disk1ImgBase.IsNull() && len(data.Disk1ImgBase.ValueString()) > 0 {
		data.Disk1Img = types.StringValue(filepath.Join(d, "disk1.disk_img.qcow2"))
		qemuImgArgs := []string{
			"create", "-f", "qcow2",
			"-b", data.Disk1ImgBase.ValueString(), "-F", "qcow2",
			data.Disk1Img.ValueString(),
		}
		if !data.DiskSizeMB.IsNull() {
			qemuImgArgs = append(qemuImgArgs, fmt.Sprintf("%sM", data.DiskSizeMB.String()))
		}
		res, err := cmd.Run(d, r.providerConf.QemuImg, qemuImgArgs...)
		if err != nil {
			resp.Diagnostics.AddError("Edge Node Resource Error",
				"Unable to create a new disk image")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}
	}

	varsFile := filepath.Join(d, "UEFI_OVMF_VARS.bin")
	ovSrc := r.providerConf.BaseOVMFVars
	if !data.OvmfVarsSrc.IsNull() {
		ovSrc = data.OvmfVarsSrc.ValueString()
	}
	if _, err := cmd.CopyFile(ovSrc, varsFile); err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			fmt.Sprintf("Unable to copy UEFI OVMF vars: %s", err))
	}
	data.OvmfVars = types.StringValue(varsFile)

	data.QmpSocket = types.StringValue(fmt.Sprintf("unix:%s,server,nowait", filepath.Join(d, "qmp.socket")))

	qemuArgs := qemuStdArgs
	if !data.Name.IsNull() {
		qemuArgs = append(qemuArgs, []string{"--name", fmt.Sprintf("edge_node_%s", data.Name.ValueString())}...)
	} else {
		qemuArgs = append(qemuArgs, []string{"--name", fmt.Sprintf("edge_node_%s", data.ID.ValueString())}...)
	}
	mem := "4G"
	if !data.Mem.IsNull() {
		mem = data.Mem.ValueString()
	}
	cpus := "4"
	if !data.CPUs.IsNull() {
		cpus = data.CPUs.ValueString()
	}
	qemuArgs = append(qemuArgs, []string{
		"-m", mem,
		"-smp", cpus,
		"-smbios", fmt.Sprintf("type=1,serial=%s,manufacturer=Dell,product=ProLiant", data.SerialNo.ValueString()),
		"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", r.providerConf.BaseOVMFCode),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", varsFile),
		"-qmp", data.QmpSocket.ValueString(),
		"-pidfile", filepath.Join(d, "qemu.pid"),
	}...)

	nic0 := fmt.Sprintf(nic0Fmt, data.SSHPort.ValueInt32(),
		data.SSHPort.ValueInt32()+1, data.SSHPort.ValueInt32()+2)
	if !data.Nic0.IsNull() && len(data.Nic0.ValueString()) > 0 {
		nic0 = data.Nic0.ValueString()
	}
	qemuArgs = append(qemuArgs, []string{"-nic", nic0}...)

	if !data.DriveIf.IsNull() {
		qemuArgs = append(qemuArgs, []string{
			"-drive", fmt.Sprintf("file=%s,format=qcow2,if=%s",
				data.DiskImg.ValueString(), data.DriveIf.ValueString()),
		}...)
		if len(data.Disk1Img.ValueString()) > 0 {
			qemuArgs = append(qemuArgs, []string{
				"-drive", fmt.Sprintf("file=%s,format=qcow2,if=%s",
					data.Disk1Img.ValueString(), data.DriveIf.ValueString()),
			}...)
		}
	} else {
		qemuArgs = append(qemuArgs, []string{
			"-drive", fmt.Sprintf("file=%s,format=qcow2", data.DiskImg.ValueString()),
		}...)
		if len(data.Disk1Img.ValueString()) > 0 {
			qemuArgs = append(qemuArgs, []string{
				"-drive", fmt.Sprintf("file=%s,format=qcow2", data.Disk1Img.ValueString()),
			}...)
		}
	}

	swtpmSock := data.SwTPMSock.ValueString()
	if swtpmSock != "" {
		qemuArgs = append(qemuArgs, []string{
			// Define the character device connected to the swtpm socket.
			"-chardev", fmt.Sprintf("socket,id=chrtpm,path=%s", swtpmSock),
			// Define the TPM backend device using the character device.
			"-tpmdev", "emulator,id=tpm0,chardev=chrtpm",
			// Add the virtual TPM device to the VM (use tpm-crb for TPM 2.0).
			"-device", "tpm-crb,id=mytpm,tpmdev=tpm0",
			/*
				-chardev socket,id=chrtpm,path=/tmp/mytpm1/swtpm-sock \
				-tpmdev emulator,id=tpm0,chardev=chrtpm \
				-device tpm-tis,tpmdev=tpm0 test.img
			*/
		}...)
	}

	if data.SerialPortServer.ValueBool() {
		data.SerialPortSocket = types.StringValue(filepath.Join(d, "serial_port.socket"))
		data.SerialConsoleLog = types.StringValue("")
		qemuArgs = append(qemuArgs, []string{"-serial", fmt.Sprintf("unix:%s,server,wait", data.SerialPortSocket.ValueString())}...)

		data.SerialConsoleLog = types.StringValue(filepath.Join(d, "serial_console_run.log"))
		res, err = cmd.RunDetached(d, os.Args[0], []string{"-socket-tailer", "-st.connect", data.SerialPortSocket.ValueString(), "-st.out", data.SerialConsoleLog.ValueString()}...)
		if err != nil {
			resp.Diagnostics.AddError("Edge Node Resource Error",
				"Failed to run socket tailer")
			resp.Diagnostics.Append(res.Diagnostics()...)
			return
		}

	} else {
		data.SerialPortSocket = types.StringValue("")
		data.SerialConsoleLog = types.StringValue(filepath.Join(d, "serial_console_run.log"))
		qemuArgs = append(qemuArgs, []string{"-serial", fmt.Sprintf("file:%s", data.SerialConsoleLog.ValueString())}...)
	}

	extraArgs := []string{}
	if !data.ExtraArgs.IsNull() {
		diags := data.ExtraArgs.ElementsAs(ctx, &extraArgs, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		qemuArgs = append(qemuArgs, extraArgs...)
	}

	startVMscript := `#!/usr/bin/env bash

set -eu;

#### QEMU ARGS: %v

%s %s
`
	blob := []byte(fmt.Sprintf(startVMscript, qemuArgs, r.providerConf.Qemu, strings.Join(qemuArgs, " ")))
	if err := os.WriteFile(filepath.Join(d, "start_vm.bash"), blob, 0o755); err != nil {
		tflog.Debug(ctx, "Failed to write start VM script", map[string]any{"error": err})
	}

	res, err = cmd.RunDetached(d, r.providerConf.Qemu, qemuArgs...)
	if err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			"Failed to run QEMU VM for installing EVE-OS")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	tflog.Trace(ctx, "Edge Node Resource created succesfully")

	x, err := readEdgeNode(r.providerConf, r.getResourceDir(data.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddWarning("Edge Node Resource Read Error",
			fmt.Sprintf("Can't read EVE-OS console log: %v", err))

		return
	}

	data.VMRunning = types.BoolValue(x)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func readEdgeNode(_ *ZedAmigoProviderConfig, path string) (bool, error) {
	mon, err := qmp.NewSocketMonitor("unix", filepath.Join(path, "qmp.socket"), 1*time.Second)
	if err != nil {
		return false, fmt.Errorf("%w", err)
	}
	if err := mon.Connect(); err != nil {
		return false, fmt.Errorf("%w", err)
	}
	defer mon.Disconnect()

	cmd := []byte(`{ "execute": "query-status" }`)
	raw, err := mon.Run(cmd)
	if err != nil {
		return false, fmt.Errorf("%w", err)
	}

	type StatusResult struct {
		ID     string `json:"id"`
		Return struct {
			Running    bool   `json:"running"`
			Singlestep bool   `json:"singlestep"`
			Status     string `json:"status"`
		} `json:"return"`
	}

	qs := StatusResult{}
	if err := json.Unmarshal(raw, &qs); err != nil {
		return false, fmt.Errorf("%w", err)
	}

	return qs.Return.Running, nil

	/*
		x, err := os.ReadFile(filepath.Join(path, "serial_console_run.log"))
		if err != nil {
			return false, fmt.Errorf("%w", err)
		}

		return bytes.Contains(x, []byte("EVE-OS installation completed")), nil
	*/
}

func (r *EdgeNode) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data EdgeNodeModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	x, err := readEdgeNode(r.providerConf, r.getResourceDir(data.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddWarning("Edge Node Resource Read Warning",
			fmt.Sprintf("Treating this as a warning since most likely"+
				" the corresponding QEMU instance is not running anymore."+
				" Can't read EVE-OS console log: %v", err))
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

	// Try to shutdown the running VM via QMP.
	if data.VMRunning.ValueBool() {
		mon, err := qmp.NewSocketMonitor("unix", filepath.Join(d, "qmp.socket"), 2*time.Second)
		if err != nil {
			resp.Diagnostics.AddError("Edge Node Resource Delete Error",
				fmt.Sprintf("Can't create a QEMU QMP monitor: %v", err))
		}
		if err := mon.Connect(); err != nil {
			resp.Diagnostics.AddError("Edge Node Resource Delete Error",
				fmt.Sprintf("Can't QMP connect to QEMU: %v", err))
		}
		defer mon.Disconnect()

		cmd := []byte(`{ "execute": "quit" }`)
		raw, err := mon.Run(cmd)
		if err != nil {
			resp.Diagnostics.AddWarning("Edge Node Resource Delete Warning",
				fmt.Sprintf("Error encountered during sending the QEMU quit command via QMP,"+
					" however this might just be that we didn't get an answer before the QEMU process exited: %v", err))
		}

		type QMPResult struct {
			ID     string `json:"id"`
			Return struct {
				Status string `json:"status"`
			} `json:"return"`
		}

		qr := QMPResult{}
		if err := json.Unmarshal(raw, &qr); err != nil {
			resp.Diagnostics.AddWarning("Edge Node Resource Delete Warning",
				fmt.Sprintf("Malformed QEMU QMP answer,"+
					" however this might just be that the QEMU process exited: %v", err))
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
