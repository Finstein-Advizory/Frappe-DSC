DSC Bridge — Linux
==================

The DSC Bridge is a small tray agent that runs on the signer's own machine. The
browser talks to it over https://127.0.0.1:4645 to sign documents with a
hardware DSC (Digital Signature Certificate) USB token via PKCS#11.

Once installed it is fully automatic:

    open the document  ->  click "Sign with DSC"  ->  enter the token PIN once
                                                       per session  ->  signed

No starting the bridge by hand, no certificate warnings, no pairing step.


-------------------------------------------------------------------------------
1. Install the prerequisites (one time)
-------------------------------------------------------------------------------
Debian / Ubuntu:
    sudo apt install libnss3-tools zenity libsecret-1-0 opensc

Fedora / RHEL:
    sudo dnf install nss-tools zenity libsecret opensc

  - libnss3-tools  : provides `certutil`, used to trust the bridge cert in
                     Chrome/Firefox automatically. Without it you'd have to
                     accept a browser warning once.
  - zenity         : the native "Allow pairing?" dialog (only if you disable
                     auto-confirm; see section 5).
  - libsecret      : stores the per-site token in your login keyring.
  - opensc         : a generic PKCS#11 driver. If your token vendor ships its
                     own .so, you can point the bridge at it (section 4).

Tray icon note: the tray icon uses the system AppIndicator. On GNOME you may
need the "AppIndicator and KStatusNotifierItem" extension for the icon to show.
Signing works regardless of whether the icon is visible.


-------------------------------------------------------------------------------
2. Install the bridge
-------------------------------------------------------------------------------
Option A — tarball (any distro, no root for your own browsers):
    ./install.sh

Option B — Debian/Ubuntu package (system-wide):
    sudo apt install ./dsc-bridge_<version>_amd64.deb

Either way the installer:
  - trusts the bridge's TLS certificate in your browsers,
  - sets the bridge to start automatically on login,
  - starts it immediately.


-------------------------------------------------------------------------------
3. Sign
-------------------------------------------------------------------------------
Plug in your DSC token, open the document in the site, and click
"Sign with DSC". The first signature of each login session asks for the token
PIN; after that, signing this session does not ask again (PIN is cached in
memory only and cleared when you log out / reboot).


-------------------------------------------------------------------------------
4. Using a vendor PKCS#11 driver
-------------------------------------------------------------------------------
The bridge auto-detects common drivers (OpenSC, SafeNet eToken, ePass2003,
HYP/Hypersecu, Watchdata, TrustKey, ProxKey, ...). If yours isn't found, add
its .so path to the config:

    ~/.local/share/dsc-bridge/dsc-bridge.json     (tarball install)
    /etc/dsc-bridge/dsc-bridge.json               (deb install, if provided)

    {
      "host": "127.0.0.1",
      "port": 4645,
      "pkcs11_libs": ["/usr/lib/your-vendor/libyourpkcs11.so"]
    }

Then restart the bridge (log out/in, or `pkill dsc-bridge` and start it again).


-------------------------------------------------------------------------------
5. Security notes
-------------------------------------------------------------------------------
- Auto-confirm pairing: the autostart entry runs the bridge with
  DSC_BRIDGE_AUTO_CONFIRM_PAIRING=1 so the very first pairing needs no click.
  For consent-on-pair instead, remove that env prefix from the Exec line in:
      ~/.config/autostart/dsc-bridge.desktop   (tarball)
      /etc/xdg/autostart/dsc-bridge.desktop    (deb)

- PIN caching is per session (in memory). Anyone at your unlocked machine can
  sign until you log out. Lock your screen when away.

- Firefox: trust is added directly to each Firefox profile. If you use the
  Snap build of Firefox and signing still warns, open
  https://127.0.0.1:4645/v1/status once and accept, or ensure the Snap profile
  under ~/snap/firefox/common/.mozilla/firefox exists before installing.


-------------------------------------------------------------------------------
6. Uninstall
-------------------------------------------------------------------------------
Tarball:   ./uninstall.sh            (add --purge to also delete tokens/cert)
Deb:       sudo apt remove dsc-bridge
