package utils

import (
	"net/url"

	"github.com/gcash/bchd/rpcclient"
)

func NewBchMainnetClient(rpcUrlStr string) (*rpcclient.Client, error) {
	rpcUrl, err := url.Parse(rpcUrlStr)
	if err != nil {
		return nil, err
	}
	pass, _ := rpcUrl.User.Password()
	connCfg := &rpcclient.ConnConfig{
		Host:         rpcUrl.Host,
		User:         rpcUrl.User.Username(),
		Pass:         pass,
		DisableTLS:   rpcUrl.Scheme == "http",
		HTTPPostMode: true,
	}
	return rpcclient.New(connCfg, nil)
}
