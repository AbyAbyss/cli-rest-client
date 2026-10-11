package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AbyAbyss/cli-rest-client/internal/load"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// Load runs a load test on saved requests: each virtual user runs the
// named requests in order, again and again. It returns 1 when a threshold
// fails, 2 for usage errors.
func Load(ctx context.Context, in io.Reader, out, errOut io.Writer, ws *models.Workspace, args []string) int {
	fs := flag.NewFlagSet("load", flag.ContinueOnError)
	fs.SetOutput(errOut)
	vus := fs.Int("vus", 10, "virtual users, each sending the requests back to back")
	duration := fs.Duration("duration", 30*time.Second, "how long to run (with -vus or -rate)")
	stages := fs.String("stages", "", "ramp the load: duration:target pairs, e.g. 30s:50,1m:50,30s:0 (targets with /s are a request rate)")
	rate := fs.String("rate", "", "start iterations at a fixed rate instead, e.g. 200/s, however slow the server gets")
	maxVUs := fs.Int("max-vus", 0, "with a rate: most iterations running at once (default: twice the peak rate, at least 100)")
	think := fs.Duration("think", 0, "pause after each iteration, per virtual user")
	var thresholds multiFlag
	fs.Var(&thresholds, "threshold", "fail (exit 1) unless this holds, e.g. p95<300ms or errors<1% (repeatable, or comma-separated)")
	outFile := fs.String("out", "", "also write the results: .json (summary and per-second timeline) or .csv (timeline)")
	yes := fs.Bool("yes", false, "don't ask before load testing hosts other than this machine or a private network")
	quiet := fs.Bool("q", false, "no live progress line")
	overrides := setFlags{}
	fs.Var(overrides, "set", "override a variable for this run (name=value, repeatable)")
	envName := fs.String("env", "", "environment to use (default: the active one; \"none\" for globals only)")
	fs.Usage = func() {
		fmt.Fprintf(errOut, "Usage: rest load [flags] <Collection/Request | Collection/Folder>...\n\n")
		fmt.Fprintf(errOut, "Each virtual user runs the named requests in order, over and over, with its own\n")
		fmt.Fprintf(errOut, "copy of the variables. Tests become checks. Examples:\n\n")
		fmt.Fprintf(errOut, "  rest load -vus 20 -duration 1m \"Shop API/Checkout\"\n")
		fmt.Fprintf(errOut, "  rest load -stages 30s:50,1m:50,30s:0 \"Shop API\"\n")
		fmt.Fprintf(errOut, "  rest load -rate 200/s -duration 1m -threshold \"p95<300ms,errors<1%%\" \"Shop API/Search\"\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	usage := func(format string, a ...any) int {
		fmt.Fprintf(errOut, "load: "+format+"\n", a...)
		return 2
	}
	if fs.NArg() == 0 {
		return usage("name at least one request or folder, e.g. \"User Service/Lookup/Get JSON\" (see the list command)")
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	// The plan.
	var plan load.Plan
	switch {
	case *stages != "":
		if set["vus"] || set["duration"] || set["rate"] {
			return usage("-stages sets the load and the duration; drop -vus, -duration and -rate (use /s in the stages for a rate)")
		}
		p, err := load.ParseStages(*stages)
		if err != nil {
			return usage("%v", err)
		}
		plan = p
	case *rate != "":
		if set["vus"] {
			return usage("-rate and -vus don't mix; with a rate, -max-vus caps how many iterations run at once")
		}
		r, err := load.ParseRate(*rate)
		if err != nil {
			return usage("%v", err)
		}
		plan = load.Constant(r, *duration, true)
	default:
		if *vus <= 0 {
			return usage("-vus must be at least 1")
		}
		plan = load.Constant(float64(*vus), *duration, false)
	}
	if plan.Total() <= 0 {
		return usage("-duration must be more than zero")
	}
	var ths []load.Threshold
	for _, t := range thresholds {
		parsed, err := load.ParseThresholds(t)
		if err != nil {
			return usage("%v", err)
		}
		ths = append(ths, parsed...)
	}
	if *outFile != "" && !strings.HasSuffix(*outFile, ".json") && !strings.HasSuffix(*outFile, ".csv") {
		return usage("-out must end in .json or .csv")
	}

	// The environment and the requests.
	activeBefore := ws.ActiveEnvironment
	if *envName != "" {
		defer func() { ws.ActiveEnvironment = activeBefore }()
		if isGlobalsOnly(*envName) {
			ws.SetActive("")
		} else if ws.Environment(*envName) == nil {
			fmt.Fprintf(errOut, "load: no environment named %q\n%s\n", *envName, envHint(ws))
			return 2
		} else {
			ws.SetActive(*envName)
		}
	}
	var steps []load.Step
	skipped := 0
	for _, name := range fs.Args() {
		found := find(ws, name)
		if len(found) == 0 {
			fmt.Fprintf(errOut, "load: no request, collection or folder named %q\n%s\n", name, pathHint(ws, name))
			return 2
		}
		for _, r := range found {
			if r.Type == models.TypeWebSocket {
				skipped++
				continue
			}
			steps = append(steps, load.Step{Name: requestPath(ws, r), Request: *r})
		}
	}
	if skipped > 0 {
		fmt.Fprintf(errOut, "load: skipping %d WebSocket request(s); load tests send HTTP requests only\n", skipped)
	}
	if len(steps) == 0 {
		return usage("no HTTP requests to send")
	}
	vars := ws.VariableMap()
	for k, v := range overrides {
		vars[k] = v
	}

	// Ask before load testing someone else's server.
	if remote := load.Remote(load.Hosts(steps, vars)); len(remote) > 0 && !*yes {
		f, isFile := in.(*os.File)
		if !isFile || !isTerminal(f) {
			return usage("%s isn't on this machine or a private network. Only load test servers you own or are allowed to test, then add -yes", strings.Join(remote, ", "))
		}
		fmt.Fprintf(errOut, "This sends %s to %s.\nOnly load test servers you own or are allowed to test. Continue? [y/N] ", plan.Describe(), strings.Join(remote, ", "))
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Fprintln(errOut, "Cancelled.")
			return 1
		}
	}

	timeout := time.Duration(ws.Settings.TimeoutSeconds) * time.Second
	what := steps[0].Name
	if len(steps) > 1 {
		what = fmt.Sprintf("%d requests per iteration, starting with %s", len(steps), steps[0].Name)
	}
	fmt.Fprintf(out, "Load test: %s\n", what)
	if env := ws.Active(); env != nil {
		fmt.Fprintf(out, "Environment: %s\n", env.Name)
	}
	fmt.Fprintf(out, "Plan: %s. Press Ctrl+C to stop early.\n", plan.Describe())

	live := !*quiet
	stderrTTY := false
	if f, ok := errOut.(*os.File); ok {
		stderrTTY = isTerminal(f)
	}
	var mu sync.Mutex
	lastLog := time.Time{}
	tick := func(p load.Progress) {
		if !live {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		line := load.ProgressLine(p, plan.Rate)
		switch {
		case stderrTTY && p.Done:
			fmt.Fprint(errOut, "\r\033[K")
		case stderrTTY:
			fmt.Fprintf(errOut, "\r\033[K%s", line)
		case p.Done || time.Since(lastLog) >= 10*time.Second:
			// Logs (CI): a line every 10 seconds instead of redrawing.
			if !p.Done {
				fmt.Fprintln(errOut, line)
			}
			lastLog = time.Now()
		}
	}
	s, err := load.Run(ctx, load.Config{
		Steps: steps, Plan: plan, Variables: vars, MaxVUs: *maxVUs, Think: *think,
		Timeout: timeout, InsecureTLS: ws.Settings.InsecureSkipVerify, DisableRedirects: ws.Settings.DisableRedirects,
		Thresholds: ths, Tick: tick,
	})
	if err != nil {
		fmt.Fprintln(errOut, "load:", err)
		return 2
	}
	load.WriteSummary(out, s)
	if *outFile != "" {
		if err := writeResults(*outFile, s); err != nil {
			fmt.Fprintln(errOut, "load:", err)
			return 1
		}
		fmt.Fprintf(out, "Results written to %s\n", *outFile)
	}
	if !s.OK() {
		return 1
	}
	return 0
}

func writeResults(path string, s *load.Summary) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if strings.HasSuffix(path, ".csv") {
		err = load.WriteCSV(f, s)
	} else {
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		err = enc.Encode(s)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// requestPath is a request's "Collection/Folder/Request" path.
func requestPath(ws *models.Workspace, req *models.Request) string {
	out := req.Name
	ws.WalkRequests(func(path []*models.Collection, r *models.Request) {
		if r == req {
			out = joinPath(path, r.Name)
		}
	})
	return out
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
