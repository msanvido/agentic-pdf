package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// InstallWatch sets up an auto-start entry so the spool-folder watcher runs
// at login without any user action:
//   - Windows: scheduled task ("Microsoft Print to PDF" into the spool folder)
//   - macOS:   launchd agent (complements the CUPS backend)
func InstallWatch(spool, outDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	if spool == "" {
		spool = filepath.Join(home, "Documents", "Agentic-Spool")
	}
	if outDir == "" {
		outDir = filepath.Join(home, "Documents", "Agentic-PDF")
	}
	for _, d := range []string{spool, outDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	switch runtime.GOOS {
	case "windows":
		return installWatchWindows(exe, spool, outDir)
	case "darwin":
		return installWatchMac(exe, spool, outDir)
	default:
		return installWatchLinux(exe, spool, outDir)
	}
}

// UninstallWatch removes the auto-start entry created by InstallWatch.
func UninstallWatch() error {
	switch runtime.GOOS {
	case "windows":
		tryRun("schtasks", "/Delete", "/TN", "AgenticPDF Watcher", "/F")
		fmt.Println("✅ Scheduled task removed.")
	case "darwin":
		plist := plistPath()
		tryRun("launchctl", "unload", plist)
		tryRun("rm", "-f", plist)
		fmt.Println("✅ LaunchAgent removed.")
	default:
		service := filepath.Join(xdgConfigHome(), "systemd", "user", "agentic-pdf-watch.service")
		tryRun("systemctl", "--user", "disable", "--now", "agentic-pdf-watch.service")
		tryRun("rm", "-f", service)
		tryRun("systemctl", "--user", "daemon-reload")
		fmt.Println("✅ systemd user service removed.")
	}
	return nil
}

func installWatchWindows(exe, spool, outDir string) error {
	taskArgs := fmt.Sprintf(`watch %q --out %q`, spool, outDir)
	if err := exec.Command("schtasks", "/Create", "/F",
		"/TN", "AgenticPDF Watcher",
		"/SC", "ONLOGON",
		"/RL", "LIMITED",
		"/TR", `"`+exe+` `+taskArgs+`"`).Run(); err != nil {
		return fmt.Errorf("creating scheduled task (try running as Administrator): %w", err)
	}
	fmt.Printf(`✅ Windows watcher installed.
   1. Print from any app → "Microsoft Print to PDF"
   2. Save into:            %s
   3. Agentic PDFs appear:  %s
The watcher starts at logon and shows a notification when a PDF is ready.
`, spool, outDir)
	return nil
}

const macPlistLabel = "com.agentic-pdf.watch"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", macPlistLabel+".plist")
}

func installWatchMac(exe, spool, outDir string) error {
	plist := plistPath()
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>watch</string>
        <string>%s</string>
        <string>--out</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardErrorPath</key><string>%s/.watch.log</string>
</dict>
</plist>
`, macPlistLabel, exe, spool, spool, outDir)
	tmp := "/tmp/agentic-pdf-watch.plist"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	owner := os.Getenv("SUDO_USER")
	if owner == "" || owner == "root" {
		owner = os.Getenv("USER")
	}
	run("sudo", "cp", tmp, plist)
	if owner != "" && owner != "root" {
		run("sudo", "chown", owner, plist)
	}
	tryRun("launchctl", "unload", plist)
	if err := exec.Command("launchctl", "load", plist).Run(); err != nil {
		return fmt.Errorf("loading LaunchAgent: %w", err)
	}
	fmt.Printf(`✅ macOS watcher installed.
   Spool folder: %s
   Output:       %s
(The CUPS backend remains the primary route; this watcher also converts any
PDF dropped into the spool folder — e.g. from iOS AirPrint or other tools.)
`, spool, outDir)
	return nil
}

func installWatchLinux(exe, spool, outDir string) error {
	dir := filepath.Join(xdgConfigHome(), "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	unit := filepath.Join(dir, "agentic-pdf-watch.service")
	content := fmt.Sprintf(`[Unit]
Description=agentic-pdf watcher (converts dropped PDFs to agentic PDFs)

[Service]
ExecStart=%s watch %q --out %q
Restart=on-failure

[Install]
WantedBy=default.target
`, exe, spool, outDir)
	if err := os.WriteFile(unit, []byte(content), 0o644); err != nil {
		return err
	}
	if err := exec.Command("systemctl", "--user", "enable", "--now", "agentic-pdf-watch.service").Run(); err != nil {
		return fmt.Errorf("enabling service: %w", err)
	}
	fmt.Printf("✅ Linux watcher installed (%s).\n   Spool: %s\n   Out:   %s\n", unit, spool, outDir)
	return nil
}

func xdgConfigHome() string {
	if c := os.Getenv("XDG_CONFIG_HOME"); c != "" {
		return c
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}
