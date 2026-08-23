package cli

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/msanvido/agentic-pdf/internal/core"
)

// Receive runs a small HTTP server that accepts PDFs printed by the CUPS
// backend (which runs sandboxed and cannot write to disk) and converts them
// into agentic PDFs as the logged-in user.
//
//	POST /print   body: PDF bytes   header: X-Job-Id, X-Job-Title
func Receive(port int, outDir string) error {
	if outDir == "" {
		home, _ := os.UserHomeDir()
		outDir = filepath.Join(home, "Documents", "Agentic-PDF")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/print", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 512<<20))
		if err != nil || len(body) == 0 {
			http.Error(w, "empty body", http.StatusBadRequest)
			return
		}
		title := sanitizeFilename(r.Header.Get("X-Job-Title"))
		if title == "" {
			title = "Untitled"
		}
		// Diagnose payloads that are not PDFs (PostScript, text, …) instead
		// of failing blindly — visible in /tmp/agentic-pdf-receive.log.
		if len(body) < 5 || string(body[:4]) != "%PDF" {
			preview := body
			if len(preview) > 60 {
				preview = preview[:60]
			}
			fmt.Fprintf(os.Stderr,
				"⚠️  job %q: non-PDF payload (%d bytes, %q…)\n",
				r.Header.Get("X-Job-Id"), len(body), preview)
			http.Error(w, "non-PDF payload", http.StatusUnsupportedMediaType)
			return
		}
		out, err := receiveConvert(body, outDir, title)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  job failed: %v\n", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(os.Stderr, "✅ printed → %s\n", out)
		notifyUser("Agentic PDF ready", filepath.Base(out))
		w.Write([]byte("READY"))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Another receiver (or a stale instance) owns the port. Exit cleanly
		// so a KeepAlive supervisor does not crash-loop us.
		fmt.Fprintf(os.Stderr, "receiver: %s already in use — another instance is running; exiting\n", addr)
		os.Exit(0)
	}
	fmt.Printf("📥 receiver listening on %s\n   output: %s\n   inbox:  /tmp/agentic-pdf-inbox\n", addr, outDir)

	// Poll the CUPS-backend inbox: the sandboxed backend drops spooled PDFs
	// there (network is denied inside the sandbox, filesystem writes to /tmp
	// are not).
	const inbox = "/tmp/agentic-pdf-inbox"
	_ = os.MkdirAll(inbox, 0o777)
	go func() {
		for {
			entries, _ := os.ReadDir(inbox)
			for _, e := range entries {
				path := filepath.Join(inbox, e.Name())
				if strings.HasSuffix(path, ".title") {
					continue
				}
				if processInboxFile(path, outDir) {
					os.Remove(path)
					titlePath := strings.TrimSuffix(path, ".pdf") + ".title"
					os.Remove(titlePath)
				}
			}
			time.Sleep(time.Second)
		}
	}()

	return http.Serve(ln, mux)
}

// processInboxFile converts one spooled PDF; returns true when handled.
func processInboxFile(path, outDir string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 5 || string(data[:4]) != "%PDF" {
		return false // not ours / still being written / unreadable
	}
	titleBytes, _ := os.ReadFile(strings.TrimSuffix(path, ".pdf") + ".title")
	title := strings.TrimSpace(string(titleBytes))
	fmt.Fprintf(os.Stderr, "[debug] inbox=%s titleBytes=%d title=%q\n", path, len(titleBytes), title)
	if title == "" {
		// Title sidecar may land a moment after the data file; retry once.
		time.Sleep(400 * time.Millisecond)
		titleBytes, _ = os.ReadFile(strings.TrimSuffix(path, ".pdf") + ".title")
		title = strings.TrimSpace(string(titleBytes))
	}
	if title == "" {
		title = "Printed document"
	}
	pages, err := core.ExtractPages(data)
	if err != nil {
		return false // maybe still being written; retry next tick
	}
	result, err := core.InjectAgentLayer(data, pages, title, "", "", "", true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", filepath.Base(path), err)
		return true // don't loop forever on broken files
	}
	name := fmt.Sprintf("%s (printed).pdf", title)
	out := filepath.Join(outDir, name)
	tmp := out + ".part"
	if err := os.WriteFile(tmp, result, 0o644); err != nil {
		return false
	}
	if err := os.Rename(tmp, out); err != nil {
		return false
	}
	fmt.Fprintf(os.Stderr, "✅ printed → %s\n", out)
	notifyUser("Agentic PDF ready", filepath.Base(out))
	return true
}

var spacesRE = regexp.MustCompile(`\s+`)
var invalidRE = regexp.MustCompile(`[/\\:*?"<>|]`)

func receiveConvert(pdfBytes []byte, outDir, title string) (string, error) {
	pages, err := core.ExtractPages(pdfBytes)
	if err != nil {
		return "", fmt.Errorf("extracting text: %w", err)
	}
	result, err := core.InjectAgentLayer(pdfBytes, pages, title, "", "", "", true)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s (printed).pdf", title)
	out := filepath.Join(outDir, name)
	tmp := out + ".part"
	if err := os.WriteFile(tmp, result, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", err
	}
	return out, nil
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	s = spacesRE.ReplaceAllString(s, " ")
	return invalidRE.ReplaceAllString(s, "_")
}
