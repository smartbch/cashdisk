package webdavledger

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/net/webdav"

	"github.com/smartbch/cashdisk/config"
	"github.com/smartbch/cashdisk/types"
)

type DiskService struct {
	cfg       *config.Config
	listenUrl string

	db      *badger.DB
	workDir string
}

func NewDiskService(url string) *DiskService {
	d := &DiskService{
		cfg:       &config.Config{},
		listenUrl: url,
	}
	return d
}

func (d *DiskService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//check basic auth
	addr, errStr := authFunc(d.db, w, r)
	if len(errStr) != 0 {
		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, errStr, http.StatusUnauthorized)
		return
	}

	// read uid
	uid := types.GetUID(d.db, addr)
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
			Dir: webdav.Dir(path.Join(d.workDir, username)),
			db:  d.db,
			uid: uid,
			ro:  false,
		}
		handler.ServeHTTP(w, r)
		return
	}

	// friend-shared directory
	friendName := parts[0]
	friendAddr := common.HexToAddress(friendName)
	friendUid := types.GetUID(d.db, friendAddr)
	if friendUid < 0 {
		http.Error(w, "Inconsistent Database", http.StatusInternalServerError)
		return
	}
	expireTime := getExpireTime(d.db, friendUid, uid, path.Join(parts[1:]...))
	if expireTime < time.Now().UnixNano() {
		http.Error(w, "Permission Denied", http.StatusBadRequest)
		return
	}
	handler.Prefix = friendName
	handler.FileSystem = &WatchedDir{
		Dir: webdav.Dir(path.Join(d.workDir, friendName)),
		db:  d.db,
		uid: friendUid,
		ro:  true,
	}
	handler.ServeHTTP(w, r)
}

func (d *DiskService) Run() {
	fmt.Printf("start disk service on %s\n", d.listenUrl)
	err := http.ListenAndServe(d.listenUrl, d)
	if err != nil {
		panic(err)
	}
}
