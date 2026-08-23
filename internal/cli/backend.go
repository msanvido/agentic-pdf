package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

//go:embed agentic.ppd
var agenticPPD string

//go:embed pdffilter.sh
var pdfFilterScript string

// DefaultReceiverPort is the localhost port the CUPS backend hands printed
// PDFs to; the user-space receiver (launchd agent) listens here.
const DefaultReceiverPort = 47631

func toJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// InstallBackend installs the CUPS virtual printer (macOS/Linux with CUPS).
// InstallBackend installs the CUPS virtual printer. Architecture on macOS:
//
//	app --print--> cupsd (sandboxed) --> backend script
//	                                   | curl POST localhost:<port>
//	                                   v
//	receiver ("agentic-pdf receive", user LaunchAgent) --convert--> output dir
//
// Everything after the sandbox boundary runs as the logged-in user.
func InstallBackend(spool string) error {
	if spool == "" {
		home, _ := os.UserHomeDir()
		spool = filepath.Join(home, "Documents", "Agentic-PDF")
	}
	if err := os.MkdirAll(spool, 0o755); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	port := DefaultReceiverPort

	fmt.Fprintln(os.Stderr, "Installing CUPS backend (requires sudo)…")

	// 1. User-space receiver as a LaunchAgent, started immediately.
	if runtime.GOOS == "darwin" {
		if err := installReceiveAgent(exe, port, spool); err != nil {
			return err
		}
	}

	// 2. Backend + filter + queue (root-owned locations).
	script := buildBackendScript(exe, port)
	tmpScript := "/tmp/agentic-pdf-backend-agentpdf"
	if err := os.WriteFile(tmpScript, []byte(script), 0o755); err != nil {
		return err
	}
	run("sudo", "cp", tmpScript, "/usr/libexec/cups/backend/agentpdf")
	run("sudo", "chown", "root:wheel", "/usr/libexec/cups/backend/agentpdf")
	run("sudo", "chmod", "755", "/usr/libexec/cups/backend/agentpdf")

	ppd := "/tmp/agentic-pdf.ppd"
	if err := os.WriteFile(ppd, []byte(agenticPPD), 0o644); err != nil {
		return err
	}
	pdff := "/tmp/agentic-pdf-filter"
	if err := os.WriteFile(pdff, []byte(pdfFilterScript), 0o755); err != nil {
		return err
	}
	run("sudo", "cp", pdff, "/usr/libexec/cups/filter/agentic_pdf")
	run("sudo", "chown", "root:wheel", "/usr/libexec/cups/filter/agentic_pdf")
	run("sudo", "chmod", "755", "/usr/libexec/cups/filter/agentic_pdf")
	run("sudo", "lpadmin", "-p", "AgenticPDF",
		"-v", fmt.Sprintf("agentpdf://127.0.0.1:%d", port),
		"-P", ppd,
		"-D", "Agentic PDF Printer")
	// A single backend failure must not stop the queue.
	run("sudo", "lpadmin", "-p", "AgenticPDF",
		"-o", "printer-error-policy=retry-current-job")
	run("sudo", "lpadmin", "-p", "AgenticPDF", "-E")
	run("sudo", "cupsenable", "AgenticPDF")
	run("sudo", "launchctl", "kickstart", "-k", "system/org.cups.cupsd")

	fmt.Printf(`✅ Installed "Agentic PDF Printer".
   - Queue:   AgenticPDF (available in every Print dialog)
   - Output:  %s
   - Receiver: user LaunchAgent on 127.0.0.1:%d
Print from any app and pick "Agentic PDF Printer" — the resulting PDF lands
in the output folder with an embedded agent-readable layer.
`, spool, port)
	return nil
}

func installReceiveAgent(exe string, port int, spool string) error {
	plist := receivePlistPath()
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>receive</string>
        <string>--port</string>
        <string>%d</string>
        <string>--out</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
    <key>StandardErrorPath</key><string>/tmp/agentic-pdf-receive.log</string>
</dict>
</plist>
`, receiveLabel, exe, port, spool)
	if err := os.WriteFile(plist, []byte(content), 0o644); err != nil {
		return err
	}
	tryRun("launchctl", "unload", plist)
	if err := exec.Command("launchctl", "load", plist).Run(); err != nil {
		return fmt.Errorf("loading receiver LaunchAgent: %w", err)
	}
	return nil
}

const receiveLabel = "com.agentic-pdf.receive"

func receivePlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", receiveLabel+".plist")
}

func UninstallBackend() error {
	fmt.Fprintln(os.Stderr, "Removing CUPS backend (requires sudo)…")
	tryRun("sudo", "lpadmin", "-x", "AgenticPDF")
	run("sudo", "rm", "-f", "/usr/libexec/cups/backend/agentpdf")
	run("sudo", "launchctl", "kickstart", "-k", "system/org.cups.cupsd")
	fmt.Println("✅ Uninstalled.")
	return nil
}

func buildBackendScript(cliPath string, port int) string {
	return `#!/bin/sh
# CUPS backend for agentic-pdf.
#
# cupsd runs backends inside a sandbox that denies nearly all filesystem
# writes, so this script simply hands the (already PDF) spool to the
# user-space receiver over localhost HTTP. The receiver does the actual
# conversion and delivery with normal user permissions.

# Discovery (no args): advertise the device.
if [ $# -eq 0 ]; then
  echo "network agentpdf \"Agentic PDF Printer\" \"Prints to agentic PDFs with an embedded agent-readable layer\""
  exit 0
fi

job_id="$1"; user="$2"; title="$3"; copies="$4"; options="$5"; file="$6"

say() {
  echo "agentpdf: $*" >&2          # -> /var/log/cups/error_log
  logger -t agentpdf "$*" 2>/dev/null
}

PORT=` + strconv.Itoa(port) + `
URL="http://127.0.0.1:$PORT/print"

# Read job data (stdin) or spool file into a temp buffer in /tmp — one of
# the few writable locations inside the backend sandbox.
tmp_in=$(mktemp /tmp/agentpdf.XXXXXX)
if [ -n "$file" ] && [ -r "$file" ]; then
  cat "$file" > "$tmp_in"
else
  cat > "$tmp_in"
fi

# Forward everything to the receiver; it does validation and logging.
rc=$(curl -s --max-time 120 \
      -H "X-Job-Id: $job_id" \
      -H "X-Job-Title: $title" \
      --data-binary @"$tmp_in" \
      -o /dev/null -w "%{http_code}" \
      "$URL")
rm -f "$tmp_in"

if [ "$rc" = "200" ]; then
  say "job $job_id: delivered via receiver"
  echo "READY"
  exit 0
fi
say "job $job_id: receiver error HTTP $rc (is 'agentic-pdf receive' running?)"
exit 1
`
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		die(fmt.Sprintf("%s failed: %v", name, err))
	}
}

func tryRun(name string, args ...string) {
	_ = exec.Command(name, args...).Run()
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	os.Exit(1)
}

// OpenBrowser opens url in the default browser.
func OpenBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		tryRun("open", url)
	case "windows":
		tryRun("cmd", "/c", "start", strings.ReplaceAll(url, "&", "^&"))
	default:
		tryRun("xdg-open", url)
	}
}
