package ticker

import (
	"math/rand"
	"time"
)

// Ticker tries to emit events on channel C at minDuration intervals plus up to maxJitter.
type Ticker struct {
	C          chan time.Time
	baseTicker *time.Ticker
	Duration   time.Duration
	MaxJitter  time.Duration
}

func NewTicker(interval time.Duration, maxJitter time.Duration) *Ticker {
	if interval < 0 {
		panic("non-positive interval for NewTicker")
	}
	if maxJitter < 0 {
		panic("non-positive jitter for NewTicker")
	}
	baseTicker := time.NewTicker(interval)
	ticker := &Ticker{
		C:          make(chan time.Time, 1),
		baseTicker: baseTicker,
		Duration:   interval,
		MaxJitter:  maxJitter,
	}
	go ticker.loop()
	return ticker
}

func (t *Ticker) loop() {
	for {
		select {
		case now, ok := <-t.baseTicker.C:
			if !ok {
				return
			}
			jitter := time.Duration(rand.Int63n(int64(t.MaxJitter)))
			time.Sleep(jitter)
			t.C <- now
			t.baseTicker.Reset(t.Duration)
		}
	}
}

func (t *Ticker) Stop() {
	t.baseTicker.Stop()
}

func (t *Ticker) Reset(interval time.Duration, maxJitter time.Duration) {
	if interval < 0 {
		panic("non-positive interval for NewTicker")
	}
	if maxJitter < 0 {
		panic("non-positive jitter for NewTicker")
	}

	t.Duration = interval
	t.MaxJitter = maxJitter
	t.baseTicker.Reset(interval)
}
