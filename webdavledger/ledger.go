package webdavledger

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/net/webdav"
)

const (
	RemainedPoints = byte(100) // key: RemainedPoints + uid, value: 8-byte int64
	DeductPoints   = byte(101) // key: DeductPoints + uid + timestamp, value: 8-byte int64 + operation
	AddPoints      = byte(102) // key: AddPoints + uid + timestamp, value: 8-byte int64 + txid
	PasswordHash   = byte(103) // key: PasswordHash + 20-byte address, value: 32-byte passwd hash
	SharedDir      = byte(104) // key: SharedDir + from-uid + to-uid + sha256(dir),
	                           // value: 8-byte expiretime + dir
	UserToId       = byte(105) // key: UserToId + 20-byte address, value: 8-byte uid
	IdToUser       = byte(106) // key: IdToUser + uid, value: 20-byte address
	
	PointsPerFileInfo = int64(30)
	PointsOfMkdir     = int64(200)
	PointsOfRename    = int64(150)

	ConsumeLogDuration = 30 * 24 * time.Hour
)

var (
	ErrReadOnly = errors.New("The shared directory is readonly")
)

func int64ToBytes(i int64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(i))
	return buf[:]
}

func bytesToInt64(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

func addFunc(existing, delta []byte) []byte {
	return int64ToBytes(bytesToInt64(existing) + bytesToInt64(delta))
}

func consumePoints(db *badger.DB, uid, points int64, operation string) error {
	key := append([]byte{RemainedPoints}, int64ToBytes(uid)...)
	m := db.GetMergeOperator(key, addFunc, 200*time.Millisecond)
	defer m.Stop()

	m.Add(int64ToBytes(-points))

	res, err := m.Get()
	if err != nil {
		return err
	}
	if bytesToInt64(res) < 0 {
		return errors.New("Not enough points after operation: "+operation)
	}

	key = append([]byte{DeductPoints}, int64ToBytes(uid)...)
	key = append(key, int64ToBytes(UniqTS.GetTimestamp())...)
	value := append(int64ToBytes(uid), operation...)
	return db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key, value).WithTTL(ConsumeLogDuration)
		return txn.SetEntry(e)
	})
}

type WatchedDir struct {
	webdav.Dir
	db  *badger.DB
	uid int64
	ro  bool
}

var _ webdav.FileSystem = (*WatchedDir)(nil)

func (wd *WatchedDir) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	if wd.ro {
		return ErrReadOnly
	}
	if common.IsHexAddress(name) || name[0] == '/' && common.IsHexAddress(name[1:]) {
		return errors.New("In the root directory, EVM address cannot be used as directory name")
	}
	operation := fmt.Sprintf("Mkdir '%s'", name)
	err := consumePoints(wd.db, wd.uid, PointsOfMkdir, operation)
	if err != nil {
		return err
	}
	return wd.Dir.Mkdir(ctx, name, perm)
}

func (wd *WatchedDir) OpenFile(ctx context.Context, name string, flag int,
	perm os.FileMode) (webdav.File, error) {
	f, err := wd.Dir.OpenFile(ctx, name, flag, perm)
	return &WatchedFile{File: f, db: wd.db, name: name, uid: wd.uid, ro: wd.ro}, err
}

func (wd *WatchedDir) Rename(ctx context.Context, oldName, newName string) error {
	if wd.ro {
		return ErrReadOnly
	}
	operation := fmt.Sprintf("Rename '%s' to '%s'", oldName, newName)
	err := consumePoints(wd.db, wd.uid, PointsOfRename, operation)
	if err != nil {
		return err
	}
	return wd.Dir.Rename(ctx, oldName, newName)
}

func (wd *WatchedDir) Stat(ctx context.Context, name string) (fi os.FileInfo, err error) {
	operation := fmt.Sprintf("Stat '%s'", name)
	err = consumePoints(wd.db, wd.uid, PointsPerFileInfo, operation)
	if err != nil {
		return
	}
	return wd.Dir.Stat(ctx, name)
}

var _ webdav.File = (*WatchedFile)(nil)

