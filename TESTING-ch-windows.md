# Testing Windows guests on the `ch` provider

**Status: VERIFIED — Windows Server 2025, full clean pipeline.** A real
Windows Server 2025 Standard image was installed, syspreped, and
successfully booted under real `cloud-hypervisor` (v53.0). A completely
unmodified `onctl create` → `onctl ssh` → `onctl destroy` cycle (no manual
console/registry intervention) was run end-to-end against it, through
onctl's actual `ch.go`/`common.go` code path (`ConfigDriveSeedPreparer` +
`DnsmasqDHCPManager`):
```
$ onctl create -n win2025-clean --provider ch --ch-os windows ... --username Administrator -k ~/.ssh/id_ed25519.pub
✔ VM is Ready
✔ VM Configured...
$ onctl ssh win2025-clean -- "whoami & hostname"
win2025-clean\administrator
win2025-clean
$ onctl destroy win2025-clean -f
✔ VM Destroyed: win2025-clean
```
`--username` (or `ch.vm.username` in `onctl.yaml`) must be set to
`Administrator` for both `create` and `ssh` — it's provider-level config,
not stored per-VM, same as the rest of `ch`/`fc`.

Windows 11 was **not** tested (TPM/Secure Boot bypass untested; expect the
same fixes in §3d/§3e to apply, plus the LabConfig registry bypass Windows
11's installer needs — see §3a).

This mirrors `TESTING-fc.md`'s style: a manual runbook for a Linux/KVM host
with `cloud-hypervisor` installed (same host `ch` already runs on for Linux
guests).

## 0. What you need before starting

- A Linux/KVM host (bare metal or nested virt, same as `fc-host` /
  `TESTING-fc.md`'s host).
- `qemu-system-x86_64` + OVMF (`apt install qemu-system-x86 ovmf` on
  Debian/Ubuntu) — used **only** to install/prep the Windows image; Cloud
  Hypervisor itself has no installer.
- A Windows Server or Windows 11 ISO. Cloud Hypervisor has only been tested
  upstream (per its own docs) with Windows Server 2019, Windows Server Core
  2004 and Windows 11 IoT Enterprise LTSC 2024 — **Windows 11 with TPM 2.0
  enabled is documented as not working**; disable/skip TPM. Windows Server
  2025 (this doc's actually-verified target) works too.
- The [virtio-win driver ISO](https://github.com/virtio-win/virtio-win-pkg-scripts)
  (paravirtual storage/net drivers Windows needs to see the guest's disks
  and NIC at all). **Only install the modern per-OS-folder drivers** (e.g.
  `NetKVM\2k25\amd64\`, `viostor\2k25\amd64\`) — a blind
  `pnputil /add-driver *.inf /subdirs` also picks up old, self-signed
  test-cert drivers from ancient OS folders (`Balloon\2k12\...`) that
  trigger an interactive "Windows Security" driver-signing prompt with no
  way to dismiss it headlessly (it renders on an input-isolated
  desktop/session synthetic keystrokes can't reach). Scope every
  `pnputil` call to the specific `<driver>\<osfolder>\amd64\` directory
  instead.
- [cloudbase-init](https://cloudbase-init.readthedocs.io/) — the Windows
  cloud-image customization agent onctl's seed ISO targets (§3). Install it
  inside the guest during image prep, same idea as `cloud-init` on a Linux
  cloud image. Silent install: `msiexec /i CloudbaseInitSetup_Stable_x64.msi
  /qn /norestart RUNSERVICEASLOCALSYSTEM=1 LOGGINGSERIALPORTNAME=COM1`. The
  installer automatically registers a sysprep hook
  (`<install dir>\conf\Unattend.xml`) — run sysprep with
  `/unattend:"<that path>"` (§3c) rather than a bare `/generalize /oobe
  /shutdown`, so cloudbase-init actually re-runs after generalization.
- `genisoimage`, `mkisofs`, or `xorriso` on the **onctl host** (not the
  guest) — onctl shells out to whichever is found first to build the seed
  ISO (`internal/providerch/common.go`'s `isoBuilders`).
- `dnsmasq` on the onctl host — used for DHCP on the Windows path only (the
  Linux `ch` path is untouched and doesn't need it).

## 1. Get Cloud Hypervisor's UEFI firmware

```bash
curl -LO https://github.com/cloud-hypervisor/edk2/releases/latest/download/CLOUDHV.fd
mkdir -p ~/.onctl/cloud-hypervisor/images
mv CLOUDHV.fd ~/.onctl/cloud-hypervisor/images/
```

## 2. Create the raw disk image

```bash
qemu-img create -f raw ~/.onctl/cloud-hypervisor/images/windows.raw 40G   # 40G is enough for Server; use 64G for Windows 11
```

## 3. Install Windows under QEMU+OVMF, with virtio-win + cloudbase-init

An unattended `autounattend.xml` install works well here (see Microsoft's
own answer-file schema) and is what was actually used to produce the
verified image — a manual VNC-driven install works identically. Either way,
install onto a **plain SATA/AHCI disk** (`-drive ...,if=none,id=disk0
-device ahci,id=ahci0 -device ide-hd,bus=ahci0.0,drive=disk0`), not
virtio-blk — this sidesteps needing a WinPE-phase driver injection just to
get Setup to see the disk at all. Attach the Windows ISO, and separately
attach virtio-win.iso for driver injection **after** install (§3b).

```bash
qemu-system-x86_64 \
  -enable-kvm -machine q35 -cpu host -smp 2 -m 4096 \
  -drive if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/OVMF_CODE_4M.fd \
  -drive if=pflash,format=raw,file=/tmp/OVMF_VARS.fd \
  -device ahci,id=ahci0 \
  -drive if=none,id=disk0,format=raw,file=~/.onctl/cloud-hypervisor/images/windows.raw \
  -device ide-hd,bus=ahci0.0,drive=disk0 \
  -drive if=none,id=cd0,media=cdrom,file=windows-server-2025.iso \
  -device ide-cd,bus=ahci0.1,drive=cd0 \
  -netdev user,id=net0 -device e1000,netdev=net0 \
  -boot order=d,menu=off \
  -vnc :0
```

(Watch for the "Press any key to boot from CD" El Torito prompt in the
first couple of boot seconds — miss it and Setup falls through to PXE.)

After install completes and you're at a logged-in desktop:

### 3a. (Windows 11 only) TPM/Secure Boot bypass

Not needed for Windows Server. For Windows 11, add a `RunSynchronous` block
to the `windowsPE` pass of `autounattend.xml` running, before disk setup:
```
reg add HKLM\SYSTEM\Setup\LabConfig /v BypassTPMCheck /t REG_DWORD /d 1 /f
reg add HKLM\SYSTEM\Setup\LabConfig /v BypassSecureBootCheck /t REG_DWORD /d 1 /f
reg add HKLM\SYSTEM\Setup\LabConfig /v BypassRAMCheck /t REG_DWORD /d 1 /f
reg add HKLM\SYSTEM\Setup\LabConfig /v BypassStorageCheck /t REG_DWORD /d 1 /f
reg add HKLM\SYSTEM\Setup\LabConfig /v BypassCPUCheck /t REG_DWORD /d 1 /f
```
This part is **unverified** — derived from well-documented community
practice, not confirmed against a real install in this pass.

### 3b. Install virtio-win drivers, scoped to the target OS folder only

```powershell
$osFolder = "2k25"   # or "w11" for Windows 11
Get-ChildItem E:\ -Recurse -Filter *.inf |
  Where-Object { $_.FullName -match "\\$osFolder\\amd64\\" } |
  ForEach-Object { pnputil /add-driver $_.FullName /install }
```
(`E:` = the mounted virtio-win ISO.) This only stages the drivers into the
**driver store** — it does *not* make the boot disk bootable from
virtio-blk by itself. See §3d.

### 3c. Install cloudbase-init and sysprep

```powershell
msiexec /i CloudbaseInitSetup_Stable_x64.msi /qn /norestart RUNSERVICEASLOCALSYSTEM=1 LOGGINGSERIALPORTNAME=COM1

$conf = "C:\Program Files\Cloudbase Solutions\Cloudbase-Init\conf"
Add-Content "$conf\cloudbase-init.conf" "metadata_services=cloudbaseinit.metadata.services.configdrive.ConfigDriveService"
Add-Content "$conf\cloudbase-init-unattend.conf" "metadata_services=cloudbaseinit.metadata.services.configdrive.ConfigDriveService"

Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
Start-Service sshd
Set-Service sshd -StartupType Automatic

& "$env:WINDIR\System32\Sysprep\sysprep.exe" /generalize /oobe /shutdown /unattend:"$conf\Unattend.xml"
```

Shut down after sysprep completes. `windows.raw` is now the reusable base
image — onctl copies it per-VM (`ch.rootfsImage`, or `--ch-rootfs-image`)
and never modifies it in place, same contract as the Linux `rootfs.ext4`
base image.

### 3d. Make the boot disk actually bootable from virtio-blk

**This is the step that isn't obvious and will boot-loop
(`INACCESSIBLE_BOOT_DEVICE`, no on-screen error since Cloud Hypervisor has
no VGA) if skipped.** `pnputil /add-driver` alone does not register
`viostor` as a **boot-critical** driver — that requires a
`CriticalDeviceDatabase` entry mapping the virtio-blk PCI hardware ID to
the service, and that entry only gets created automatically by live PnP
enumeration of an actually-present device — which never happened, since
Setup ran against a SATA disk (§3, deliberately, to dodge WinPE driver
injection).

Confirmed via `viostor.inf` under `virtio-win.iso` (device IDs are
spec-standard across VMMs; Cloud Hypervisor's own subsystem ID differs from
QEMU's, so add both the QEMU- and CH-shaped IDs, plus the ID-only fallback):

```bash
# offline, from the host — mount the image and add the CDD keys directly
losetup -fP --show windows.raw            # e.g. /dev/loop0
mount -t ntfs3 /dev/loop0p3 /mnt/win        # ntfs3 kernel driver (Linux 5.15+)
```

```
reged -I /mnt/win/Windows/System32/config/SYSTEM "HKEY_LOCAL_MACHINE\SYSTEM" cdd_fix.reg
```
where `cdd_fix.reg` contains, for **both** `ControlSet001` and
`ControlSet002` (check `HKLM\SYSTEM\Select` for which is current):
```
[HKEY_LOCAL_MACHINE\SYSTEM\ControlSet001\Control\CriticalDeviceDatabase\pci#ven_1af4&dev_1042]
"ClassGUID"="{4D36E97B-E325-11CE-BFC1-08002BE10318}"
"Service"="viostor"

[HKEY_LOCAL_MACHINE\SYSTEM\ControlSet001\Control\CriticalDeviceDatabase\pci#ven_1af4&dev_1042&subsys_10421af4&rev_01]
"ClassGUID"="{4D36E97B-E325-11CE-BFC1-08002BE10318}"
"Service"="viostor"
```
(`10421af4` is Cloud Hypervisor's subsystem ID for virtio-blk — its
`PciConfiguration::new(...)` sets `subsystem_vendor_id`/`subsystem_device_id`
to the same values as the main vendor/device ID, i.e.
`VIRTIO_PCI_VENDOR_ID` (0x1af4) and `0x1040 + block(2) = 0x1042` — a
*different* convention from QEMU's `1100`-based subsystem IDs. The
ID-only key without `&subsys_...&rev_...` is what actually made this work
in practice; keep it regardless of which VMM you're targeting.)

Alternative that reaches the same end state with no offline registry
surgery: after §3c but **before** the final sysprep+shutdown, boot the
image once under QEMU with `-drive ...,if=virtio` instead of AHCI/SATA —
a live PnP pass against a real virtio-blk device writes the same
boot-critical registration automatically. This is what was actually done
to produce the verified image, and it's also how the two gotchas below
were caught.

### 3e. Two more gotchas that only show up once the disk boots

Both are guest-OS/network-stack issues, not onctl-specific — but they will
silently prevent `onctl ssh` from working even once boot succeeds, so fix
them in the base image now:

1. **`administrators_authorized_keys`.** Windows' OpenSSH Server has a
   `Match Group administrators` rule in `sshd_config` that ignores
   `~/.ssh/authorized_keys` for admin-group accounts (which `Administrator`
   always is) in favor of `%ProgramData%\ssh\administrators_authorized_keys`.
   cloudbase-init's `SetUserSSHPublicKeysPlugin` writes only the per-user
   file, so admin login still fails after a clean cloudbase-init run.
   **Do not just comment out the `Match Group administrators` block** —
   that block exists because Windows OpenSSH's normal per-user
   `%h`-relative `authorized_keys` resolution is unreliable for the
   built-in Administrator account specifically (its profile directory is
   `C:\Users\Admin`, not `C:\Users\Administrator`); removing the block was
   tried here and made login fail outright, not just fall back to a
   different file. Keep the block, and instead add a cloudbase-init
   `LocalScriptsPlugin` script (drop a `.ps1` in
   `C:\Program Files\Cloudbase Solutions\Cloudbase-Init\LocalScripts\` —
   already on `local_scripts_path` in `cloudbase-init.conf` by default,
   runs last, after `SetUserSSHPublicKeysPlugin`) that copies whichever
   per-user `authorized_keys` was just written to
   `%ProgramData%\ssh\administrators_authorized_keys`:
   ```powershell
   $src = Get-ChildItem "C:\Users\*\.ssh\authorized_keys" -ErrorAction SilentlyContinue |
       Sort-Object LastWriteTime -Descending | Select-Object -First 1
   if ($src) {
       $dest = "C:\ProgramData\ssh\administrators_authorized_keys"
       Copy-Item $src.FullName $dest -Force
       icacls $dest /inheritance:r /grant Administrators:F /grant SYSTEM:F | Out-Null
   }
   ```
   (`icacls` is required — sshd rejects that file if it's readable by
   anyone else.) This re-runs on every fresh onctl deployment (new
   instance UUID → cloudbase-init treats it as unseen → all plugins,
   including this script, run again), so it isn't a one-off fix tied to a
   single VM's key.
2. **Network profile defaults to Public after generalize**, and the
   OpenSSH-capability's auto-created firewall rule
   (`OpenSSH SSH Server (sshd)`) is scoped to the **Private** profile only
   — so even with the key fixed, the SSH port is unreachable. Fix in the
   base image: `Set-NetFirewallRule -Name "OpenSSH-Server-In-TCP" -Profile
   Any` (broadening the rule is more robust than trying to force the
   network category, which sysprep resets on every generalize).

## 4. Sanity-check the image boots under Cloud Hypervisor directly (bypassing onctl)

Do this **before** wiring onctl in at all — it isolates "does Cloud
Hypervisor + this image work" from "does onctl's seed-ISO/DHCP code work".
No VGA means no on-screen errors; if this hangs or resets in a loop with
nothing useful in the log, enable EMS first (`bcdedit /ems on;
bcdedit /emssettings EMSPORT:1 EMSBAUDRATE:115200` inside the guest, one
more boot under QEMU, then shut down) so boot diagnostics and the SAC
console mirror to COM1/the `--serial tty` log.

```bash
cp ~/.onctl/cloud-hypervisor/images/windows.raw /tmp/win-test.raw

sudo cloud-hypervisor \
  --api-socket /tmp/ch.sock \
  --kernel ~/.onctl/cloud-hypervisor/images/CLOUDHV.fd \
  --disk path=/tmp/win-test.raw \
  --cpus boot=2,kvm_hyperv=on \
  --memory size=4096M \
  --net "tap=,mac=52:54:00:12:34:56" \
  --serial tty --console off
```

If this doesn't boot to a login/SAC prompt, nothing above it (onctl's DHCP
or seed-ISO plumbing) will work either — fix the image/firmware first (most
likely §3d).

## 5. Run onctl end-to-end

```bash
sudo onctl create -n win-test --provider ch --ch-os windows \
  --ch-kernel-image ~/.onctl/cloud-hypervisor/images/CLOUDHV.fd \
  --ch-rootfs-image ~/.onctl/cloud-hypervisor/images/windows.raw \
  --vcpu 2 --memory 4096 \
  --username Administrator \
  -k ~/.ssh/id_rsa.pub

# `ssh` has no --username flag (unlike `create`) -- set it once in
# onctl.yaml instead (ch.vm.username: Administrator), or it defaults to
# root and auth fails.
sudo onctl ssh win-test -- whoami   # confirm cloudbase-init applied the hostname/SSH key and sshd is reachable

sudo onctl destroy win-test -f   # confirm the bridge/tap/DHCP reservation/state dir are all cleaned up
```

## 6. Confirm the Linux `ch` path is unaffected

```bash
sudo onctl create -n linux-test --provider ch \
  --ch-kernel-image ~/.onctl/cloud-hypervisor/images/vmlinux \
  --ch-rootfs-image ~/.onctl/cloud-hypervisor/images/rootfs.ext4
sudo onctl ssh linux-test
sudo onctl destroy linux-test -f
```

(no `--ch-os` needed — defaults to `linux`, unchanged from before this
integration.)

## Known limitations

- No VGA console — interact via SSH (once cloudbase-init + sshd are up) or
  Cloud Hypervisor's serial-port SAC (`--serial tty`; enable EMS in the
  guest first, §4).
- No pause/resume/snapshot for `ch` at all (Windows or Linux) — `onctl
  destroy` + `onctl create` only.
- Windows 11 + TPM 2.0 is documented upstream as not working on Cloud
  Hypervisor. Windows 11 generally (TPM bypassed) has not been tested here
  — only Windows Server 2025.
- Two real dnsmasq bugs were found and fixed while verifying this:
  `chMAC()` used to emit a mixed-case address (`02:C4:...`); dnsmasq's
  `--dhcp-hostsfile` matching is case-sensitive against the
  lowercase-normalized MAC a DHCP client actually sends, so the
  reservation silently never matched its own guest. And
  `DnsmasqDHCPManager` used to rely on dnsmasq's system-wide default lease
  file (`/var/lib/misc/dnsmasq.leases`), shared with *any* other dnsmasq on
  the host — a stale lease there for an onctl-reserved address (e.g. from
  an unrelated manual test) silently wins over a fresh
  `--dhcp-hostsfile` reservation for the same address, no error, dnsmasq
  just allocates a different one from the free pool. Both are fixed
  (`chMAC` is lowercase throughout; each bridge gets its own
  `--dhcp-leasefile` under onctl's state dir) — if you're testing against
  an onctl build from before this fix, watch for `onctl ssh` timing out to
  the IP `onctl ls` reports, and check `ip neigh` / dnsmasq's own DHCPOFFER
  log for a mismatch.
