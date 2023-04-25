package webdavledger

import (
	"time"

	"golang.org/x/net/webdav"
)

type DummyLockSystem struct {
}

func (_ DummyLockSystem) Confirm(now time.Time, name0, name1 string, conditions ...webdav.Condition) (release func(), err error) {
	err = webdav.ErrNotImplemented
	return
}

func (_ DummyLockSystem) Create(now time.Time, details webdav.LockDetails) (token string, err error) {
	err = webdav.ErrNotImplemented
	return
}

func (_ DummyLockSystem) Refresh(now time.Time, token string, duration time.Duration) (details webdav.LockDetails, err error) {
	err = webdav.ErrNotImplemented
	return
}

func (_ DummyLockSystem) Unlock(now time.Time, token string) error {
	return webdav.ErrNotImplemented
}