type WatchedFile struct {
	webdav.File
	db   *badger.DB
	uid  int64
	name string
	ro   bool
}

func (wf *WatchedFile) Write(p []byte) (n int, err error) {
	if wf.ro {
		return 0, ErrReadOnly
	}
	operation := fmt.Sprintf("Write to '%s' for %d bytes", wf.name, len(p))
	err = consumePoints(wf.db, wf.uid, int64((len(p)+1023)/1024), operation)
	if err != nil {
		return 0, err
	}
	return wf.File.Write(p)
}

func (wf *WatchedFile) Readdir(count int) ([]fs.FileInfo, error) {
	res, err := wf.File.Readdir(count)
	if err == nil {
		operation := fmt.Sprintf("Read dir '%s' for %d entries", wf.name, len(res))
		err = consumePoints(wf.db, wf.uid, int64(len(res))*PointsPerFileInfo, operation)
	} 
	return res, err
}

func (wf *WatchedFile) Stat() (fs.FileInfo, error) {
	res, err := wf.File.Stat()
	if err == nil {
		operation := fmt.Sprintf("Stat '%s'", wf.name)
		err = consumePoints(wf.db, wf.uid, PointsPerFileInfo, operation)
	} 
	return res, err
}

func (wf *WatchedFile) Read(p []byte) (n int, err error) {
	n, err = wf.File.Read(p)
	if err == nil {
		operation := fmt.Sprintf("Read '%s' for %d bytes", wf.name, n)
		err = consumePoints(wf.db, wf.uid, int64((n+1023)/1024), operation)
	} 
	return n, err
}

// ========================================================

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
	key := append([]byte{PasswordHash}, addr[:]...)
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
		return addr, "No such user: "+username
	}

	passwordHash := sha256.Sum256([]byte(password))
	if subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 0 {
		return addr, "Incorrect password"
	}
	return addr, ""
}

func getUID(db *badger.DB, addr common.Address) (uid int64) {
	key := append([]byte{UserToId}, addr[:]...)
	err := db.View(func(txn *badger.Txn) (err error) {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			uid = bytesToInt64(val)
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
	key[0] = SharedDir
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
			expireTime = bytesToInt64(val[:8])
			return nil
		})
	})
	if err != nil {
		return -1
	}
	return
}

func NewHandler(db *badger.DB, workDir string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//check basic auth
		addr, errStr := authFunc(db, w, r)
		if len(errStr) != 0 {
			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, errStr, http.StatusUnauthorized)
			return
		}

		// read uid
		uid := getUID(db, addr)
		if uid < 0 {
			http.Error(w, "Inconsistent Database", http.StatusInternalServerError)
			return
		}

		// create webdav handler
		handler := &webdav.Handler{LockSystem: &DummyLockSystem{}}
		username, _, _ := r.BasicAuth()
		parts := strings.Split(r.URL.Path, "/")
		accessUserOwnedDir := len(parts) == 0 || 
			(len(parts) > 0 && (parts[0] == username || !common.IsHexAddress(parts[0])))
		if accessUserOwnedDir {
			if parts[0] == username {
				handler.Prefix = username
			}
			handler.FileSystem = &WatchedDir{
				Dir: webdav.Dir(path.Join(workDir, username)),
				db:  db,
				uid: uid,
				ro:  false,
			}
			handler.ServeHTTP(w, r)
			return
		}

		// friend-shared directory
		friendName := parts[0]
		friendAddr := common.HexToAddress(friendName)
		friendUid := getUID(db, friendAddr)
		if friendUid < 0 {
			http.Error(w, "Inconsistent Database", http.StatusInternalServerError)
			return
		}
		expireTime := getExpireTime(db, friendUid, uid, path.Join(parts[1:]...))
		if expireTime < time.Now().UnixNano() {
			http.Error(w, "Permission Denied", http.StatusBadRequest)
			return
		}
		handler.Prefix = friendName
		handler.FileSystem = &WatchedDir{
			Dir: webdav.Dir(path.Join(workDir, friendName)),
			db:  db,
			uid: friendUid,
			ro:  true,
		}
		handler.ServeHTTP(w, r)
	})
}

