package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/msanvido/agentic-pdf/internal/cli"
	"github.com/msanvido/agentic-pdf/internal/viewer"
)

var version = "0.2.0"

func help() string {
	return `agentic-pdf v` + version + ` — PDF printer driver + viewer with a hidden agent-readable layer

Usage:
  agentic-pdf agentify <input> [-o out.pdf] [--title "T"] [--author "A"] [--canonical URL] [--no-html]
      Extract everything automatically (text, tables, figures, metadata)
      using the best available extraction tools, and embed the agent layer.
      Works on PDFs and any cupsfilter-convertible input.

  agentic-pdf print <input.pdf> --md agent.md [--html agent.html]
      [--attach file ...] [-o out.pdf] [--title "T"] [--canonical URL]
      Embed a MANUALLY authored agent layer. No extraction happens.

  agentic-pdf read <file.pdf> [--raw | --html | --meta]
      Extract and display the hidden agentic layer.
        (default)   markdown without frontmatter
        --raw       raw markdown exactly as embedded (pipe-friendly for agents)
        --html      rendered HTML
        --meta      metadata + frontmatter as JSON

  agentic-pdf view <file.pdf> [--port 4173] [--no-browser]
      Serve the viewer at http://localhost:<port>/?file=doc.pdf

  agentic-pdf check <file.pdf>
      Exit 0 and print a summary if the file carries an agentic layer.

  agentic-pdf install-backend [--spool DIR]   Install CUPS virtual printer (sudo)
  agentic-pdf uninstall-backend               Remove the CUPS virtual printer

  agentic-pdf watch <spool-dir> [--out DIR]
      Convert every PDF dropped into <spool-dir> into an agentic PDF
      (this is the engine behind the Windows print driver setup).

  agentic-pdf install-watch [--spool DIR] [--out DIR]
      Install the watcher as an auto-start service:
      Windows scheduled task / macOS LaunchAgent / Linux systemd unit.
  agentic-pdf uninstall-watch                 Remove the watcher service.

  agentic-pdf receive [--port 47631] [--out DIR]
      Run the HTTP receiver used by the CUPS backend (started automatically
      by install-backend on macOS).
`
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(help())
		return
	}
	if args[0] == "--version" || args[0] == "-v" {
		fmt.Println(version)
		return
	}

	var err error
	switch args[0] {
	case "print":
		err = cmdPrint(args[1:])
	case "agentify":
		err = cmdAgentify(args[1:])
	case "read":
		err = cmdRead(args[1:])
	case "view":
		err = cmdView(args[1:])
	case "check":
		err = cmdCheck(args[1:])
	case "install-backend":
		err = cli.InstallBackend(flagValue(args, "--spool"))
	case "uninstall-backend":
		err = cli.UninstallBackend()
	case "watch":
		err = cmdWatch(args[1:])
	case "install-watch":
		err = cli.InstallWatch(flagValue(args, "--spool"), flagValue(args, "--out"))
	case "uninstall-watch":
		err = cli.UninstallWatch()
	case "receive":
		err = cmdReceive(args[1:])
	case "debug-tables":
		err = cli.DebugTables(args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		fmt.Print(help())
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cmdAgentify(args []string) error {
	input := ""
	out := ""
	title := ""
	author := ""
	canonical := ""
	noHTML := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			i++
			if i >= len(args) {
				return fmt.Errorf("agentify: %s needs a value", args[i-1])
			}
			out = args[i]
		case "--title":
			i++
			if i >= len(args) {
				return fmt.Errorf("agentify: --title needs a value")
			}
			title = args[i]
		case "--author":
			i++
			if i >= len(args) {
				return fmt.Errorf("agentify: --author needs a value")
			}
			author = args[i]
		case "--canonical":
			i++
			if i >= len(args) {
				return fmt.Errorf("agentify: --canonical needs a value")
			}
			canonical = args[i]
		case "--no-html":
			noHTML = true
		default:
			input = args[i]
		}
	}
	if input == "" {
		return fmt.Errorf("agentify: missing <input>")
	}
	return cli.Agentify(input, out, title, author, canonical, !noHTML)
}

func cmdPrint(args []string) error {
	input := ""
	out := ""
	mdFile := ""
	htmlFile := ""
	title := ""
	canonical := ""
	var attaches []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			i++
			if i >= len(args) {
				return fmt.Errorf("print: %s needs a value", args[i-1])
			}
			out = args[i]
		case "--md":
			i++
			if i >= len(args) {
				return fmt.Errorf("print: --md needs a value")
			}
			mdFile = args[i]
		case "--html":
			i++
			if i >= len(args) {
				return fmt.Errorf("print: --html needs a value")
			}
			htmlFile = args[i]
		case "--attach":
			i++
			if i >= len(args) {
				return fmt.Errorf("print: --attach needs a value")
			}
			attaches = append(attaches, args[i])
		case "--title":
			i++
			if i >= len(args) {
				return fmt.Errorf("print: --title needs a value")
			}
			title = args[i]
		case "--canonical":
			i++
			if i >= len(args) {
				return fmt.Errorf("print: --canonical needs a value")
			}
			canonical = args[i]
		default:
			input = args[i]
		}
	}
	if input == "" {
		return fmt.Errorf("print: missing <input>")
	}
	return cli.Print(input, out, mdFile, htmlFile, attaches, title, canonical)
}

func cmdRead(args []string) error {
	input := ""
	raw, htmlMode, meta := false, false, false
	for _, a := range args {
		switch a {
		case "--raw":
			raw = true
		case "--html":
			htmlMode = true
		case "--meta":
			meta = true
		default:
			input = a
		}
	}
	if input == "" {
		return fmt.Errorf("read: missing <file.pdf>")
	}
	return cli.Read(input, raw, htmlMode, meta)
}

func cmdCheck(args []string) error {
	input := ""
	for _, a := range args {
		if a != "" && a[0] != '-' {
			input = a
		}
	}
	if input == "" {
		return fmt.Errorf("check: missing <file.pdf>")
	}
	return cli.Check(input)
}

func cmdView(args []string) error {
	input := ""
	port := 4173
	openBrowser := true
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port":
			i++
			if i >= len(args) {
				return fmt.Errorf("view: --port needs a value")
			}
			p, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("view: invalid port %q", args[i])
			}
			port = p
		case "--no-browser":
			openBrowser = false
		default:
			input = args[i]
		}
	}
	if input == "" {
		return fmt.Errorf("view: missing <file.pdf>")
	}
	return viewer.Serve(input, port, openBrowser)
}

func cmdWatch(args []string) error {
	spool := ""
	out := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--out":
			i++
			if i >= len(args) {
				return fmt.Errorf("watch: --out needs a value")
			}
			out = args[i]
		default:
			if spool == "" {
				spool = args[i]
			}
		}
	}
	return cli.Watch(spool, out, false)
}

func cmdReceive(args []string) error {
	out := ""
	port := cli.DefaultReceiverPort
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port":
			i++
			if i >= len(args) {
				return fmt.Errorf("receive: --port needs a value")
			}
			p, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("receive: invalid port %q", args[i])
			}
			port = p
		case "--out":
			i++
			if i >= len(args) {
				return fmt.Errorf("receive: --out needs a value")
			}
			out = args[i]
		}
	}
	return cli.Receive(port, out)
}

func flagValue(args []string, flag string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
