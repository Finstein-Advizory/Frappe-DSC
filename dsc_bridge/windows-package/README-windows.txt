DSC Bridge — Windows Setup
===========================

This package lets you sign documents with your DSC USB token (e.g. HYP2003).
You install it ONCE. After that it starts automatically every time you log in,
and you just plug in the token and click "Sign with DSC".

What's in this package
----------------------
1. dsc-bridge.exe        - the bridge program
2. dsc-bridge.json       - config (token driver paths)
3. install.bat           - ONE-CLICK installer (use this)
4. uninstall.bat         - remover
5. start-dsc-bridge.bat  - manual start (only if you skip install.bat)
6. README-windows.txt    - this file

One-time setup
--------------
1. Install your token's driver:
   - Many tokens install their driver automatically when plugged in.
   - If not, install it from your CA (eMudhra / Sify / Capricorn / etc.) or the
     CD that came with the token. You should then see your name in the vendor's
     Token Manager utility.

2. Double-click  install.bat
   - Windows asks for Administrator permission -> click Yes.
   - Windows may show "Windows protected your PC" -> click "More info" then
     "Run anyway". (The file just isn't code-signed yet; code-signing removes
     this warning.)
   - The installer trusts the certificate, opens the firewall, sets the bridge
     to auto-start, and starts it. Done.

3. Plug in your DSC token.

4. In the website, open your document and click "Sign with DSC". Enter your
   token PIN once per session.

From now on the bridge is always running in the background — you never start it
by hand.

Signing a document
------------------
1. Plug in your token.
2. Open the document in the site and click "Sign with DSC".
3. Enter your token PIN (asked once per login session).
4. Done — the signed PDF is attached.

Pairing happens automatically the first time you sign — no codes to copy.

To remove
---------
Double-click  uninstall.bat

Troubleshooting
---------------
"Sign with DSC says the bridge isn't found"
    Make sure install.bat finished. You can simply double-click install.bat
    again — it is safe to re-run.

"No token detected"
    Unplug and replug the token. Confirm the vendor's Token Manager sees it.

"PKCS#11 library not found"
    Your token driver is in a non-standard location. Open
    %LOCALAPPDATA%\dsc-bridge\dsc-bridge.json and add the full path to your
    token's PKCS#11 DLL (e.g. eps2003csp11.dll), then re-run install.bat.

Notes
-----
- The bridge listens only on https://127.0.0.1:4645 (your own machine).
- Your token PIN and signing key never leave your computer.

Need help: contact your DSC administrator.
