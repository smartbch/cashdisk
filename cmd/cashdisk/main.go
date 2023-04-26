package main

import (
	"flag"

	"github.com/smartbch/cashdisk/usermanager"
	"github.com/smartbch/cashdisk/webdavledger"
)

func main() {
	var userManagerUrl string
	var diskServiceRul string
	flag.StringVar(&userManagerUrl, "ul", "127.0.0.1:8082", "user manager service listen url")
	flag.StringVar(&diskServiceRul, "dl", "127.0.0.1:8083", "disk service listen url")
	flag.Parse()

	m := usermanager.NewUserManager(userManagerUrl)
	go m.Run()
	d := webdavledger.NewDiskService(diskServiceRul)
	go d.Run()
	select {}
}
