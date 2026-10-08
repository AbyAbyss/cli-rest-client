// Command term-rest-client is a terminal REST API client.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AbyAbyss/cli-rest-client/internal/cli"
	"github.com/AbyAbyss/cli-rest-client/internal/storage"
	"github.com/AbyAbyss/cli-rest-client/internal/ui"
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
	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "Usage:\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE]                 start the interactive UI\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] list            list saved requests\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] run [-v] [-env NAME] [-set name=value]... <Collection/Request>...\n")
		fmt.Fprintf(out, "                                                send saved requests and run their tests\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] env [NAME|none] list environments, or switch the active one\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] import FILE...  import Postman collections, environments or globals\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] export [-o FILE] <Collection/Folder> | -env NAME | -globals\n")
		fmt.Fprintf(out, "                                                export to Postman format\n")
		fmt.Fprintf(out, "  term-rest-client [-data FILE] history [-n N]  show recently sent requests\n\nFlags:\n")
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
		case "export":
			return cli.Export(os.Stdout, os.Stderr, ws, rest[1:])
		case "env":
			return cli.Env(os.Stdout, os.Stderr, ws, store, rest[1:])
		case "history":
			return cli.History(os.Stdout, os.Stderr, store, rest[1:])
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
	app := ui.New(ws, store, ui.BuildInfo{Version: Version, BuildTime: BuildTime})
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}
