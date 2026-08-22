package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/msanvido/agentic-pdf/internal/core"
)

// Watch monitors a spool directory for PDFs (typically produced by
// "Microsoft Print to PDF" on Windows, or any drop into the folder) and
// converts each one into an agentic PDF in the output directory.
func Watch(spoolDir, outDir string, deleteOriginal bool) error {
	if spoolDir == "" {
		return fmt.Errorf("watch: missing spool directory")
	}
	spoolAbs, err := filepath.Abs(spoolDir)
	if err != nil {
		return err
	}
	if outDir == "" {
		outDir = spoolAbs + ".agentic"
	}
	outAbs, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	for _, d := range []string{spoolAbs, outAbs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("watch: creating %s: %w", d, err)
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	if err := watcher.Add(spoolAbs); err != nil {
		return fmt.Errorf("watch: %w", err)
	}

	fmt.Printf("👀 watching %s\n   output: %s\n   Ctrl-C to stop\n", spoolAbs, outAbs)

	process := func(name string) {
		path := filepath.Join(spoolAbs, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || filepath.Ext(name) != ".pdf" || isHidden(name) {
			return
		}
		if err := convertSpooledFile(path, outAbs); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %s: %v\n", name, err)
			return
		}
		if deleteOriginal {
			os.Remove(path)
		}
	}

	// Quiescence loop: only convert once a file has been stable for a moment,
	// so partially-written spool files are not processed.
	stableWait := 700 * time.Millisecond
	timers := map[string]*time.Timer{}

	settle := func(name string) {
		if t, ok := timers[name]; ok {
			t.Reset(stableWait)
			return
		}
		timers[name] = time.AfterFunc(stableWait, func() {
			delete(timers, name)
			process(name)
		})
	}

	for {
		select {
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			name := filepath.Base(ev.Name)
			switch {
			case ev.Has(fsnotify.Create), ev.Has(fsnotify.Write), ev.Has(fsnotify.Rename):
				settle(name)
			case ev.Has(fsnotify.Remove):
				if t, ok := timers[name]; ok {
					t.Stop()
					delete(timers, name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "watcher error: %v\n", err)
		}
	}
}

func convertSpooledFile(path, outAbs string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	base := filepath.Base(path)
	outPath := filepath.Join(outAbs, base[:len(base)-len(".pdf")]+" (agentic).pdf")

	// Already an agentic file? Leave it untouched.
	if atts, err := readAtts(data); err == nil && hasLayerAtt(atts) {
		fmt.Fprintf(os.Stderr, "⏭  %s already carries an agent layer\n", base)
		return nil
	}

	pages, err := core.ExtractPages(data)
	if err != nil {
		return fmt.Errorf("extracting text: %w", err)
	}
	result, err := core.InjectAgentLayer(data, pages, "", "", "", "", true)
	if err != nil {
		return err
	}
	tmp := outPath + ".part"
	if err := os.WriteFile(tmp, result, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, outPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "✅ %s → %s (%d page(s))\n", base, filepath.Base(outPath), len(pages))
	notifyUser("Agentic PDF ready", filepath.Base(outPath))
	return nil
}

// hasLayerAtt reports whether an attachment list contains the agent layer.
func hasLayerAtt(atts []core.Attachment) bool {
	for _, a := range atts {
		if a.Name == core.AgentMD || a.Name == core.AgentHTML {
			return true
		}
	}
	return false
}

func readAtts(data []byte) ([]core.Attachment, error) {
	return core.ReadAttachments(data)
}

func isHidden(name string) bool {
	return len(name) > 0 && name[0] == '.'
}
