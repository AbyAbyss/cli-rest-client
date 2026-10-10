package ui

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/AbyAbyss/cli-rest-client/internal/load"
	"github.com/AbyAbyss/cli-rest-client/internal/models"
)

// loadForm is the last load test settings, kept for the session.
type loadForm = load.Form

var defaultLoadForm = load.DefaultForm

// loadCurrent load tests the request in the builder (Alt+L, F7).
func (a *App) loadCurrent() {
	if a.req.Type == models.TypeWebSocket {
		a.setStatus(levelWarning, "Load tests send HTTP requests; WebSocket isn't supported yet")
		return
	}
	name := a.req.Name
	if a.linked != nil {
		name = a.requestPath(a.linked)
	}
	a.loadDialog(name, []load.Step{{Name: name, Request: a.req.Clone()}})
}

// loadSelected load tests the request, folder or collection selected in
// the tree (l): a folder runs its requests in order, sub-folders included.
func (a *App) loadSelected() {
	c, r := a.selection()
	switch {
	case r != nil:
		if r.Type == models.TypeWebSocket {
			a.setStatus(levelWarning, "Load tests send HTTP requests; WebSocket isn't supported yet")
			return
		}
		name := a.requestPath(r)
		a.loadDialog(name, []load.Step{{Name: name, Request: r.Clone()}})
	case c != nil:
		var steps []load.Step
		a.ws.WalkRequests(func(path []*models.Collection, req *models.Request) {
			for _, p := range path {
				if p == c && req.Type != models.TypeWebSocket {
					steps = append(steps, load.Step{Name: a.requestPath(req), Request: req.Clone()})
					return
				}
			}
		})
		if len(steps) == 0 {
			a.setStatus(levelWarning, c.Name+" has no HTTP requests to load test")
			return
		}
		a.loadDialog(c.Name, steps)
	default:
		a.setStatus(levelWarning, "Select a request or folder to load test")
	}
}

// requestPath is "Collection/Folder/Request".
func (a *App) requestPath(req *models.Request) string {
	out := req.Name
	a.ws.WalkRequests(func(path []*models.Collection, r *models.Request) {
		if r == req {
			names := make([]string, 0, len(path)+1)
			for _, c := range path {
				names = append(names, c.Name)
			}
			out = strings.Join(append(names, r.Name), "/")
		}
	})
	return out
}

func (a *App) loadDialog(name string, steps []load.Step) {
	f := a.loadSettings
	inputs := []*tview.InputField{
		a.newInput("Load      ").SetText(f.Load).SetPlaceholder("10 virtual users, or 200/s for a rate"),
		a.newInput("Duration  ").SetText(f.Duration).SetPlaceholder("30s"),
		a.newInput("Stages    ").SetText(f.Stages).SetPlaceholder("optional: 30s:50,1m:50,30s:0 ramps up, holds, ramps down"),
		a.newInput("Think     ").SetText(f.Think).SetPlaceholder("optional: pause after each iteration, e.g. 500ms"),
		a.newInput("Pass if   ").SetText(f.Thresholds).SetPlaceholder("optional: p95<300ms, errors<1%"),
	}
	items := make([]tview.Primitive, len(inputs))
	for i, in := range inputs {
		items[i] = in
	}
	what := name
	if len(steps) > 1 {
		what = fmt.Sprintf("%s (%d requests per iteration)", name, len(steps))
	}
	submit := func() {
		form := loadForm{Load: inputs[0].GetText(), Duration: inputs[1].GetText(), Stages: inputs[2].GetText(), Think: inputs[3].GetText(), Thresholds: inputs[4].GetText()}
		plan, think, ths, err := form.Parse()
		if err != nil {
			a.setStatus(levelError, err.Error())
			return
		}
		a.loadSettings = form
		a.closeDialog()
		cfg := load.Config{
			Steps: steps, Plan: plan, Variables: a.ws.VariableMap(), Think: think, Thresholds: ths,
			Timeout:     time.Duration(a.ws.Settings.TimeoutSeconds) * time.Second,
			InsecureTLS: a.ws.Settings.InsecureSkipVerify, DisableRedirects: a.ws.Settings.DisableRedirects,
		}
		start := func() { a.runLoad(what, cfg) }
		if remote := load.Remote(load.Hosts(steps, cfg.Variables)); len(remote) > 0 {
			a.confirm(fmt.Sprintf("This sends %s to %s.\n\nOnly load test servers you own or are allowed to test.",
				plan.Describe(), strings.Join(remote, ", ")), []string{"Start", "Cancel"}, func(label string) {
				if label == "Start" {
					start()
				}
			})
			return
		}
		start()
	}
	a.formDialog("Load test · "+what,
		"Enter starts · Esc cancels · each user repeats the request(s) with its own variables",
		items, submit, 96)
}

// loadView is the live view of a running load test.
type loadView struct {
	what     string
	cfg      load.Config
	cancel   context.CancelFunc
	last     load.Progress
	points   []load.Point
	summary  *load.Summary
	text     *tview.TextView
	started  time.Time
	stopping bool
}

