package load

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Stage moves the load from the previous stage's target (or Plan.Start) to
// Target over Duration, in a straight line. Target is a number of virtual
// users, or requests per second in rate mode.
type Stage struct {
	Duration time.Duration `json:"duration"`
	Target   float64       `json:"target"`
}

// Plan is how much load to apply over time.
type Plan struct {
	// Rate is true when targets are iterations started per second (an open
	// model: work arrives on schedule however slow the server gets).
	// Otherwise they are virtual users, each running iterations back to
	// back (a closed model).
	Rate   bool    `json:"rate"`
	Start  float64 `json:"start"`
	Stages []Stage `json:"stages"`
}

// Total is how long the plan runs.
func (p Plan) Total() time.Duration {
	var t time.Duration
	for _, s := range p.Stages {
		t += s.Duration
	}
	return t
}

// At is the target at t since the start.
func (p Plan) At(t time.Duration) float64 {
	from := p.Start
	for _, s := range p.Stages {
		if t < s.Duration {
			if s.Duration <= 0 {
				return s.Target
			}
			f := float64(t) / float64(s.Duration)
			return from + (s.Target-from)*f
		}
		t -= s.Duration
		from = s.Target
	}
	return from
}

// Peak is the highest target in the plan.
func (p Plan) Peak() float64 {
	m := p.Start
	for _, s := range p.Stages {
		m = max(m, s.Target)
	}
	return m
}

// Constant is a plan that holds target for d.
func Constant(target float64, d time.Duration, rate bool) Plan {
	return Plan{Rate: rate, Start: target, Stages: []Stage{{Duration: d, Target: target}}}
}

// ParseStages reads "30s:50,1m:50,30s:0": ramp to 50 virtual users over 30
// seconds, hold for a minute, ramp down. Targets written as "100/s" make it
// a rate plan; every stage must then use /s. The plan starts from 0.
func ParseStages(spec string) (Plan, error) {
	var p Plan
	parts := strings.Split(spec, ",")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		d, t, ok := strings.Cut(part, ":")
		if !ok {
			return p, fmt.Errorf("stage %q: expected duration:target, like 30s:50 or 1m:100/s", part)
		}
		dur, err := time.ParseDuration(strings.TrimSpace(d))
		if err != nil || dur < 0 {
			return p, fmt.Errorf("stage %q: %q is not a duration like 30s or 2m", part, d)
		}
		t = strings.TrimSpace(t)
		rate := strings.HasSuffix(t, "/s")
		if i == 0 {
			p.Rate = rate
		} else if rate != p.Rate {
			return p, errors.New("stages: use /s on every target (a request rate) or on none (virtual users)")
		}
		n, err := strconv.ParseFloat(strings.TrimSuffix(t, "/s"), 64)
		if err != nil || n < 0 {
			return p, fmt.Errorf("stage %q: %q is not a number", part, t)
		}
		if !p.Rate && n != float64(int(n)) {
			return p, fmt.Errorf("stage %q: virtual users must be a whole number", part)
		}
		p.Stages = append(p.Stages, Stage{Duration: dur, Target: n})
	}
	if len(p.Stages) == 0 {
		return p, errors.New("stages: none given")
	}
	if p.Total() <= 0 {
		return p, errors.New("stages: the total duration must be more than zero")
	}
	return p, nil
}

// ParseRate reads "200/s", "200" or "12000/m" as requests per second.
func ParseRate(s string) (float64, error) {
	s = strings.TrimSpace(s)
	per := 1.0
	switch {
	case strings.HasSuffix(s, "/s"):
		s = strings.TrimSuffix(s, "/s")
	case strings.HasSuffix(s, "/m"):
		s, per = strings.TrimSuffix(s, "/m"), 60
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("rate %q: expected a number per second, like 200/s", s)
	}
	return n / per, nil
}

// String writes the plan back in the stage syntax.
func (p Plan) String() string {
	var parts []string
	for _, s := range p.Stages {
		v := strconv.FormatFloat(s.Target, 'f', -1, 64)
		if p.Rate {
			v += "/s"
		}
		parts = append(parts, s.Duration.String()+":"+v)
	}
	return strings.Join(parts, ",")
}
