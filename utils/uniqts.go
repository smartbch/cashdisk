package utils

import (
	"sync/atomic"
	"time"
)

var UniqTS UniqueTimestamp

type UniqueTimestamp struct {
	t atomic.Int64
}

func GetTimestamp() int64 {
	newTs := time.Now().UnixNano()
	oldTs := UniqTS.t.Load()
	if oldTs+int64(200*time.Millisecond) < newTs { // need to sync to latest
		swapped := UniqTS.t.CompareAndSwap(oldTs, newTs)
		if swapped { // it'll be true if no other goroutine is trying CompareAndSwap
			return newTs
		}
	}
	return UniqTS.t.Add(1)
}
