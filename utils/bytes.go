package utils

import "encoding/binary"

func Int64ToBytes(i int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(i))
	return buf[:]
}

func BytesToInt64(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

func AddFunc(existing, delta []byte) []byte {
	return Int64ToBytes(BytesToInt64(existing) + BytesToInt64(delta))
}
