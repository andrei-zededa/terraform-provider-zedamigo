# Embedded UEFI firmware

`OVMF_CODE.fd` and `OVMF_VARS.fd` are EVE-OS's own x86_64 OVMF build, taken unmodified from
the `lfedge/eve-uefi` package (source: [lf-edge/eve `pkg/uefi`](https://github.com/lf-edge/eve/tree/master/pkg/uefi)).
It is built with `-DTPM2_ENABLE=TRUE -DTPM2_CONFIG_ENABLE=TRUE`, so with a `zedamigo_swtpm`
attached the firmware performs a TPM 2.0 measured boot: it extends PCRs 0 to 7 and hands the
guest an event log.

That matters for EVE-OS guests. EVE seals its vault key to those PCRs, and since EVE 17.2 it
refuses to seal against all-zero firmware PCRs:

```
refusing to seal disk key: firmware anchor PCR(s) [0 4] are all-zero in the SHA-256 bank
```

The previously embedded OVMF did no measured boot, so an EVE 17.2+ node with a vTPM never
created its vault and stayed in maintenance mode.

| File | SHA-256 |
|---|---|
| `OVMF_CODE.fd` | `2cc1fb1735bb6211…` (see `sha256sum`) |
| `OVMF_VARS.fd` | `5d2ac383371b4083…` |

Source image: `lfedge/eve-uefi:f845442ab4cb972eaefd891bf72b8d668bf45629`
(the `pkg/uefi` tag of lf-edge/eve master `7cea040d8`, 2026-11-05).

## Refreshing

```shell
I=lfedge/eve-uefi:<tag>          # e.g. `linuxkit pkg show-tag pkg/uefi` in an EVE checkout
c=$(docker create --platform linux/amd64 "$I" /x)
docker cp "$c:/OVMF_CODE.fd" . && docker cp "$c:/OVMF_VARS.fd" .
docker rm "$c"
sha256sum OVMF_CODE.fd OVMF_VARS.fd
```

Then update the table above. The provider extracts these files to
`<lib_path>/embedded_ovmf/<content-hash>/`, so a new firmware takes effect for new VMs without
any manual cleanup on existing hosts. Use the provider's `ovmf_code` / `ovmf_vars` attributes to
boot a different firmware instead.
