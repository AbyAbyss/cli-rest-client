// Command term-rest-client is a terminal REST API client.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/AbyAbyss/cli-rest-client/internal/cli"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/internal/ui"
	"github.com/AbyAbyss/cli-rest-client/internal/webui"
)

var (
	// Version is set during build.
	Version = "dev"
	// BuildTime is set during build.
	BuildTime = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("term-rest-client", flag.ContinueOnError)
	dataPath := fs.String("data", "", "workspace file (default: $"+storage.EnvDataPath+" or <config dir>/term-rest-client/workspace.json)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	gui := fs.Bool("gui", false, "open the browser interface instead of the terminal one")
	port := fs.Int("port", 0, "with -gui: port to listen on (default: a free one; a busy port falls back to a free one)")
	lan := fs.Bool("lan", false, "with -gui: let other devices on your network open it (they still need the printed link)")
	noOpen := fs.Bool("no-open", false, "with -gui: print the link without opening a browser")
	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "Usage:\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE]                 start the interactive UI\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] -gui [-port N] [-lan] [-no-open]\n")
		fmt.Fprintf(out, "                                                open the browser interface (requests still run here, so no CORS)\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] list            list saved requests\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] run [-v] [-env NAME] [-set name=value]... <Collection/Request>...\n")
		fmt.Fprintf(out, "                                                send saved requests and run their tests\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] env [NAME|none] list environments, or switch the active one\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] import FILE...  import Postman collections, environments, globals or curl files\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] curl [-into Collection/Folder] [-name NAME] [CURL ARGS | -]\n")
		fmt.Fprintf(out, "                                                save a curl command as a request (stdin when no args)\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] export [-o FILE] <Collection/Folder> | -env NAME | -globals\n")
		fmt.Fprintf(out, "                                                export to Postman format\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] export -lang LANG [-raw] <Collection/Request>...\n")
		fmt.Fprintf(out, "                                                print requests as code: curl, python, javascript, go, httpie\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] load [-vus N -duration D | -stages 30s:50,1m:50,30s:0 | -rate 100/s]\n")
		fmt.Fprintf(out, "                     [-threshold p95<300ms] [-out FILE] <Collection/Request>...   load test (load -h for more)\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] history [-n N]  show recently sent requests\n")
		fmt.Fprintf(out, "  term-rest-client demo [-addr localhost:8080]  serve the demo API that the sample's Local environment uses\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Printf("term-rest-client %s (built %s)\n", Version, BuildTime)
		return 0
	}

	path := *dataPath
	if path == "" {
		p, err := storage.DefaultPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		path = p
	}
	store := &storage.Store{Path: path}
	ws, created, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot load workspace: %v\n", err)
		fmt.Fprintf(os.Stderr, "Fix or move the file, or start with a different one using -data.\n")
		return 1
	}

	if rest := fs.Args(); len(rest) > 0 {
		switch rest[0] {
		case "list":
			return cli.List(os.Stdout, ws)
		case "run":
			return cli.Run(os.Stdout, os.Stderr, ws, store, rest[1:])
		case "import":
			return cli.Import(os.Stdout, os.Stderr, ws, store, rest[1:])
		case "curl":
			return cli.Curl(os.Stdin, os.Stdout, os.Stderr, ws, store, rest[1:])
		case "export":
			return cli.Export(os.Stdout, os.Stderr, ws, rest[1:])
		case "env":
			return cli.Env(os.Stdout, os.Stderr, ws, store, rest[1:])
		case "history":
			return cli.History(os.Stdout, os.Stderr, store, rest[1:])
		case "load":
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return cli.Load(ctx, os.Stdin, os.Stdout, os.Stderr, ws, rest[1:])
		case "demo":
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return cli.Demo(ctx, os.Stdout, os.Stderr, rest[1:])
		case "help":
			fs.Usage()
			return 0
		default:
			fmt.Fprintf(os.Stderr, "unknown command %q\n\n", rest[0])
			fs.Usage()
			return 2
		}
	}

	if created {
		if err := store.Save(ws); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot create %s: %v\n", path, err)
		}
	}
	if *gui {
		return runGUI(ws, store, *port, *lan, *noOpen)
	}
	app := ui.New(ws, store, ui.BuildInfo{Version: Version, BuildTime: BuildTime})
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func runGUI(ws *models.Workspace, store *storage.Store, port int, lan, noOpen bool) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := webui.Run(ctx, ws, store, webui.Options{
		Port: port, LAN: lan,
		Info: webui.Info{Version: Version, BuildTime: BuildTime},
		Ready: func(a webui.Addresses) {
			fmt.Printf("\n  term-rest-client GUI is running\n\n")
			if a.Fallback != "" {
				fmt.Printf("  Note:    %s\n\n", a.Fallback)
			}
			for i, u := range a.Local {
				label := "Local:  "
				if i > 0 {
					label = "        "
				}
				fmt.Printf("  %s %s\n", label, u)
			}
			switch {
			case lan && len(a.Network) == 0:
				fmt.Printf("  Network: no network address found\n")
			case lan:
				for i, u := range a.Network {
					label := "Network:"
					if i > 0 {
						label = "        "
					}
					fmt.Printf("  %s %s\n", label, u)
				}
			default:
				fmt.Printf("  Network: use -lan to open it from other devices\n")
			}
			fmt.Printf("\n  The link carries a one-time key; keep it private. Press Ctrl+C to stop.\n\n")
			if !noOpen {
				if err := openBrowser(a.Open()); err != nil {
					fmt.Printf("  Couldn't open a browser (%v). Open the Local link yourself.\n\n", err)
				}
			}
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck // reap the process
	return nil
}