func (a *App) runLoad(what string, cfg load.Config) {
	t := a.theme
	ctx, cancel := context.WithCancel(context.Background())
	v := &loadView{what: what, cfg: cfg, cancel: cancel, started: time.Now()}
	v.text = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	v.text.SetBackgroundColor(t.Input)
	v.text.SetBorder(true).SetBorderColor(t.Focus).SetTitleColor(t.Title).SetBorderPadding(1, 0, 2, 2)
	v.text.SetTitle(" Load test · " + what + " ")
	a.loadRun = v
	v.text.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyEsc || (ev.Key() == tcell.KeyRune && ev.Rune() == 'q'):
			if v.summary == nil {
				v.stopping = true
				v.cancel()
				a.renderLoad(v)
				return nil
			}
			a.loadRun = nil
			a.closeDialog()
		case ev.Key() == tcell.KeyRune && ev.Rune() == 'c' && v.summary != nil:
			var b bytes.Buffer
			fmt.Fprintf(&b, "Load test: %s\n", v.what)
			load.WriteSummary(&b, v.summary)
			a.copyText(b.String(), "the load test summary")
		}
		return nil
	})
	cfg.Tick = func(p load.Progress) {
		a.tv.QueueUpdateDraw(func() {
			v.last = p
			if p.Point.Second > 0 && (len(v.points) == 0 || v.points[len(v.points)-1].Second != p.Point.Second) {
				v.points = append(v.points, p.Point)
			}
			a.renderLoad(v)
		})
	}
	a.renderLoad(v)
	a.openDialog(v.text, 110, 30)
	go func() {
		s, err := load.Run(ctx, cfg)
		a.tv.QueueUpdateDraw(func() {
			if err != nil {
				a.setStatus(levelError, "Load test: "+err.Error())
				if a.loadRun == v {
					a.loadRun = nil
					a.closeDialog()
				}
				return
			}
			v.summary = s
			v.points = s.Timeline
			a.renderLoad(v)
			switch {
			case !s.OK():
				a.setStatus(levelError, "Load test done: thresholds failed")
			case s.Failed > 0:
				a.setStatus(levelWarning, fmt.Sprintf("Load test done: %s of requests failed", load.FormatPercent(s.FailedRate())))
			default:
				a.setStatus(levelSuccess, fmt.Sprintf("Load test done: %d requests, p95 %s", s.Requests, load.FormatDuration(s.Latency.P95)))
			}
		})
	}()
}

// sparkline draws values as ▁▂▃▄▅▆▇█, scaled to the largest.
func sparkline(vals []float64, width int) string {
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	peak := 0.0
	for _, v := range vals {
		peak = math.Max(peak, v)
	}
	if peak == 0 {
		return strings.Repeat("▁", len(vals))
	}
	const bars = "▁▂▃▄▅▆▇█"
	runes := []rune(bars)
	var b strings.Builder
	for _, v := range vals {
		i := int(math.Round(v / peak * float64(len(runes)-1)))
		b.WriteRune(runes[i])
	}
	return b.String()
}

func (a *App) renderLoad(v *loadView) {
	t := a.theme
	var b strings.Builder
	muted, accent, text := t.HexMuted, t.HexAccent, t.HexText
	if v.summary != nil {
		var out bytes.Buffer
		load.WriteSummary(&out, v.summary)
		s := tview.Escape(out.String())
		s = strings.ReplaceAll(s, "✓", "["+t.HexSuccess+"]✓["+text+"]")
		s = strings.ReplaceAll(s, "✗", "["+t.HexError+"]✗["+text+"]")
		fmt.Fprintf(&b, "[%s]%s", text, strings.TrimLeft(s, "\n"))
		fmt.Fprintf(&b, "\n[%s]c copies this summary · Esc closes", muted)
		v.text.SetText(b.String())
		v.text.ScrollToBeginning()
		return
	}
	p := v.last
	fmt.Fprintf(&b, "[%s]%s[-]\n\n", muted, tview.Escape(v.cfg.Plan.Describe()))
	elapsed := p.Elapsed
	if elapsed == 0 {
		elapsed = time.Since(v.started)
	}
	total := v.cfg.Plan.Total()
	frac := math.Min(float64(elapsed)/float64(total), 1)
	const barW = 40
	filled := int(frac * barW)
	fmt.Fprintf(&b, "[%s]%s[%s]%s[-]  [%s]%s / %s[-]\n\n", accent, strings.Repeat("█", filled), muted, strings.Repeat("░", barW-filled),
		text, clock(elapsed), clock(total))

	var vus, rps, p95 []float64
	for _, pt := range v.points {
		vus = append(vus, float64(pt.VUs))
		rps = append(rps, float64(pt.Requests))
		p95 = append(p95, float64(pt.P95))
	}
	// A blank line between rows keeps the sparklines apart: full-height
	// blocks on adjacent lines run together.
	row := func(label, value, spark string) {
		fmt.Fprintf(&b, "[%s]%-15s[%s::b]%-14s[-:-:-] [%s]%s[-]\n\n", muted, label, text, value, accent, spark)
	}
	who := "Virtual users"
	whoVal := fmt.Sprintf("%d / %.0f", p.VUs, p.Target)
	if v.cfg.Plan.Rate {
		who, whoVal = "Busy users", fmt.Sprintf("%d (%.0f/s)", p.VUs, p.Target)
	}
	row(who, whoVal, sparkline(vus, 60))
	row("Requests/s", strconv.Itoa(p.Point.Requests), sparkline(rps, 60))
	row("p95", load.FormatDuration(p.Point.P95), sparkline(p95, 60))
	failed := "0%"
	if p.Requests > 0 {
		failed = fmt.Sprintf("%s  (%d of %d)", load.FormatPercent(float64(p.Failed)/float64(p.Requests)), p.Failed, p.Requests)
	}
	fmt.Fprintf(&b, "[%s]%-15s[%s]%s[-]\n", muted, "Failed", text, failed)
	fmt.Fprintf(&b, "[%s]%-15s[%s]%d[-]\n\n", muted, "Requests", text, p.Requests)
	if v.stopping {
		fmt.Fprintf(&b, "[%s]Stopping: letting requests in flight finish…", t.HexWarning)
	} else {
		fmt.Fprintf(&b, "[%s]Esc stops early; the results stay on screen", muted)
	}
	v.text.SetText(b.String())
}

func clock(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}
