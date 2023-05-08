package types

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"

	"github.com/OneOfOne/xxhash"
	"github.com/dgraph-io/badger/v3"
	"github.com/ethereum/go-ethereum/common"

	"github.com/smartbch/cashdisk/utils"
)

const (
	RemainedPoints = byte(100) // key: RemainedPoints + uid, value: 8-byte int64
	DeductPoints   = byte(102) // key: DeductPoints + uid + timestamp, value: 8-byte int64 + operation
	AddPoints      = byte(104) // key: AddPoints + uid + 0x01(finalized tx) or 0x02(pending tx) or 0x04(dead tx) + timestamp, value: 8-byte int64 + 32-byte txid
	PasswordHash   = byte(106) // key: PasswordHash + 20-byte address, value: 32-byte passwd hash
	SharedDir      = byte(108) // key: SharedDir + from-uid + to-uid + sha256(dir), value: 8-byte expiretime + dir
	UserToId       = byte(110) // key: UserToId + 20-byte address, value: 8-byte uid
	IdToUser       = byte(112) // key: IdToUser + uid, value: 20-byte address

	PointsPerFileInfo         = int64(30)
	PointsOfMkdir             = int64(200)
	PointsOfRename            = int64(150)
	PointsOfUserManagerAccess = int64(10)
	PointsForStorage          = int64(1000)

	ConsumeLogDuration = 30 * 24 * time.Hour

	TxFinalized byte = 0x01
	TxPending   byte = 0x02
	TxDead      byte = 0x04
)

func AddressToUID(db *badger.DB, addr common.Address) int64 {
	h := xxhash.New64()
	h.Write(addr.Bytes())
	h.Reset()
	uid := int64(h.Sum64())
	for {
		_, err := GetAddressByUID(db, uid)
		if err != nil {
			break
		}
		uid++
		if uid < 0 {
			uid = 0
		}
	}
	return uid
}

func AddNewUser(db *badger.DB, address common.Address, uid int64, passwordHash [32]byte) error {
	return db.Update(func(txn *badger.Txn) error {
		uidBz := utils.Int64ToBytes(uid)
		key := append([]byte{IdToUser}, uidBz...)
		err := txn.Set(key, address.Bytes())
		if err != nil {
			return err
		}
		key = append([]byte{UserToId}, address.Bytes()...)
		err = txn.Set(key, uidBz)
		if err != nil {
			return err
		}
		key = append([]byte{PasswordHash}, address.Bytes()...)
		return txn.Set(key, passwordHash[:])
	})
}

func GetAddressByUID(db *badger.DB, uid int64) (common.Address, error) {
	key := append([]byte{IdToUser}, utils.Int64ToBytes(uid)...)
	var res []byte
	err := db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		res, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return common.Address{}, err
	}
	return common.BytesToAddress(res), nil
}

func GetUID(db *badger.DB, addr common.Address) (uid int64) {
	key := append([]byte{UserToId}, addr[:]...)
	err := db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			uid = utils.BytesToInt64(val)
			return nil
		})
	})
	if err != nil {
		return -1
	}
	return
}

func UpdatePoints(db *badger.DB, uid int64, changeAmount int64) error {
	key := append([]byte{RemainedPoints}, utils.Int64ToBytes(uid)...)
	update := func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		balB, err := item.ValueCopy(nil)
		balance := utils.BytesToInt64(balB) + changeAmount
		if balance < 0 {
			return errors.New("balance not enough")
		}
		return txn.Set(key, utils.Int64ToBytes(balance))
	}
	return db.Update(update)
}

func getPointsUpdateHistory(uid, startTime, endTime int64) {
}

func UpdateUserPasswordHash(db *badger.DB, addr common.Address, passwordHash [32]byte) error {
	key := append([]byte{PasswordHash}, addr[:]...)
	update := func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err != nil {
			return err
		}
		return txn.Set(key, passwordHash[:])
	}
	return db.Update(update)
}

func UpdateSharedDir(db *badger.DB, fromUid, toUid int64, dir string, expiredTime int64) error {
	key := make([]byte, 1+8+8+32)
	key[0] = SharedDir
	binary.BigEndian.PutUint64(key[1:9], uint64(fromUid))
	binary.BigEndian.PutUint64(key[9:17], uint64(toUid))
	dirHash := sha256.Sum256([]byte(dir))
	copy(key[17:], dirHash[:])
	update := func(txn *badger.Txn) error {
		return txn.Set(key, append(utils.Int64ToBytes(expiredTime), dir...))
	}
	return db.Update(update)
}

func ConsumePoints(db *badger.DB, uid, points int64, operation string) error {
	key := append([]byte{RemainedPoints}, utils.Int64ToBytes(uid)...)
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

	key = append([]byte{DeductPoints}, utils.Int64ToBytes(uid)...)
	key = append(key, utils.Int64ToBytes(utils.GetTimestamp())...)
	value := append(utils.Int64ToBytes(uid), operation...)
	return db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key, value).WithTTL(ConsumeLogDuration)
		return txn.SetEntry(e)
	})
}

type PendingPaymentInfo struct {
	Uid       int64
	Txid      [32]byte
	Timestamp int64
	Value     int64
}

func GetAllPendingTxInfo(db *badger.DB) []*PendingPaymentInfo {
	var infos []*PendingPaymentInfo
	getter := func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := append([]byte{AddPoints})
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			isPending := k[1+8] == TxPending
			if !isPending {
				continue
			}
			err := item.Value(func(v []byte) error {
				info := PendingPaymentInfo{
					Uid:       utils.BytesToInt64(k[1:9]),
					Timestamp: utils.BytesToInt64(k[1+8+1:]),
					Value:     utils.BytesToInt64(v[:8]),
				}
				copy(info.Txid[:], v[8:])
				infos = append(infos, &info)
				return nil
			})
			if err != nil {
				continue
			}
		}
		return nil
	}
	err := db.View(getter)
	if err != nil {
		panic(err)
	}
	return infos
}

func AddAddPoints(db *badger.DB, uid, timestamp, value int64, txid [32]byte) error {
	add := func(txn *badger.Txn) error {
		key := append([]byte{AddPoints}, utils.Int64ToBytes(uid)...)
		t := utils.Int64ToBytes(timestamp)
		key = append(key, TxPending)
		key = append(key, t...)
		return txn.Set(key, append(utils.Int64ToBytes(value), txid[:]...))
	}
	return db.Update(add)
}

func UpdateAddPointRecord(db *badger.DB, uid, timestamp int64, txStatus byte, txid [32]byte, value int64) error {
	update := func(txn *badger.Txn) error {
		key := append([]byte{AddPoints}, utils.Int64ToBytes(uid)...)
		t := utils.Int64ToBytes(timestamp)
		oldKey := append(key, TxPending)
		oldKey = append(oldKey, t...)
		key = append(key, txStatus)
		key = append(key, t...)
		err := txn.Delete(oldKey)
		if err != nil {
			return err
		}
		return txn.Set(key, append(utils.Int64ToBytes(value), txid[:]...))
	}
	return db.Update(update)
}
