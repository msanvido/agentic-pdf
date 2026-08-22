package viewer

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"

	"github.com/msanvido/agentic-pdf/internal/core"
)

//go:embed viewer.html
var indexHTML []byte

// Serve hosts a local viewer for the given PDF.
// The UI is fully client-side: pdf.js renders the pages and extracts the
// embedded agent layer in the browser; the server only serves bytes.
func Serve(pdfPath string, port int, openBrowser bool) error {
	pdfPath, err := filepath.Abs(pdfPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/doc.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Link", `</doc.pdf>; rel="canonical"`)
		_, _ = w.Write(data)
	})

	addr := fmt.Sprintf("http://localhost:%d/", port)
	hasLayer := hasAgentLayer(data)
	fmt.Printf("👁  viewing %s\n", pdfPath)
	fmt.Printf("   viewer: %s\n", addr)
	if hasLayer {
		fmt.Println("   agentic layer detected ✔")
	} else {
		fmt.Fprintln(os.Stderr, "   ⚠ no agentic layer in this PDF (showing visual content only)")
	}
	fmt.Println("   Ctrl-C to stop")

	server := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", port), Handler: mux}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		<-sig
		_ = server.Close()
		os.Exit(0)
	}()

	if openBrowser {
		openURL(addr + "?file=doc.pdf")
	}
	return server.ListenAndServe()
}

func openURL(url string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Start()
	case "windows":
		_ = exec.Command("cmd", "/c", "start", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}

func hasAgentLayer(pdfBytes []byte) bool {
	atts, err := core.ReadAttachments(pdfBytes)
	if err != nil {
		return false
	}
	for _, a := range atts {
		if a.Name == core.AgentMD || a.Name == core.AgentHTML {
			return true
		}
	}
	return false
}
