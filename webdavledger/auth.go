package webdavledger

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"net/http"

	"github.com/dgraph-io/badger/v3"
	"github.com/ethereum/go-ethereum/common"

	"github.com/smartbch/cashdisk/types"
	"github.com/smartbch/cashdisk/utils"
)

func authFunc(db *badger.DB, w http.ResponseWriter, r *http.Request) (addr common.Address, errStr string) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return addr, "Unauthorized"
	}

	if !common.IsHexAddress(username) {
		return addr, "Invalid username format"
	}

	addr = common.HexToAddress(username)
	expectedPasswordHash := make([]byte, 32)
	key := append([]byte{types.PasswordHash}, addr[:]...)
	err := db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			copy(expectedPasswordHash, val)
			return nil
		})
	})

	if err != nil {
		return addr, "No such user: " + username
	}

	passwordHash := sha256.Sum256([]byte(password))
	if subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 0 {
		return addr, "Incorrect password"
	}
	return addr, ""
}

func getUID(db *badger.DB, addr common.Address) (uid int64) {
	key := append([]byte{types.UserToId}, addr[:]...)
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

func getExpireTime(db *badger.DB, fromUid, toUid int64, dir string) (expireTime int64) {
	key := make([]byte, 1+8+8+32)
	key[0] = types.SharedDir
	binary.BigEndian.PutUint64(key[1:9], uint64(fromUid))
	binary.BigEndian.PutUint64(key[9:17], uint64(toUid))
	dirHash := sha256.Sum256([]byte(dir))
	copy(key[17:], dirHash[:])
	err := db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			expireTime = utils.BytesToInt64(val[:8])
			return nil
		})
	})
	if err != nil {
		return -1
	}
	return
}
