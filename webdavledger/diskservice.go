package webdavledger

import (
	"fmt"
	"net/http"

	"github.com/smartbch/cashdisk/config"
)

type DiskService struct {
	cfg       *config.Config
	listenUrl string
}

func NewDiskService(url string) *DiskService {
	d := &DiskService{
		cfg:       &config.Config{},
		listenUrl: url,
	}
	return d
}

func (u *DiskService) Run() {
	fmt.Printf("start disk service on %s\n", u.listenUrl)
	mux := http.NewServeMux()
	err := http.ListenAndServe(u.listenUrl, mux)
	if err != nil {
		panic(err)
	}
}
