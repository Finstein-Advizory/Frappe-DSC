# dsc-bridge — Desktop Agent for DSC Signing

A Go application that bridges the browser and USB DSC tokens via PKCS#11.
Runs on the signer's machine, listens on `https://127.0.0.1:4645`.

## Prerequisites

- Go 1.22+
- PKCS#11 library for your USB token (or SoftHSM2 for testing)

## Build

```bash
cd dsc_bridge
go mod tidy
go build -o dsc-bridge .
```

## Test with SoftHSM2

```bash
# Install SoftHSM2
sudo apt install softhsm2 opensc

# Initialise the test token + import a self-signed cert (idempotent)
./test_setup.sh

# Run the integration tests against the SoftHSM2 token
./build.sh integration
# (equivalent to: go test -tags softhsm -v ./...)

# Run the agent itself
./dsc-bridge
```

## Configuration

Optional config file at `~/.local/share/dsc-bridge/dsc-bridge.json`:

```json
{
  "host": "127.0.0.1",
  "port": 4645,
  "pkcs11_libs": ["/usr/lib/softhsm/libsofthsm2.so"]
}
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/status` | Agent status, detected tokens, paired sites |
| GET | `/v1/certs` | List certificates on plugged-in tokens (no PIN) |
| POST | `/v1/pair` | Pair agent with a Frappe site |
| POST | `/v1/sign` | Sign a hash using a specific certificate |

## Security

- HTTPS only (self-signed cert, fingerprint exchanged during pairing)
- Listens only on 127.0.0.1 (localhost)
- All requests (except /v1/pair and /v1/status) require `X-DSC-Site-Token` and `Origin` headers
- Private key never leaves the USB token
- Site tokens are stored in the **OS keychain** (Windows Credential Manager / macOS Keychain / Linux libsecret), not on disk

## Windows MSI

The CI workflow at `.github/workflows/dsc-bridge-msi.yml` builds a signed
MSI installer on `windows-latest`:

- Installs WiX Toolset v3 and `go-msi`
- Builds `dsc-bridge.exe` with `-H windowsgui` (no console window)
- Generates the MSI from `wix.json` (registers `dsc-bridge` as a Windows Service)
- Optionally Authenticode-signs if the repo has the secrets:
  - `CODE_SIGNING_PFX_BASE64` — base64 of the `.pfx` code-signing certificate
  - `CODE_SIGNING_PFX_PASSWORD` — password for the pfx
- Uploads the MSI as a workflow artifact

Trigger manually via **Actions → dsc-bridge MSI → Run workflow**, or push
changes under `dsc_bridge/`.

## Linux

The bridge runs on Linux as a system-tray GUI agent (like on Windows/macOS),
started automatically on login via an XDG autostart entry — not a systemd
service, because a tray + the pairing dialog need the graphical session.

### Build the packages

```bash
./build.sh linux            # native binary  -> build/dsc-bridge
./build.sh linux-package    # tarball        -> build/dsc-bridge-<ver>-linux-amd64.tar.gz
./build.sh deb              # Debian package -> build/dsc-bridge_<ver>_amd64.deb  (needs dpkg-deb)
```

### Install

```bash
# Tarball (any distro, no root needed for your own browsers)
tar -xzf dsc-bridge-<ver>-linux-amd64.tar.gz && ./dsc-bridge/install.sh

# Debian/Ubuntu (system-wide)
sudo apt install ./dsc-bridge_<ver>_amd64.deb
```

Runtime dependencies (declared by the `.deb`, install manually for the tarball):

```bash
sudo apt install libnss3-tools zenity libsecret-1-0 opensc   # Debian/Ubuntu
sudo dnf install nss-tools zenity libsecret opensc           # Fedora/RHEL
```

`libnss3-tools` provides `certutil` — **without it, browser trust is not
automated** and the signer must accept a one-time warning at
`https://127.0.0.1:4645`.

### What the installer automates

- **Certificate trust** — the bridge adds its own TLS cert to the current
  user's browser stores (`certutil`): Chrome/Chromium (`~/.pki/nssdb`) and every
  Firefox profile. This is per-user and self-heals on each startup
  (`EnsureUserTrust`), so a root `.deb` install still ends up trusted once the
  user logs in. For system-installed Firefox the `.deb` also drops an enterprise
  policy (`ImportEnterpriseRoots`).
- **Autostart** — an XDG `.desktop` under `~/.config/autostart` (tarball) or
  `/etc/xdg/autostart` (deb). Its `Exec` sets `DSC_BRIDGE_AUTO_CONFIRM_PAIRING=1`
  so first-time pairing needs no click; remove that env prefix for
  consent-on-pair.
- **PIN caching per session** — the first signature of a login session prompts
  for the token PIN; it is then held in memory (never on disk) and reused until
  the bridge restarts (logout/reboot).

Net signer experience: **open the document → Sign with DSC → enter PIN once per
session → signed.** See `linux-package/README-linux.txt` for the full user guide
and the Snap-Firefox caveat.

### Uninstall

```bash
./dsc-bridge/uninstall.sh    # tarball  (add --purge to delete tokens/cert)
sudo apt remove dsc-bridge   # deb
```
