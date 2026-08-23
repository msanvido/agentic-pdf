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
		outDir = core.DefaultOutDir()
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
	// as /tmp/agentpdf.<jobid>.<title>.XXXXXX.pdf (mktemp-style creation is
	// the only write primitive the sandbox allows; network is denied).
	go func() {
		for {
			matches, _ := filepath.Glob("/tmp/agentpdf.*.pdf")
			for _, path := range matches {
				claimed := path + ".claim"
				if err := os.Rename(path, claimed); err != nil {
					continue // another receiver claimed it
				}
				processInboxFile(claimed, outDir)
				os.Remove(claimed)
			}
			time.Sleep(time.Second)
		}
	}()

	return http.Serve(ln, mux)
}

// processInboxFile converts one spooled PDF; returns true when handled.
// Title is embedded in the filename: agentpdf.<jobid>.<title>.<rand>.pdf
func processInboxFile(path, outDir string) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 5 || string(data[:4]) != "%PDF" {
		return // not ours / unreadable
	}
	base := filepath.Base(path)
	title := "Printed document"
	// agentpdf.<jobid>.<title...>.<6-10 rand>.pdf
	// claimed files carry an extra ".claim" suffix; drop it plus ".pdf",
	// leaving agentpdf.<jobid>.<title...>.<rand>
	rest := strings.TrimSuffix(strings.TrimSuffix(base, ".claim"), ".pdf")
	rest = strings.TrimPrefix(rest, "agentpdf.")
	parts := strings.Split(rest, ".")
	if len(parts) >= 3 {
		title = strings.Join(parts[1:len(parts)-1], ".") // drop jobid + rand
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Printed document"
	}
	pages, err := core.ExtractPages(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", filepath.Base(path), err)
		return
	}
	result, err := core.InjectAgentLayer(data, pages, title, "", "", "", true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", filepath.Base(path), err)
		return
	}
	name := fmt.Sprintf("%s (printed).pdf", title)
	out := filepath.Join(outDir, name)
	tmp := out + ".part"
	if err := os.WriteFile(tmp, result, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, out); err != nil {
		return
	}
	fmt.Fprintf(os.Stderr, "✅ printed → %s\n", out)
	notifyUser("Agentic PDF ready", filepath.Base(out))
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
