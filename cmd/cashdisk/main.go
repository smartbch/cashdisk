package main

import (
	"flag"

	"github.com/smartbch/cashdisk/usermanager"
	"github.com/smartbch/cashdisk/webdavledger"
)

func main() {
	var userManagerUrl string
	var diskServiceRul string
	var bchRpcUrl string
	var dbPath string
	var receiverPubkeyHash string
	
	flag.StringVar(&userManagerUrl,
		"ul", "127.0.0.1:8082", "user manager service listen url")
	flag.StringVar(&diskServiceRul,
		"dl", "127.0.0.1:8083", "disk service listen url")
	flag.StringVar(&bchRpcUrl,
		"bu", "https://user:password@localhost:8333", "bch mainnet rpc url")
	flag.StringVar(&dbPath,
		"dp", "./db", "db path")
	flag.StringVar(&receiverPubkeyHash,
		"rh", "", "cash disk manager receiver pubkey hash in hex string")
	flag.Parse()

	m := usermanager.NewUserManager(userManagerUrl, bchRpcUrl, dbPath, receiverPubkeyHash)
	go m.Run()

	d := webdavledger.NewDiskService(diskServiceRul)
	go d.Run()

	select {}
}
