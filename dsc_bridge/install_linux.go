//go:build linux

package main

// Post-install / pre-uninstall hooks and per-user trust setup for Linux.
//
// Design mirrors the Windows (registry Run key) and macOS (LaunchAgent) files,
// but adapted to Linux conventions:
//
//   - Autostart: an XDG autostart .desktop entry. The bridge is a system-tray
//     GUI agent, so — exactly like on Windows/macOS — it must run *inside* the
//     graphical session (where a tray and the zenity pairing dialog exist), not
//     as a background systemd service in a session with no display. A .desktop
//     under /etc/xdg/autostart (system-wide, from the root .deb install) or
//     ~/.config/autostart (per-user, from the tarball install) is the correct
//     analog. The Exec line sets DSC_BRIDGE_AUTO_CONFIRM_PAIRING=1 so first-time
//     pairing needs no click — remove it for stricter, consent-on-pair security.
//
//   - Certificate trust is split in two because Linux has multiple trust stores:
//       * System CA store (update-ca-certificates / update-ca-trust) — needs
//         root, done at install time. Firefox picks this up via an enterprise
//         policy (ImportEnterpriseRoots) we also drop.
//       * Per-user NSS DBs — Chrome/Chromium (~/.pki/nssdb) and each Firefox
//         profile keep their OWN cert store and ignore the system one. A root
//         package install can't reach a user's home, so EnsureUserTrust() runs
//         on every normal bridge startup (as the user) and idempotently adds the
//         cert to that user's NSS DBs. That closes the gap for the .deb case and
//         self-heals if a browser profile is created later.

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	linuxCertNick      = "DSC Bridge"
	autostartFileName  = "dsc-bridge.desktop"
	debCASource        = "/usr/local/share/ca-certificates/dsc-bridge.crt"
	rhCASource         = "/etc/pki/ca-trust/source/anchors/dsc-bridge.crt"
	autoConfirmPairEnv = "DSC_BRIDGE_AUTO_CONFIRM_PAIRING"
)

// firefoxSystemPolicyDirs are the locations Firefox reads managed policy from.
// We write policies.json to whichever parent (firefox install / /etc) exists so
// Firefox trusts the system CA store via ImportEnterpriseRoots.
var firefoxSystemPolicyDirs = []string{
	"/etc/firefox/policies",
	"/usr/lib/firefox/distribution",
	"/usr/lib64/firefox/distribution",
	"/usr/lib/firefox-esr/distribution",
}

// PostInstall wires up trust + autostart. Invoked as root from the .deb
// postinst, or as the user from the tarball install.sh. Idempotent, and every
// step is best-effort: a missing tool logs a warning but never aborts the rest.
func PostInstall() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Generate the TLS cert first so we have something to trust. No-op if it
	// already exists.
	if _, _, err := EnsureTLSCert(cfg); err != nil {
		return fmt.Errorf("generate cert: %w", err)
	}

	// System-wide trust (root only). Firefox picks it up via the policy file.
	if os.Geteuid() == 0 {
		if err := installCertSystemStore(cfg.TLSCertPath); err != nil {
			log.Printf("system-trust: %v (browsers may warn until the cert is trusted)", err)
		}
		if err := installFirefoxSystemPolicy(); err != nil {
			log.Printf("firefox-policy: %v (Firefox may warn until trusted manually)", err)
		}
	} else {
		// Running as the user (tarball install) — do their per-user browser
		// trust right now instead of waiting for the first startup.
		EnsureUserTrust()
	}

	if err := installAutostart(); err != nil {
		log.Printf("autostart: %v (bridge will not start automatically on login)", err)
	}

	log.Println("post-install: complete")
	return nil
}

// PreUninstall reverses PostInstall. Best-effort throughout.
func PreUninstall() error {
	if err := removeAutostart(); err != nil {
		log.Printf("autostart-remove: %v", err)
	}

	if os.Geteuid() == 0 {
		if err := removeCertSystemStore(); err != nil {
			log.Printf("system-trust-remove: %v", err)
		}
		if err := removeFirefoxSystemPolicy(); err != nil {
			log.Printf("firefox-policy-remove: %v", err)
		}
	} else {
		removeUserTrust()
	}

	log.Println("pre-uninstall: complete")
	return nil
}

// EnsureUserTrust adds the bridge cert to the invoking user's browser NSS DBs
// (Chrome/Chromium + Firefox profiles) if not already present. Called on every
// normal startup so a root package install still ends up trusted per-user, and
// so trust self-heals when a new browser profile appears. No-op and cheap when
// everything is already trusted. Never fails the caller — logs only.
func EnsureUserTrust() {
	cfg, err := LoadConfig()
	if err != nil {
		return
	}
	certPath := cfg.TLSCertPath
	if _, err := os.Stat(certPath); err != nil {
		return // cert not generated yet; nothing to trust
	}
	if _, err := exec.LookPath("certutil"); err != nil {
		log.Printf("user-trust: certutil not found — install libnss3-tools for automatic browser trust")
		return
	}

	for _, db := range userNSSDatabases() {
		if err := nssAddCert(db, certPath); err != nil {
			log.Printf("user-trust: %s: %v", db, err)
		}
	}
}

func removeUserTrust() {
	if _, err := exec.LookPath("certutil"); err != nil {
		return
	}
	for _, db := range userNSSDatabases() {
		_ = exec.Command("certutil", "-D", "-d", "sql:"+db, "-n", linuxCertNick).Run()
	}
}

