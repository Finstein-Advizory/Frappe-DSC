#!/usr/bin/env bash
# ============================================================
# DSC Bridge — Linux uninstaller (per-user)
# ============================================================
# Reverses install.sh: removes browser trust + autostart, stops the running
# bridge, and deletes the binary. Leaves the data directory (paired-site
# tokens, TLS cert) unless you pass --purge.
# ============================================================
set -uo pipefail

APP="dsc-bridge"
BIN_DIR="${HOME}/.local/bin"
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/${APP}"

PURGE=0
[ "${1:-}" = "--purge" ] && PURGE=1

echo "Uninstalling ${APP} for user ${USER}..."

# Remove per-user trust + autostart (best-effort).
if [ -x "${BIN_DIR}/${APP}" ]; then
    "${BIN_DIR}/${APP}" --pre-uninstall || true
fi

# Stop a running instance.
pkill -x "${APP}" 2>/dev/null && echo "  stopped ${APP}" || true

# Remove the binary.
rm -f "${BIN_DIR}/${APP}" && echo "  removed ${BIN_DIR}/${APP}" || true

if [ "${PURGE}" -eq 1 ]; then
    rm -rf "${DATA_DIR}" && echo "  purged ${DATA_DIR}" || true
else
    echo "  kept data directory: ${DATA_DIR} (use --purge to remove)"
fi

echo "Done."
