package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/abhirupghosh/quota/internal/config"
	"github.com/abhirupghosh/quota/internal/status"
	"github.com/abhirupghosh/quota/internal/switcher"
)

// Overridden at release with -ldflags "-X main.version=vX.Y.Z".
var version = "0.1.0"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	switch args[0] {
	case "ls":
		list, err := switcher.List("")
		if err != nil {
			fatal(err.Error())
		}
		if len(list) == 0 {
			fmt.Println("no snapshots yet")
			return
		}
		for _, m := range list {
			fmt.Printf("%-10s  %-6s  %s  %s\n", m.Name, m.Tool, m.Email, m.Plan)
		}
	case "save":
		tool, name, host := parseToolNameHost(args[1:])
		if tool != "claude" {
			fatal("phase 2 is Claude only")
		}
		var m switcher.Meta
		var err error
		if host == "" || host == "mac" {
			m, err = switcher.SaveLocal("", name, "mac")
		} else {
			h, herr := switcher.FindHost(host)
			if herr != nil {
				fatal(herr.Error())
			}
			m, err = switcher.SaveRemote(h, name)
		}
		if err != nil {
			fatal(err.Error())
		}
		fmt.Printf("saved %s %s (%s)\n", m.Name, m.Email, m.Plan)
	case "use":
		tool, name, host := parseToolNameHost(args[1:])
		if tool != "claude" {
			fatal("phase 2 is Claude only")
		}
		var m switcher.Meta
		var err error
		if host == "" || host == "mac" {
			m, err = switcher.UseLocal("", name)
		} else {
			h, herr := switcher.FindHost(host)
			if herr != nil {
				fatal(herr.Error())
			}
			m, err = switcher.UseRemote(h, name)
		}
		if err != nil {
			fatal(err.Error())
		}
		where := displayHost(host)
		fmt.Printf("using %s on %s (%s)\n", m.Name, where, m.Email)
		notify(fmt.Sprintf("%s Claude → %s", where, m.Name), "Next new claude uses this. Running sessions stay put.")
	case "push":
		host := ""
		for i := 1; i < len(args); i++ {
			if args[i] == "--host" {
				i++
				if i >= len(args) {
					fatal("--host needs a name")
				}
				host = args[i]
			}
		}
		for _, h := range config.Hosts() {
			if h.Local {
				continue
			}
			if host != "" && h.Name != host && h.SSH != host {
				continue
			}
			if err := switcher.PushHost(h); err != nil {
				fatal(h.Name + ": " + err.Error())
			}
			fmt.Println("pushed snapshots to", h.Name)
		}
	case "status":
		jsonOut := false
		swiftbar := false
		host := ""
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--json":
				jsonOut = true
			case "--swiftbar":
				swiftbar = true
			case "--host":
				i++
				if i >= len(args) {
					fatal("--host needs a name")
				}
				host = args[i]
			case "-h", "--help":
				usage()
				return
			default:
				fatal("unknown flag " + args[i])
			}
		}
		rep := status.Collect(config.Hosts(), host)
		switch {
		case jsonOut:
			if err := status.WriteJSON(rep, os.Stdout); err != nil {
				fatal(err.Error())
			}
		case swiftbar:
			snaps, _ := switcher.List("")
			opts := make([]status.AccountOption, 0, len(snaps))
			for _, m := range snaps {
				opts = append(opts, status.AccountOption{Name: m.Name, Email: m.Email, Tool: m.Tool})
			}
			bin, err := os.Executable()
			if err != nil {
				bin = ""
			}
			fmt.Print(status.FormatSwiftBar(rep, opts, bin))
		default:
			fmt.Print(status.FormatTable(rep))
		}
	case "hosts":
		for _, h := range config.Hosts() {
			kind := "local"
			if !h.Local {
				kind = "ssh:" + h.SSH
			}
			fmt.Printf("%-12s  %s\n", h.Name, kind)
		}
		fmt.Println("#", config.HostsPath())
	case "init":
		path, created, err := config.InitHosts()
		if err != nil {
			fatal(err.Error())
		}
		if created {
			fmt.Println("wrote", path)
			fmt.Println("edit it to add SSH aliases from ~/.ssh/config, then: quota status")
			return
		}
		fmt.Println("already exists:", path)
	case "version", "--version":
		fmt.Println(version)
	case "-h", "--help", "help":
		usage()
	default:
		fatal("unknown command " + args[0])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `quota — Claude / Codex / Grok identity, usage, and account switch

  quota status [--host NAME] [--json|--swiftbar]
  quota hosts
  quota init
  quota ls
  quota save claude NAME [--host NAME]
  quota use  claude NAME [--host NAME]
  quota push [--host NAME]
  quota version

Hosts live in ~/.config/quota/hosts.json (see hosts.example.json).
Claude switch is future-sessions only. Codex and Grok come next.
`)
}

func parseToolNameHost(args []string) (tool, name, host string) {
	var pos []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--host" {
			i++
			if i >= len(args) {
				fatal("--host needs a name")
			}
			host = args[i]
			continue
		}
		if args[i] == "-h" || args[i] == "--help" {
			usage()
			os.Exit(0)
		}
		pos = append(pos, args[i])
	}
	if len(pos) < 2 {
		fatal("need: claude NAME")
	}
	return pos[0], pos[1], host
}

func displayHost(host string) string {
	if host == "" {
		return "mac"
	}
	return host
}

func notify(title, body string) {
	script := fmt.Sprintf(`display notification %q with title %q`, body, title)
	_ = runOSScript(script)
}

func runOSScript(script string) error {
	return exec.Command("osascript", "-e", script).Run()
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "quota:", msg)
	os.Exit(1)
}