// userNSSDatabases returns the NSS DB directories for the current user's
// browsers that actually exist: Chrome/Chromium's shared ~/.pki/nssdb and every
// Firefox profile (native and Snap). ~/.pki/nssdb is created if missing so
// Chrome trust can be pre-seeded before Chrome's first run.
func userNSSDatabases() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var dbs []string

	// Chrome / Chromium — shared NSS DB. Create + initialise if absent.
	chromeDB := filepath.Join(home, ".pki", "nssdb")
	if ensureNSSDB(chromeDB) {
		dbs = append(dbs, chromeDB)
	}

	// Firefox — one cert9.db per profile. Cover native and Snap layouts.
	profileGlobs := []string{
		filepath.Join(home, ".mozilla", "firefox", "*"),
		filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox", "*"),
	}
	for _, g := range profileGlobs {
		matches, _ := filepath.Glob(g)
		for _, p := range matches {
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				if _, err := os.Stat(filepath.Join(p, "cert9.db")); err == nil {
					dbs = append(dbs, p)
				}
			}
		}
	}
	return dbs
}

// ensureNSSDB makes sure an sql: NSS DB exists at dir, creating an empty one if
// needed. Returns false if it cannot be created.
func ensureNSSDB(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "cert9.db")); err == nil {
		return true
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	// Initialise an empty, password-less DB. Ignore error if it already exists.
	_ = exec.Command("certutil", "-N", "-d", "sql:"+dir, "--empty-password").Run()
	_, err := os.Stat(filepath.Join(dir, "cert9.db"))
	return err == nil
}

// nssAddCert adds the cert to an NSS DB as a trusted root, unless already there.
func nssAddCert(db, certPath string) error {
	// Skip if a cert with our nickname is already present (idempotent).
	if exec.Command("certutil", "-L", "-d", "sql:"+db, "-n", linuxCertNick).Run() == nil {
		return nil
	}
	// -t "C,," → trusted CA for TLS server auth only.
	out, err := exec.Command("certutil", "-A",
		"-d", "sql:"+db,
		"-n", linuxCertNick,
		"-t", "C,,",
		"-i", certPath,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil -A: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// installCertSystemStore installs the cert into the OS CA store using whichever
// tool the distro ships (Debian's update-ca-certificates or RH's
// update-ca-trust). Root only.
func installCertSystemStore(certPath string) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read cert: %w", err)
	}

	if _, err := exec.LookPath("update-ca-certificates"); err == nil {
		if err := os.MkdirAll(filepath.Dir(debCASource), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(debCASource, data, 0o644); err != nil {
			return err
		}
		out, err := exec.Command("update-ca-certificates").CombinedOutput()
		if err != nil {
			return fmt.Errorf("update-ca-certificates: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	if _, err := exec.LookPath("update-ca-trust"); err == nil {
		if err := os.MkdirAll(filepath.Dir(rhCASource), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(rhCASource, data, 0o644); err != nil {
			return err
		}
		out, err := exec.Command("update-ca-trust", "extract").CombinedOutput()
		if err != nil {
			return fmt.Errorf("update-ca-trust: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	return fmt.Errorf("no supported CA tool (update-ca-certificates / update-ca-trust) found")
}

func removeCertSystemStore() error {
	removed := false
	for _, p := range []string{debCASource, rhCASource} {
		if err := os.Remove(p); err == nil {
			removed = true
		}
	}
	if !removed {
		return nil
	}
	if _, err := exec.LookPath("update-ca-certificates"); err == nil {
		return exec.Command("update-ca-certificates", "--fresh").Run()
	}
	if _, err := exec.LookPath("update-ca-trust"); err == nil {
		return exec.Command("update-ca-trust", "extract").Run()
	}
	return nil
}

// installFirefoxSystemPolicy drops a policies.json that tells Firefox to trust
// the OS CA store (which now contains our cert). Written to whichever policy
// directory's parent exists.
func installFirefoxSystemPolicy() error {
	const policy = `{
  "policies": {
    "Certificates": {
      "ImportEnterpriseRoots": true
    }
  }
}
`
	wrote := false
	for _, dir := range firefoxSystemPolicyDirs {
		parent := filepath.Dir(dir)
		if _, err := os.Stat(parent); err != nil {
			continue // that Firefox flavour isn't installed
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, "policies.json"), []byte(policy), 0o644); err == nil {
			wrote = true
		}
	}
	if !wrote {
		return fmt.Errorf("no Firefox policy directory available (Firefox may not be installed)")
	}
	return nil
}

func removeFirefoxSystemPolicy() error {
	for _, dir := range firefoxSystemPolicyDirs {
		_ = os.Remove(filepath.Join(dir, "policies.json"))
	}
	return nil
}

// installAutostart writes the XDG autostart .desktop entry so the bridge starts
// on login. System-wide when root, per-user otherwise.
func installAutostart() error {
	exe, err := exeFullPath()
	if err != nil {
		return err
	}

	dir, err := autostartDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// `env DSC_BRIDGE_AUTO_CONFIRM_PAIRING=1 <exe>` → first-time pairing is
	// approved without a dialog. Delete the env prefix for consent-on-pair.
	entry := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=DSC Bridge
Comment=Digital Signature Certificate signing agent
Exec=env %s=1 %s
Icon=application-certificate
Terminal=false
X-GNOME-Autostart-enabled=true
`, autoConfirmPairEnv, exe)

	return os.WriteFile(filepath.Join(dir, autostartFileName), []byte(entry), 0o644)
}

func removeAutostart() error {
	dir, err := autostartDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, autostartFileName))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func autostartDir() (string, error) {
	if os.Geteuid() == 0 {
		return "/etc/xdg/autostart", nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "autostart"), nil
}

func exeFullPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Clean(exe), nil
}
