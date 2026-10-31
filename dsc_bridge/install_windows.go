//go:build windows

package main

// Post-install / pre-uninstall hooks invoked by the MSI installer.
//
// The bridge is a system-tray GUI agent, NOT a Windows Service.
// Services run in Session 0 with no desktop access, so a systray app
// inside a service produces no visible icon and often hangs at startup.
//
// Instead we use the standard mechanism for per-user GUI agents:
//   - Auto-start: HKLM\...\Run registry key for all users
//   - Firewall: netsh rule for inbound TCP 4645 on the bridge binary
//   - Trust: certutil adds the bridge's self-signed cert to the local
//            machine's Trusted Root store so the browser stops warning.

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath      = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	runKeyValueName = "DSC Bridge"
	firewallRuleNm  = "DSC Bridge Agent"
	certStoreName   = "Root"
)

// PostInstall is invoked once by the MSI immediately after files are placed.
// It must be idempotent — re-running it should not produce duplicate firewall
// rules, duplicate trust-store entries, or extra registry values.
func PostInstall() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 1. Generate the TLS cert now so we can immediately install it to the
	//    trust store. EnsureTLSCert is a no-op if cert already exists.
	if _, _, err := EnsureTLSCert(cfg); err != nil {
		return fmt.Errorf("generate cert: %w", err)
	}

	if err := installCertToTrustStore(cfg.TLSCertPath); err != nil {
		log.Printf("install-cert: %v (continuing; browser may warn until trusted manually)", err)
	}

	exe, err := exeFullPath()
	if err != nil {
		return fmt.Errorf("resolve exe path: %w", err)
	}

	if err := addFirewallRule(exe); err != nil {
		log.Printf("firewall-rule: %v (continuing; signers may hit a Windows Defender prompt)", err)
	}

	if err := addAutoStartEntry(exe); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}

	// Additive hardening — the HKLM\Run entry above is still the guaranteed
	// autostart; both steps below are best-effort and never block the install,
	// so an older machine (no PowerShell/Defender) behaves exactly as before.
	//
	//  1. Self-healing watchdog: HKLM\Run only launches the bridge once, at
	//     login, with no restart if it later dies (crash, forced close, or an
	//     antivirus kill). The watchdog task re-checks every 5 minutes and
	//     relaunches the bridge only when it is NOT already running — so it can
	//     never spawn a duplicate or fight over port 4645.
	//  2. Defender exclusion: the exe is unsigned, so a routine AV scan can
	//     quarantine it and leave HKLM\Run pointing at a deleted file — the most
	//     common reason the bridge "vanishes" and users are told to reinstall.
	if err := installWatchdog(exe); err != nil {
		log.Printf("watchdog: %v (continuing; bridge still auto-starts at login via Run)", err)
	}
	if err := addDefenderExclusion(filepath.Dir(exe)); err != nil {
		log.Printf("defender-exclusion: %v (continuing; add it manually if AV quarantines the bridge)", err)
	}

	log.Println("post-install: complete")
	return nil
}

// PreUninstall reverses the steps from PostInstall. Best-effort: each step
// logs but does not abort, so a partial state never blocks uninstall.
func PreUninstall() error {
	if err := removeAutoStartEntry(); err != nil {
		log.Printf("autostart-remove: %v", err)
	}
	if err := removeFirewallRule(); err != nil {
		log.Printf("firewall-remove: %v", err)
	}
	if err := removeWatchdog(); err != nil {
		log.Printf("watchdog-remove: %v", err)
	}

	cfg, err := LoadConfig()
	if err == nil {
		if err := removeCertFromTrustStore(cfg.TLSCertPath); err != nil {
			log.Printf("cert-remove: %v", err)
		}
		if err := removeDefenderExclusion(cfg.DataDir); err != nil {
			log.Printf("defender-exclusion-remove: %v", err)
		}
	}

	log.Println("pre-uninstall: complete")
	return nil
}

