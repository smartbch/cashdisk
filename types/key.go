package types

import "time"

const (
	RemainedPoints = byte(100) // key: RemainedPoints + uid, value: 8-byte int64
	DeductPoints   = byte(102) // key: DeductPoints + uid + timestamp, value: 8-byte int64 + operation
	AddPoints      = byte(104) // key: AddPoints + uid + timestamp, value: 8-byte int64 + txid
	PasswordHash   = byte(106) // key: PasswordHash + 20-byte address, value: 32-byte passwd hash
	SharedDir      = byte(108) // key: SharedDir + from-uid + to-uid + sha256(dir),
	// value: 8-byte expiretime + dir
	UserToId = byte(110) // key: UserToId + 20-byte address, value: 8-byte uid
	IdToUser = byte(112) // key: IdToUser + uid, value: 20-byte address

	PointsPerFileInfo = int64(30)
	PointsOfMkdir     = int64(200)
	PointsOfRename    = int64(150)

	ConsumeLogDuration = 30 * 24 * time.Hour
)
