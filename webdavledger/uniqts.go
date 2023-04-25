package webdavledger

import (
	"sync/atomic"
	"time"
)

var UniqTS UniqueTimestamp

type UniqueTimestamp struct {
	t atomic.Int64
}

func (ut *UniqueTimestamp) GetTimestamp() int64 {
	newTs := time.Now().UnixNano()
	oldTs := ut.t.Load()
	if oldTs + int64(200 * time.Millisecond) < newTs { // need to sync to latest
		swapped := ut.t.CompareAndSwap(oldTs, newTs)
		if swapped { // it'll be true if no other goroutine is trying CompareAndSwap
			return newTs
		}
	}
	return ut.t.Add(1)
}
