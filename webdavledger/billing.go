package webdavledger

import (
	"errors"
	"time"

	"github.com/dgraph-io/badger/v3"

	"github.com/smartbch/cashdisk/types"
	"github.com/smartbch/cashdisk/utils"
)

func consumePoints(db *badger.DB, uid, points int64, operation string) error {
	key := append([]byte{types.RemainedPoints}, utils.Int64ToBytes(uid)...)
	m := db.GetMergeOperator(key, utils.AddFunc, 200*time.Millisecond)
	defer m.Stop()

	m.Add(utils.Int64ToBytes(-points))

	res, err := m.Get()
	if err != nil {
		return err
	}
	if utils.BytesToInt64(res) < 0 {
		return errors.New("Not enough points after operation: " + operation)
	}

	key = append([]byte{types.DeductPoints}, utils.Int64ToBytes(uid)...)
	key = append(key, utils.Int64ToBytes(UniqTS.GetTimestamp())...)
	value := append(utils.Int64ToBytes(uid), operation...)
	return db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key, value).WithTTL(types.ConsumeLogDuration)
		return txn.SetEntry(e)
	})
}