func installCertToTrustStore(certPath string) error {
	// `-f` overwrites an existing matching entry so re-install is idempotent.
	cmd := exec.Command("certutil", "-f", "-addstore", certStoreName, certPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil addstore: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeCertFromTrustStore(certPath string) error {
	// certutil -delstore needs the cert's SHA-1 hash or subject CN. We use
	// the CN set in generateSelfSignedCert (tls.go).
	cmd := exec.Command("certutil", "-delstore", certStoreName, "dsc-bridge")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil delstore: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func addFirewallRule(exePath string) error {
	// `netsh advfirewall firewall add rule` errors if a rule with the same
	// name already exists; we remove it first to keep the operation idempotent.
	_ = exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
		"name="+firewallRuleNm).Run()

	cmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+firewallRuleNm,
		"dir=in",
		"action=allow",
		"program="+exePath,
		"protocol=TCP",
		"localport=4645",
		"enable=yes",
		"profile=any",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh add rule: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeFirewallRule() error {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule",
		"name="+firewallRuleNm)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh delete rule: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func addAutoStartEntry(exePath string) error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open HKLM\\Run: %w", err)
	}
	defer key.Close()
	// Quote the path so spaces in "Program Files" do not split the command.
	return key.SetStringValue(runKeyValueName, `"`+exePath+`"`)
}

func removeAutoStartEntry() error {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open HKLM\\Run: %w", err)
	}
	defer key.Close()
	if err := key.DeleteValue(runKeyValueName); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("delete value: %w", err)
	}
	return nil
}

func exeFullPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Clean(exe), nil
}

const watchdogTaskName = "DSC Bridge Watchdog"

// psSingleQuote escapes a value for safe use inside a PowerShell single-quoted
// literal (the only metacharacter there is the single quote itself, doubled).
func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// installWatchdog registers a self-healing Scheduled Task that relaunches the
// bridge whenever it isn't running. It is purely additive to the HKLM\Run
// autostart: Run still starts the bridge at login; this is the safety net that
// recovers from a crash, a forced close, or an antivirus kill mid-session.
//
// The task's action checks for a running dsc-bridge before starting one, so it
// can never create a second instance (and therefore never collide on port
// 4645). The checking process exits immediately, so the task itself is never
// long-running — no execution-time-limit or duplicate-instance concerns.
//
// Everything is driven from two .ps1 files written next to the exe and run with
// -File, so there is no fragile command-line quoting to get wrong.
func installWatchdog(exePath string) error {
	dir := filepath.Dir(exePath)
	watchdogPath := filepath.Join(dir, "dsc-bridge-watchdog.ps1")
	setupPath := filepath.Join(dir, "dsc-bridge-install-watchdog.ps1")

	// Recurring check: start the bridge only if it is not already running.
	watchdog := "if (-not (Get-Process -Name dsc-bridge -ErrorAction SilentlyContinue)) " +
		"{ Start-Process -FilePath '" + psSingleQuote(exePath) + "' -WindowStyle Hidden }\r\n"
	if err := os.WriteFile(watchdogPath, []byte(watchdog), 0644); err != nil {
		return fmt.Errorf("write watchdog script: %w", err)
	}

	// One-time registration. Runs under the interactive user so Start-Process
	// lands the systray UI in their session; LeastPrivilege (no admin needed to
	// run); fires 5 min after logon and repeats every 5 min indefinitely.
	setup := "$ErrorActionPreference = 'Stop'\r\n" +
		"$wd = '" + psSingleQuote(watchdogPath) + "'\r\n" +
		"$action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument \"-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File `\"$wd`\"\"\r\n" +
		"$trigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 5) -RepetitionDuration (New-TimeSpan -Days 3650)\r\n" +
		"$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew\r\n" +
		"$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited\r\n" +
		"Register-ScheduledTask -TaskName '" + watchdogTaskName + "' -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Force | Out-Null\r\n"
	if err := os.WriteFile(setupPath, []byte(setup), 0644); err != nil {
		return fmt.Errorf("write watchdog setup script: %w", err)
	}

	out, err := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", setupPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("register watchdog task: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeWatchdog deletes the watchdog Scheduled Task. Best-effort.
func removeWatchdog() error {
	out, err := exec.Command("schtasks", "/delete", "/tn", watchdogTaskName, "/f").CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete watchdog task: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// addDefenderExclusion tells Windows Defender to stop scanning the bridge's
// folder, so it stops quarantining the (unsigned) exe. Needs admin, which
// install.bat already holds; a no-op when Defender is absent or managed by GPO.
func addDefenderExclusion(dir string) error {
	cmd := "Add-MpPreference -ExclusionPath '" + psSingleQuote(dir) + "'"
	out, err := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("add defender exclusion: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeDefenderExclusion reverses addDefenderExclusion. Best-effort.
func removeDefenderExclusion(dir string) error {
	cmd := "Remove-MpPreference -ExclusionPath '" + psSingleQuote(dir) + "'"
	out, err := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove defender exclusion: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// EnsureUserTrust is a no-op on Windows: the cert is added to the machine
// Trusted Root store at MSI time, which every user and browser honours. Only
// Linux needs per-user, per-startup NSS trust.
func EnsureUserTrust() {}
