package webdavledger

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/net/webdav"

	"github.com/smartbch/cashdisk/types"
)

type WatchedDir struct {
	webdav.Dir
	db  *badger.DB
	uid int64
	ro  bool
}

var _ webdav.FileSystem = (*WatchedDir)(nil)

func (wd *WatchedDir) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	if wd.ro {
		return types.ErrReadOnly
	}
	if common.IsHexAddress(name) || name[0] == '/' && common.IsHexAddress(name[1:]) {
		return errors.New("in the root directory, EVM address cannot be used as directory name")
	}
	operation := fmt.Sprintf("Mkdir '%s'", name)
	err := consumePoints(wd.db, wd.uid, types.PointsOfMkdir, operation)
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
		return types.ErrReadOnly
	}
	operation := fmt.Sprintf("Rename '%s' to '%s'", oldName, newName)
	err := consumePoints(wd.db, wd.uid, types.PointsOfRename, operation)
	if err != nil {
		return err
	}
	return wd.Dir.Rename(ctx, oldName, newName)
}

func (wd *WatchedDir) Stat(ctx context.Context, name string) (fi os.FileInfo, err error) {
	operation := fmt.Sprintf("Stat '%s'", name)
	err = consumePoints(wd.db, wd.uid, types.PointsPerFileInfo, operation)
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
		return 0, types.ErrReadOnly
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
		err = consumePoints(wf.db, wf.uid, int64(len(res))*types.PointsPerFileInfo, operation)
	}
	return res, err
}

func (wf *WatchedFile) Stat() (fs.FileInfo, error) {
	res, err := wf.File.Stat()
	if err == nil {
		operation := fmt.Sprintf("Stat '%s'", wf.name)
		err = consumePoints(wf.db, wf.uid, types.PointsPerFileInfo, operation)
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
