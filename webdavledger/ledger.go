package webdavledger

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	"golang.org/x/net/webdav"
)

const (
	RemainedPoints = byte(100) // key: RemainedPoints + uid
	DeductPoints   = byte(101) // key: DeductPoints + uid + timestamp
	AddPoints      = byte(102) // key: AddPoints + uid + timestamp
	PasswordHash   = byte(103) // key: PasswordHash + 20-byte address
	SharedDir      = byte(104) // key: SharedDir + from-uid + to-uid + dir hash
	UserToId       = byte(105) // key: UserToId + 20-byte address
	IdToUser       = byte(106) // key: IdToUser + uid
	
	PointsPerFileInfo = int64(30)
	PointsOfMkdir     = int64(200)
	PointsOfRename    = int64(150)
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
		return txn.Set(key, value)
	})
}

type WatchedDir struct {
	dir webdav.Dir
	db  *badger.DB
	uid int64
}

var _ webdav.FileSystem = (*WatchedDir)(nil)

func (wd *WatchedDir) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	operation := fmt.Sprintf("Mkdir '%s'", name)
	err := consumePoints(wd.db, wd.uid, PointsOfMkdir, operation)
	if err != nil {
		return err
	}
	return wd.dir.Mkdir(ctx, name, perm)
}

func (wd *WatchedDir) OpenFile(ctx context.Context, name string, flag int,
	perm os.FileMode) (webdav.File, error) {
	f, err := wd.dir.OpenFile(ctx, name, flag, perm)
	return &WatchedFile{file: f, db: wd.db, name: name, uid: wd.uid}, err
}

func (wd *WatchedDir) RemoveAll(ctx context.Context, name string) error {
	return wd.dir.RemoveAll(ctx, name)
}

func (wd *WatchedDir) Rename(ctx context.Context, oldName, newName string) error {
	operation := fmt.Sprintf("Rename '%s' to '%s'", oldName, newName)
	err := consumePoints(wd.db, wd.uid, PointsOfRename, operation)
	if err != nil {
		return err
	}
	return wd.dir.Rename(ctx, oldName, newName)
}

func (wd *WatchedDir) Stat(ctx context.Context, name string) (fi os.FileInfo, err error) {
	operation := fmt.Sprintf("Stat '%s'", name)
	err = consumePoints(wd.db, wd.uid, PointsPerFileInfo, operation)
	if err != nil {
		return
	}
	return wd.dir.Stat(ctx, name)
}

var _ webdav.File = (*WatchedFile)(nil)

type WatchedFile struct {
	file webdav.File
	db   *badger.DB
	uid  int64
	name string
}

func (wf *WatchedFile) Write(p []byte) (n int, err error) {
	operation := fmt.Sprintf("Write to '%s' for %d bytes", wf.name, len(p))
	err = consumePoints(wf.db, wf.uid, int64((len(p)+1023)/1024), operation)
	if err != nil {
		return 0, err
	}
	return wf.file.Write(p)
}

func (wf *WatchedFile) Readdir(count int) ([]fs.FileInfo, error) {
	res, err := wf.file.Readdir(count)
	if err == nil {
		operation := fmt.Sprintf("Read dir '%s' for %d entries", wf.name, len(res))
		err = consumePoints(wf.db, wf.uid, int64(len(res))*PointsPerFileInfo, operation)
	} 
	return res, err
}

func (wf *WatchedFile) Stat() (fs.FileInfo, error) {
	res, err := wf.file.Stat()
	if err == nil {
		operation := fmt.Sprintf("Stat '%s'", wf.name)
		err = consumePoints(wf.db, wf.uid, PointsPerFileInfo, operation)
	} 
	return res, err
}

func (wf *WatchedFile) Close() error {
	return wf.file.Close()
}

func (wf *WatchedFile) Read(p []byte) (n int, err error) {
	n, err = wf.file.Read(p)
	if err == nil {
		operation := fmt.Sprintf("Read '%s' for %d bytes", wf.name, n)
		err = consumePoints(wf.db, wf.uid, int64((n+1023)/1024), operation)
	} 
	return n, err
}

func (wf *WatchedFile) Seek(offset int64, whence int) (int64, error) {
	return wf.file.Seek(offset, whence)
}

