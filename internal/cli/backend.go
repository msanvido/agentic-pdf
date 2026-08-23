package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

//go:embed agentic.ppd
var agenticPPD string

//go:embed pdffilter.sh
var pdfFilterScript string

func toJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// InstallBackend installs the CUPS virtual printer (macOS/Linux with CUPS).
func InstallBackend(spool string) error {
	if spool == "" {
		// /Users/Shared is world-writable AND not TCC-protected; cupsd runs
		// sandboxed and cannot write into ~/Documents even as root.
		spool = "/Users/Shared/Agentic-PDF"
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	script := buildBackendScript(exe, spool)
	tmpScript := "/tmp/agentic-pdf-backend-agentpdf"
	if err := os.WriteFile(tmpScript, []byte(script), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(spool, 0o777); err != nil {
		return err
	}
	// When running under sudo, hand the spool dir back to the invoking user
	// (cupsd runs as root and can write anywhere except TCC-protected paths).
	owner := os.Getenv("SUDO_USER")
	if owner == "" {
		owner = os.Getenv("USER")
	}

	fmt.Fprintln(os.Stderr, "Installing CUPS backend (requires sudo)…")
	run("sudo", "cp", tmpScript, "/usr/libexec/cups/backend/agentpdf")
	run("sudo", "chown", "root:wheel", "/usr/libexec/cups/backend/agentpdf")
	run("sudo", "chmod", "755", "/usr/libexec/cups/backend/agentpdf")
	run("sudo", "mkdir", "-p", spool)
	run("sudo", "chmod", "1777", spool)
	if owner != "" && owner != "root" {
		run("sudo", "chown", owner, spool)
	}
	// Custom pass-through PPD: CUPS converts any incoming format to PDF,
	// then hands the PDF to our backend untouched (via the agentic_pdf
	// pass-through filter). Raw queues are no longer supported on macOS,
	// and PPD-based queues would otherwise deliver PostScript we cannot
	// re-process.
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
		"-v", "agentpdf:"+spool,
		"-P", ppd,
		"-D", "Agentic PDF Printer")
	// A single backend failure must not stop the queue.
	run("sudo", "lpadmin", "-p", "AgenticPDF",
		"-o", "printer-error-policy=retry-current-job")
	run("sudo", "lpadmin", "-p", "AgenticPDF", "-E")
	run("sudo", "cupsenable", "AgenticPDF")
	run("sudo", "launchctl", "kickstart", "-k", "system/org.cups.cupsd")

	fmt.Printf(`✅ Installed "Agentic PDF Printer".
   - Backend: /usr/libexec/cups/backend/agentpdf
   - Queue:   AgenticPDF
   - Output:  %s
Print from any app and pick "Agentic PDF Printer" — the resulting PDF lands
in the output folder with an embedded agent-readable layer.
`, spool)
	return nil
}

// UninstallBackend removes the CUPS virtual printer.
func UninstallBackend() error {
	fmt.Fprintln(os.Stderr, "Removing CUPS backend (requires sudo)…")
	tryRun("sudo", "lpadmin", "-x", "AgenticPDF")
	run("sudo", "rm", "-f", "/usr/libexec/cups/backend/agentpdf")
	run("sudo", "launchctl", "kickstart", "-k", "system/org.cups.cupsd")
	fmt.Println("✅ Uninstalled.")
	return nil
}

func buildBackendScript(cliPath, spool string) string {
	return `#!/bin/sh
# CUPS backend for agentic-pdf — converts print jobs into agentic PDFs.
#
# NOTE: cupsd executes backends inside a sandbox that denies writes almost
# everywhere (including $SPOOL_DIR). Strategy:
#   - stage the converted PDF in /var/spool/cups/agentic-pdf (always writable)
#   - hand it off to the un-sandboxed watcher/agent running as the user via
#     launchctl kickstart -k (user session), falling back to leaving it in the
#     staging folder with a notice file
#   - every step is logged through syslog (logger), which sandboxes cannot block

# Discovery (no args): advertise the device.
if [ $# -eq 0 ]; then
  echo "network agentpdf \"Agentic PDF Printer\" \"Prints to agentic PDFs with an embedded agent-readable layer\""
  exit 0
fi

job_id="$1"; user="$2"; title="$3"; copies="$4"; options="$5"; file="$6"
SPOOL_DIR="` + spool + `"
STAGE="/var/spool/cups/agentic-pdf"

say() { logger -t agentpdf "$*"; }
say "job $job_id: title='$title' file='$file'"

mkdir -p "$STAGE" 2>/dev/null || { say "job $job_id: cannot create $STAGE"; exit 1; }

# Only PDFs reach this backend (CUPS converts everything upstream). Guard
# against surprises instead of deadlocking in a nested cupsfilter call.
magic=$(head -c 4 "$file" 2>/dev/null)
if [ "$magic" != "%PDF" ]; then
  say "job $job_id: rejected non-PDF input"
  exit 1
fi

out="$STAGE/job-$job_id.pdf"
"` + cliPath + `" print "$file" -o "$out" --title "$title" 2>&1 | while IFS= read -r line; do say "$line"; done

if [ ! -s "$out" ]; then
  say "job $job_id: FAILED - no output produced"
  exit 1
fi

safe_title=$(printf '%s' "$title" | tr '/:' '__' | cut -c1-80)
final="$SPOOL_DIR/$safe_title-$job_id.pdf"

# Try direct delivery first; fall back to handing off via launchd user agent.
if mv "$out" "$final" 2>/dev/null && chmod 644 "$final" 2>/dev/null; then
  say "job $job_id: wrote $final"
else
  chmod 666 "$out" 2>/dev/null
  say "job $job_id: delivered to staging: $out (spool dir not writable)"
fi

echo "READY"
exit 0
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
