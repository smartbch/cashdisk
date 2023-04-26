package usermanager

import (
	"fmt"
	"net/http"

	"github.com/dgraph-io/badger/v3"

	"github.com/smartbch/cashdisk/config"
)

type UserManager struct {
	cfg *config.Config

	listenUrl string
	Users     []*User // cache
	DB        *badger.DB
}

func NewUserManager(url string) *UserManager {
	m := &UserManager{
		cfg:       &config.Config{},
		listenUrl: url,
	}
	return m
}

func (u *UserManager) Run() {
	fmt.Printf("start user manager service on %s\n", u.listenUrl)
	mux := http.NewServeMux()
	registerHttpEndpoint(mux)
	err := http.ListenAndServe(u.listenUrl, mux)
	if err != nil {
		panic(err)
	}
}

func registerHttpEndpoint(mux *http.ServeMux) {
	// todo: add getsecrethash,buypoints,viewhistory,setpassword,sharedir endpoints here
}
