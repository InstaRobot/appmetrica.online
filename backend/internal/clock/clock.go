// Package clock — единственный источник текущего времени. Код вне пакета не вызывает time.Now
// (правило forbidigo), чтобы тесты работали с детерминированным временем.
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

// System — реальные часы, время в UTC.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fixed — часы для тестов: время меняется только через Set и Advance. Безопасны для конкурентного использования.
type Fixed struct {
	mu  sync.Mutex
	now time.Time
}

func NewFixed(t time.Time) *Fixed { return &Fixed{now: t} }

func (f *Fixed) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fixed) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
