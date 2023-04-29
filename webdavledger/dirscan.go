package webdavledger

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v3"
	"github.com/ethereum/go-ethereum/common"

	"github.com/smartbch/cashdisk/utils"
	"github.com/smartbch/cashdisk/types"
)

const (
	Mega = 1024*1024
)

func dirScan(db *badger.DB, workDir string, hash common.Hash, thres int64, logger *log.Logger,
	uid int64, addr common.Address) {
	dir := filepath.Join(workDir, addr.Hex())
	err := filepath.Walk(dir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			logger.Printf("Error in filepath.Walk: %s\n", err.Error())
		}
		size := int64(1)
		if !f.IsDir() {
			size = (f.Size() + Mega - 1)/Mega
		}
		hash := sha256.Sum256(append(hash[:], path...))
		n := utils.BytesToInt64(hash[:8])
		if n < thres * size {
			operation := fmt.Sprintf("Storage: block=%s path=%s size=%d", hash, path, size)
			return types.ConsumePoints(db, uid, types.PointsForStorage, operation)
		}
		return nil
	})
	if err != nil {
		logger.Printf("Error in filepath.Walk: %s\n", err.Error())
	}
}

func DirScan(db *badger.DB, workDir string, hash common.Hash, thres int64, logger *log.Logger) {
	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte{types.UserToId}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			err := item.Value(func(v []byte) error {
				uid := utils.BytesToInt64(v)
				var addr common.Address
				copy(addr[:], k[1:])
				dirScan(db, workDir, hash, thres, logger, uid, addr)
				return nil
			})
			if err != nil {
				logger.Printf("Error in getting value of %#v\n", k)
			}
		}
		return nil
	})
	if err != nil {
		logger.Printf("Error in db.View: %s\n", err.Error())
	}
}
