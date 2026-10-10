package load

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Form is a load test as typed into the terminal or browser UI.
type Form struct {
	Load       string `json:"load"`       // "10" virtual users, or "200/s"
	Duration   string `json:"duration"`   // "30s"
	Stages     string `json:"stages"`     // optional; replaces Load and Duration
	Think      string `json:"think"`      // optional pause per iteration
	Thresholds string `json:"thresholds"` // optional "p95<300ms, errors<1%"
}

// DefaultForm is what the forms start with.
var DefaultForm = Form{Load: "10", Duration: "30s"}

// Parse turns the form into a plan, a think time and thresholds.
func (f Form) Parse() (Plan, time.Duration, []Threshold, error) {
	var plan Plan
	var err error
	if s := strings.TrimSpace(f.Stages); s != "" {
		plan, err = ParseStages(s)
		if err != nil {
			return plan, 0, nil, err
		}
	} else {
		d, derr := time.ParseDuration(strings.TrimSpace(f.Duration))
		if derr != nil || d <= 0 {
			return plan, 0, nil, fmt.Errorf("duration %q: use something like 30s or 2m", f.Duration)
		}
		v := strings.TrimSpace(f.Load)
		if strings.Contains(v, "/") {
			r, rerr := ParseRate(v)
			if rerr != nil {
				return plan, 0, nil, rerr
			}
			plan = Constant(r, d, true)
		} else {
			n, nerr := strconv.Atoi(v)
			if nerr != nil || n <= 0 {
				return plan, 0, nil, fmt.Errorf("load %q: give a number of virtual users like 10, or a rate like 200/s", f.Load)
			}
			plan = Constant(float64(n), d, false)
		}
	}
	var think time.Duration
	if s := strings.TrimSpace(f.Think); s != "" {
		think, err = time.ParseDuration(s)
		if err != nil || think < 0 {
			return plan, 0, nil, fmt.Errorf("think %q: use a duration like 500ms", f.Think)
		}
	}
	ths, err := ParseThresholds(f.Thresholds)
	return plan, think, ths, err
}
