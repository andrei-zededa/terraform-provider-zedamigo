// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/cmd"
	"github.com/andrei-zededa/terraform-provider-zedamigo/internal/qmp"
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
	SerialNo         types.String `tfsdk:"serial_no"`
	DiskImgBase      types.String `tfsdk:"disk_image_base"`
	DiskImg          types.String `tfsdk:"disk_image"`
	SerialConsoleLog types.String `tfsdk:"serial_console_log"`
	OvmfVarsSrc      types.String `tfsdk:"ovmf_vars_src"`
	OvmfVars         types.String `tfsdk:"ovmf_vars"`
	QmpSocket        types.String `tfsdk:"qmp_socket"`
	VMRunning        types.Bool   `tfsdk:"vm_running"`
	SSHPort          types.Int32  `tfsdk:"ssh_port"`
}

func (r *EdgeNode) getResourceDir(id string) string {
	return filepath.Join(r.providerConf.LibPath, edgeNodesDir, id)
}

func (r *EdgeNode) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_edge_node"
}

func (r *EdgeNode) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Edge Node",
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Edge Node",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Edge Node identifier",
				MarkdownDescription: "Edge Node identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description:         "Edge Node name",
				MarkdownDescription: "Edge Node name",
				Optional:            true,
				Required:            false,
			},
			"serial_no": schema.StringAttribute{
				Description:         "Edge Node serial number",
				MarkdownDescription: "Edge Node serial number",
				Optional:            false,
				Required:            true,
			},
			"disk_image_base": schema.StringAttribute{
				Description:         "Disk image base from which the actual disk image used for this node will be created (qemu-img backing file)",
				MarkdownDescription: "Disk image base from which the actual disk image used for this node will be created (qemu-img backing file)",
				Optional:            false,
				Required:            true,
			},
			"disk_image": schema.StringAttribute{
				Description:         "Edge Node disk image",
				MarkdownDescription: "Edge Node disk image",
				Computed:            true,
			},
			"serial_console_log": schema.StringAttribute{
				Description:         "Edge Node log file of serial console output",
				MarkdownDescription: "Edge Node log file of serial console output",
				Computed:            true,
			},
			"ovmf_vars_src": schema.StringAttribute{
				Description:         "UEFI OVMF vars source file (likely from the corresponding installed edge node)",
				MarkdownDescription: "UEFI OVMF vars source file (likely from the corresponding installed edge node)",
				Optional:            false,
				Required:            true,
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
	data.DiskImg = types.StringValue(filepath.Join(d, "disk0.disk_img.qcow2"))
	res, err := cmd.Run(d, r.providerConf.QemuImg, "create", "-f", "qcow2",
		"-b", data.DiskImgBase.ValueString(), "-F", "qcow2", data.DiskImg.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			"Unable to create a new disk image")
		resp.Diagnostics.Append(res.Diagnostics()...)
		return
	}

	varsFile := filepath.Join(d, "UEFI_OVMF_VARS.bin")
	if _, err := cmd.CopyFile(data.OvmfVarsSrc.ValueString(), varsFile); err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Error",
			fmt.Sprintf("Unable to copy UEFI OVMF vars: %s", err))
	}
	data.OvmfVars = types.StringValue(varsFile)

	data.SerialConsoleLog = types.StringValue(filepath.Join(d, "serial_console_run.log"))

	data.QmpSocket = types.StringValue(fmt.Sprintf("unix:%s,server,nowait", filepath.Join(d, "qmp.socket")))

	qemuArgs := []string{}
	if !data.Name.IsNull() {
		qemuArgs = append(qemuArgs, []string{"--name", fmt.Sprintf("edge_node_%s", data.Name.ValueString())}...)
	} else {
		qemuArgs = append(qemuArgs, []string{"--name", fmt.Sprintf("edge_node_%s", data.ID.ValueString())}...)
	}

	qemuArgs = append(qemuArgs, []string{
		"--enable-kvm", "-machine", "q35,accel=kvm,kernel-irqchip=split",
		"-nographic",
		"-m", "4096",
		"-cpu", "host", "-smp", "4,cores=2",
		"-device", "intel-iommu,intremap=on",
		"-smbios", fmt.Sprintf("type=1,serial=%s", data.SerialNo.ValueString()),
		"-nic", fmt.Sprintf("user,id=usernet0,hostfwd=tcp::%d-:22,model=virtio", data.SSHPort.ValueInt32()),
		"-serial", fmt.Sprintf("file:%s", data.SerialConsoleLog.ValueString()),
		"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", ovmfCode),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", varsFile),
		"-drive", fmt.Sprintf("file=%s,format=qcow2", data.DiskImg.ValueString()),
		"-qmp", data.QmpSocket.ValueString(),
		"-pidfile", filepath.Join(d, "qemu.pid"),
	}...)

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
		resp.Diagnostics.AddError("Edge Node Resource Read Error",
			fmt.Sprintf("Can't read EVE-OS console log: %v", err))
		return
	}

	data.VMRunning = types.BoolValue(x)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func readEdgeNode(_ *ZedAmigoProviderConfig, path string) (bool, error) {
	mon, err := qmp.NewSocketMonitor("unix", filepath.Join(path, "qmp.socket"), 2*time.Second)
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

	_, err := readEdgeNode(r.providerConf, r.getResourceDir(data.ID.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Edge Node Resource Read Error",
			fmt.Sprintf("Can't read EVE-OS console log: %v", err))
		return
	}

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
