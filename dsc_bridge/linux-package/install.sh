#!/usr/bin/env bash
# ============================================================
# DSC Bridge — Linux installer (per-user, no root required)
# ============================================================
# Installs the bridge for the CURRENT user:
#   - binary  -> ~/.local/bin/dsc-bridge
#   - config  -> ~/.local/share/dsc-bridge/dsc-bridge.json  (kept if present)
#   - runs `dsc-bridge --post-install`, which:
#       * adds the bridge's TLS cert to THIS user's browser trust stores
#         (Chrome/Chromium ~/.pki/nssdb and every Firefox profile) so the
#         browser never warns — no sudo needed for your own browsers
#       * installs an autostart entry so the bridge launches on login
#   - starts the bridge now, in the background
#
# After this, signing is: open the document -> Sign with DSC -> enter the
# token PIN once per session -> signed. Nothing to start by hand.
#
# Requirements (install once):
#   Debian/Ubuntu:  sudo apt install libnss3-tools zenity libsecret-1-0
#   Fedora:         sudo dnf install nss-tools zenity libsecret
#   (plus your DSC token's PKCS#11 driver, e.g. opensc)
# ============================================================
set -euo pipefail

APP="dsc-bridge"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

BIN_DIR="${HOME}/.local/bin"
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/${APP}"

echo "Installing ${APP} for user ${USER}..."

if [ ! -f "${HERE}/${APP}" ]; then
    echo "ERROR: ${APP} binary not found next to this script (${HERE}/${APP})."
    echo "       Build it first: ./build.sh linux"
    exit 1
fi

mkdir -p "${BIN_DIR}" "${DATA_DIR}"

install -m 0755 "${HERE}/${APP}" "${BIN_DIR}/${APP}"
echo "  installed ${BIN_DIR}/${APP}"

# Never clobber an existing config (may hold custom pkcs11_libs).
if [ ! -f "${DATA_DIR}/${APP}.json" ] && [ -f "${HERE}/${APP}.json" ]; then
    install -m 0644 "${HERE}/${APP}.json" "${DATA_DIR}/${APP}.json"
    echo "  installed ${DATA_DIR}/${APP}.json"
fi

# Warn (don't fail) if certutil is missing — browser trust needs it.
if ! command -v certutil >/dev/null 2>&1; then
    echo "  NOTE: 'certutil' not found. Install libnss3-tools (Debian/Ubuntu) or"
    echo "        nss-tools (Fedora) for automatic browser trust, then re-run this."
fi

# Cert trust + autostart entry (runs as this user; no sudo).
"${BIN_DIR}/${APP}" --post-install || echo "  WARN: post-install reported an issue (see above); continuing."

# Start it now, detached, inside the graphical session so the tray + pairing
# dialog work. Skip if it already appears to be running.
if ! pgrep -x "${APP}" >/dev/null 2>&1; then
    setsid "${BIN_DIR}/${APP}" >/dev/null 2>&1 < /dev/null &
    echo "  started ${APP}"
else
    echo "  ${APP} already running"
fi

echo
echo "Done. If '${BIN_DIR}' is not on your PATH, add it:"
echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
echo
echo "The bridge will start automatically on future logins."
echo "To sign: open the document, click 'Sign with DSC', enter the token PIN."
