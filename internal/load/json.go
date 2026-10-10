package load

import (
	"encoding/json"
	"time"
)

// Times are written to JSON as milliseconds (floats), not Go's nanoseconds,
// so results files and the browser UI read naturally.

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func fromMs(v float64) time.Duration { return time.Duration(v * float64(time.Millisecond)) }

func (l Latency) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]float64{
		"minMs": ms(l.Min), "meanMs": ms(l.Mean), "p50Ms": ms(l.P50), "p90Ms": ms(l.P90),
		"p95Ms": ms(l.P95), "p99Ms": ms(l.P99), "maxMs": ms(l.Max),
	})
}

func (p Point) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Second   int     `json:"second"`
		Requests int     `json:"requests"`
		Failed   int     `json:"failed"`
		VUs      int     `json:"vus"`
		Target   float64 `json:"target"`
		Mean     float64 `json:"meanMs"`
		P50      float64 `json:"p50Ms"`
		P95      float64 `json:"p95Ms"`
		P99      float64 `json:"p99Ms"`
		Max      float64 `json:"maxMs"`
	}{p.Second, p.Requests, p.Failed, p.VUs, p.Target, ms(p.Mean), ms(p.P50), ms(p.P95), ms(p.P99), ms(p.Max)})
}

func (p Progress) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Elapsed  float64 `json:"elapsedMs"`
		Total    float64 `json:"totalMs"`
		VUs      int     `json:"vus"`
		Target   float64 `json:"target"`
		Requests uint64  `json:"requests"`
		Failed   uint64  `json:"failed"`
		Point    Point   `json:"point"`
		P95      float64 `json:"p95Ms"`
		Done     bool    `json:"done"`
	}{ms(p.Elapsed), ms(p.Total), p.VUs, p.Target, p.Requests, p.Failed, p.Point, ms(p.P95), p.Done})
}

type stageJSON struct {
	Duration float64 `json:"durationMs"`
	Target   float64 `json:"target"`
}

func (s Stage) MarshalJSON() ([]byte, error) {
	return json.Marshal(stageJSON{ms(s.Duration), s.Target})
}

func (s *Stage) UnmarshalJSON(b []byte) error {
	var v stageJSON
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	s.Duration, s.Target = fromMs(v.Duration), v.Target
	return nil
}

func (s *Summary) MarshalJSON() ([]byte, error) {
	type plain Summary // drops the method, keeps the fields
	return json.Marshal(struct {
		*plain
		Planned  float64 `json:"plannedMs"`
		Duration float64 `json:"durationMs"`
	}{(*plain)(s), ms(s.Planned), ms(s.Duration)})
}
