---
page_title: "Inspecting the vTPM (swtpm) with tpm2-tools"
description: |-
  How to check the software TPM created by the zedamigo_swtpm resource, from
  inside a guest VM and from the host, including reading the endorsement key.
---

# Inspecting the vTPM (swtpm) with tpm2-tools

A `zedamigo_swtpm` resource runs an [swtpm](https://github.com/stefanberger/swtpm)
process that emulates a TPM 2.0 device for edge node VMs. This guide shows how
to verify that the vTPM works and how to read its identity (the endorsement
key, EK) — both from inside a guest VM and at the host level.

## How the pieces fit together

On the **host**, each `zedamigo_swtpm` resource owns a directory under the
provider `lib_path` (default `$HOME/.local/state/zedamigo/`):

```
$HOME/.local/state/zedamigo/swtmps/<id>/
├── swtpm.socket          # QEMU control channel (the resource's `socket` attribute)
├── swtpm.pid             # PID of the swtpm process
├── process_monitor.pid   # PID of the supervisor that restarts swtpm
├── swtpm.log             # swtpm log at level 20: every TPM command is logged
└── state/                # ALL persistent TPM state, including the EK seed
```

To find the directory for a given resource:

```shell
terraform state show zedamigo_swtpm.example   # the `socket` attribute
# the resource directory is the directory containing the socket
```

In the **guest**, QEMU exposes the vTPM as a CRB-interface TPM 2.0 device
(only when the edge node resource sets `swtpm_socket`). The kernel presents
it as `/dev/tpm0` (raw) and `/dev/tpmrm0` (kernel resource manager).

An important asymmetry: the host-side `swtpm.socket` is the **control**
channel only. The TPM **command** channel is a file descriptor that QEMU
receives over the control socket, so there is no host-side socket you can
point `tpm2_*` tools at directly. Host-level TPM commands instead use a
throwaway copy of the state directory (see below).

## Prerequisites

* Guest: the `tpm2-tools` package (Debian/Ubuntu/Fedora/Alpine all ship it).
  The tools default to `/dev/tpmrm0`, no configuration needed.
* Host: `swtpm` (already required by the provider); optionally `swtpm-tools`
  (for `swtpm_ioctl`) and `tpm2-tools` (for the state-copy inspection).

## Inside the guest

### Is the TPM there at all?

```shell
ls -l /dev/tpm*          # expect /dev/tpm0 and /dev/tpmrm0
dmesg | grep -i tpm
```

### Identify the TPM

```shell
tpm2_getcap properties-fixed
```

For an swtpm-backed vTPM expect `TPM2_PT_MANUFACTURER` = `"IBM "` (swtpm is
based on IBM's libtpms), `TPM2_PT_FAMILY_INDICATOR` = `"2.0"`, and the
firmware version fields reflecting the libtpms version.

### Basic health

```shell
tpm2_selftest --fulltest
tpm2_gettestresult
tpm2_getrandom 8 --hex     # 8 random bytes prove the command path works
```

### PCRs (measured boot)

```shell
tpm2_pcrread                       # all banks, all PCRs
tpm2_pcrread sha256:0,1,2,3,4,7    # the interesting boot-measurement PCRs
```

A generic guest without measured boot shows all-zero PCRs; an EVE-OS guest
populates them during boot (EVE seals its vault against these values). A safe
write test that does not disturb anything (PCR 23 is the debug PCR, reset on
reboot):

```shell
tpm2_pcrextend 23:sha256=$(echo test | sha256sum | cut -d' ' -f1)
tpm2_pcrread sha256:23             # now non-zero
```

### The endorsement key (EK) — the TPM's identity

The EK is derived deterministically from a seed stored in the TPM state, so
it identifies this particular vTPM for its whole life:

```shell
tpm2_createek -G ecc -c ek.ctx -u ek.pub      # or -G rsa
tpm2_readpublic -c ek.ctx -f pem -o ek.pem
openssl pkey -pubin -in ek.pem -outform DER | sha256sum   # EK fingerprint
```

Re-running this after reboots, or in a different VM attached to the same
`zedamigo_swtpm` (e.g. the EVE-OS installer VM and then the installed edge
node), must produce the same fingerprint.

### EK certificate: expected to be absent

Physical TPMs and fully provisioned vTPMs carry an EK certificate in NV
memory (`0x01c00002` for RSA, `0x01c0000a` for ECC). The provider starts a
raw swtpm without running `swtpm_setup`, so **no EK certificate exists**:

```shell
tpm2_nvreadpublic                  # lists NV indices; expect none of the above
tpm2_nvread 0x01c00002 -o ek.crt   # fails on a zedamigo vTPM — expected
```

### Keys created by the guest OS

EVE-OS creates its device key and other keys as persistent TPM objects once
it has run. List them with:

```shell
tpm2_getcap handles-persistent
```

## At the host level

### Watch the TPM work

swtpm runs at log level 20, which logs every command and control message —
this is the fastest way to see what a guest is doing to its TPM (including
the `Shutdown` control command QEMU sends when the VM powers off, after which
the provider's process monitor restarts swtpm):

```shell
tail -f "$DIR/swtpm.log"
```

### Query the control channel (swtpm_ioctl)

The control socket answers informational requests (from the `swtpm-tools`
package):

```shell
swtpm_ioctl --unix "$DIR/swtpm.socket" --info 0x1   # TPM specification, JSON
swtpm_ioctl --unix "$DIR/swtpm.socket" --info 0x2   # TPM attributes, JSON
swtpm_ioctl --unix "$DIR/swtpm.socket" -g           # configuration flags
swtpm_ioctl --unix "$DIR/swtpm.socket" -e           # TPM-established flag
```

Avoid the state-changing control commands against a live resource: `-i`
(re-initialize) and `-s`/`--stop` (shutdown/stop) disturb the TPM under a
running VM. (The provider's monitor restarts a shut-down swtpm within about a
second, but the attached VM's TPM sessions are gone.)

### Read the EK (and anything else) via a throwaway copy

All persistent TPM state — including the EK seed — lives in `$DIR/state/`.
Copying it and running a disposable swtpm with TCP sockets gives full
`tpm2_*` access without touching the real instance:

```shell
DIR=$HOME/.local/state/zedamigo/swtmps/<id>
cp -a "$DIR/state" /tmp/tpm-inspect

swtpm socket --tpm2 --daemon \
  --tpmstate dir=/tmp/tpm-inspect \
  --server type=tcp,port=2321 \
  --ctrl type=tcp,port=2322 \
  --pid file=/tmp/tpm-inspect/pid \
  --log level=5,file=/tmp/tpm-inspect/log \
  --flags not-need-init,startup-clear

# tpm2-tools' swtpm TCTI: `port` is the command port, `port + 1` MUST be the
# control port (hence 2321/2322 above).
export TPM2TOOLS_TCTI="swtpm:host=127.0.0.1,port=2321"

tpm2_getcap properties-fixed
tpm2_createek -G ecc -c /tmp/tpm-inspect/ek.ctx -u /tmp/tpm-inspect/ek.pub
tpm2_readpublic -c /tmp/tpm-inspect/ek.ctx -f pem -o /tmp/tpm-inspect/ek.pem
openssl pkey -pubin -in /tmp/tpm-inspect/ek.pem -outform DER | sha256sum

kill "$(cat /tmp/tpm-inspect/pid)"
rm -rf /tmp/tpm-inspect
```

The EK fingerprint printed here matches the one computed inside the guest:
same seed, same key. Notes:

* The copy is fully disposable — nothing done to it affects the real vTPM.
* PCR values are volatile and are NOT part of the copied state; the throwaway
  instance starts with reset PCRs regardless of what the guest measured.
* Prefer copying while the VM is powered off (or the TPM idle) to avoid
  catching a state file mid-write. The EK seed is written once at first
  initialization, so EK extraction is robust either way.

## Verifying identity persistence across installer → installed VM

The reason the same `zedamigo_swtpm` must serve both the EVE-OS installer VM
and the installed edge node VM is TPM-bound device identity. To verify it
end to end, compute the EK fingerprint at any two (or all three) of these
points and compare:

1. inside the installer VM guest,
2. inside the installed edge node guest,
3. on the host via the throwaway-copy method.

All fingerprints must be identical for a given `zedamigo_swtpm` resource. A
new resource (or a changed `name`, which forces replacement) means a new
seed, i.e. a different fingerprint.

## Quick reference

| Command                                          | What it tells you                                    |
|--------------------------------------------------|------------------------------------------------------|
| `tpm2_getcap properties-fixed`                   | Manufacturer, TPM 2.0 family, firmware version       |
| `tpm2_selftest --fulltest` / `tpm2_gettestresult`| TPM self-test status                                  |
| `tpm2_getrandom 8 --hex`                         | Command path works, RNG output                        |
| `tpm2_pcrread`                                   | PCR values (measured boot state)                      |
| `tpm2_createek` + `tpm2_readpublic`              | Endorsement key — the TPM's identity                  |
| `tpm2_nvreadpublic`                              | NV indices (EK certificates live here when present)   |
| `tpm2_getcap handles-persistent`                 | Keys the guest OS persisted in the TPM                |
| `swtpm_ioctl --info 0x1` (host, ctrl socket)     | swtpm's view of the TPM specification                 |
| `tail -f "$DIR/swtpm.log"` (host)                | Live trace of every TPM command the guest issues      |
