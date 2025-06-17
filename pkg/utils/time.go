package utils

import (
	"sync"
	"time"
)

var CNLoc = time.FixedZone("UTC", 8*60*60)

func MustParseCNTime(str string) time.Time {
	lastOpTime, _ := time.ParseInLocation("2006-01-02 15:04:05 -07", str+" +08", CNLoc)
	return lastOpTime
}

func NewDebounce(interval time.Duration) func(f func()) {
	var timer *time.Timer
	var lock sync.Mutex
	return func(f func()) {
		lock.Lock()
		defer lock.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(interval, f)
	}
}

func NewDebounce2(interval time.Duration, f func()) func() {
	var timer *time.Timer
	var lock sync.Mutex
	return func() {
		lock.Lock()
		defer lock.Unlock()
		if timer == nil {
			timer = time.AfterFunc(interval, f)
		}
		timer.Reset(interval)
	}
}

func NewThrottle(interval time.Duration) func(func()) {
	var lastCall time.Time
	var lock sync.Mutex
	return func(fn func()) {
		lock.Lock()
		defer lock.Unlock()

		now := time.Now()
		if now.Sub(lastCall) >= interval {
			lastCall = now
			go fn()
		}
	}
}

func NewThrottle2(interval time.Duration, fn func()) func() {
	var lastCall time.Time
	var lock sync.Mutex
	return func() {
		lock.Lock()
		defer lock.Unlock()

		now := time.Now()
		if now.Sub(lastCall) >= interval {
			lastCall = now
			go fn()
		}
	}
}
