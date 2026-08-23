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
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	fmt.Printf("📥 receiver listening on %s\n   output: %s\n", addr, outDir)
	return http.Serve(ln, mux)
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
