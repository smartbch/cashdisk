package utils

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"sync/atomic"
)

type BloomLimiter struct {
	countersMap  sync.Map
	numCounters  int
	numFuncs     int
	numTimeSlots int
	salt         [12]byte
}

func NewBloomLimiter(numCounters, numFuncs, numTimeSlots int, salt [12]byte) *BloomLimiter {
	if numFuncs > 16 {
		panic("numFuncs is too large")
	}
	return &BloomLimiter{
		numCounters:  numCounters,
		numFuncs:     numFuncs,
		numTimeSlots: numTimeSlots,
		salt:         salt,
	}
}

// get the total count in all the timeslots
func (b *BloomLimiter) GetTotalCount(requestor []byte) (total uint32) {
	b.countersMap.Range(func(_, value any) bool {
		counters := value.([]uint32)
		hash := sha256.Sum256(append(b.salt[:], requestor[:]...))
		for i := 0; i < int(b.numFuncs); i++ {
			slotNum := binary.LittleEndian.Uint32(hash[i*4:i*4+4]) % uint32(len(counters))
			total += atomic.LoadUint32(&counters[slotNum])
		}
		return true //don't stop iteration
	})
	return
}

// incr the count in the current timeslot
func (b *BloomLimiter) IncrCount(timeSlot int, requestor []byte) (total uint32) {
	var counters []uint32
	if value, ok := b.countersMap.Load(timeSlot); ok {
		counters = value.([]uint32)
	} else { // if the current entry has not been allocated
		counters = make([]uint32, b.numCounters)
		b.countersMap.Store(timeSlot, counters)
	}
	b.countersMap.Delete(timeSlot - b.numTimeSlots) // delete an outdated entry
	hash := sha256.Sum256(append(b.salt[:], requestor[:]...))
	for i := 0; i < int(b.numFuncs); i++ {
		slotNum := binary.LittleEndian.Uint32(hash[i*4:i*4+4]) % uint32(len(counters))
		total += atomic.AddUint32(&counters[slotNum], 1)
	}
	return
}
