package load

import (
	"math"
	"math/bits"
	"time"
)

// Histogram records durations in microseconds in log-linear buckets: exact
// below 64µs, then 32 buckets per power of two, so any percentile is within
// about 3% of the true value while memory stays fixed however many samples
// arrive. The zero value is ready to use.
type Histogram struct {
	counts [histBuckets]uint32
	n      uint64
	sum    uint64 // µs
	min    uint64
	max    uint64
}

const (
	histExact   = 64
	histSub     = 32
	histPowers  = 36 // up to 2^42µs, about 50 days
	histBuckets = histExact + histPowers*histSub
)

func bucketOf(v uint64) int {
	if v < histExact {
		return int(v)
	}
	e := bits.Len64(v)  // 7 or more
	top := v >> (e - 6) // 32..63
	i := histExact + (e-7)*histSub + int(top-histSub)
	if i >= histBuckets {
		return histBuckets - 1
	}
	return i
}

// bucketValue is a representative value (the middle) of bucket i.
func bucketValue(i int) uint64 {
	if i < histExact {
		return uint64(i)
	}
	i -= histExact
	e := i/histSub + 7
	top := uint64(i%histSub + histSub)
	width := uint64(1) << (e - 6)
	return top<<(e-6) + width/2
}

// Record adds one duration.
func (h *Histogram) Record(d time.Duration) {
	v := uint64(max(d.Microseconds(), 0))
	h.counts[bucketOf(v)]++
	if h.n == 0 || v < h.min {
		h.min = v
	}
	if v > h.max {
		h.max = v
	}
	h.n++
	h.sum += v
}

// Merge adds o's samples to h.
func (h *Histogram) Merge(o *Histogram) {
	if o.n == 0 {
		return
	}
	for i, c := range o.counts {
		h.counts[i] += c
	}
	if h.n == 0 || o.min < h.min {
		h.min = o.min
	}
	h.max = max(h.max, o.max)
	h.n += o.n
	h.sum += o.sum
}

// Count is the number of samples.
func (h *Histogram) Count() uint64 { return h.n }

// Percentile returns the value below which p (0..100) percent of samples
// fall. The exact min and max are returned at the ends.
func (h *Histogram) Percentile(p float64) time.Duration {
	if h.n == 0 {
		return 0
	}
	if p <= 0 {
		return us(h.min)
	}
	if p >= 100 {
		return us(h.max)
	}
	rank := uint64(math.Ceil(p / 100 * float64(h.n)))
	var seen uint64
	for i, c := range h.counts {
		seen += uint64(c)
		if seen >= rank {
			v := bucketValue(i)
			return us(min(max(v, h.min), h.max))
		}
	}
	return us(h.max)
}

// Mean is the average.
func (h *Histogram) Mean() time.Duration {
	if h.n == 0 {
		return 0
	}
	return us(h.sum / h.n)
}

// Min and Max are exact.
func (h *Histogram) Min() time.Duration { return us(h.min) }
func (h *Histogram) Max() time.Duration { return us(h.max) }

func us(v uint64) time.Duration { return time.Duration(v) * time.Microsecond }
