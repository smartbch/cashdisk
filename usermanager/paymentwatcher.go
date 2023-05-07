package usermanager

import (
	"time"

	"github.com/gcash/bchd/chaincfg/chainhash"

	"github.com/smartbch/cashdisk/types"
)

var (
	timeToMakeTxDead = int64(200 * 10 * 60 * 1000 * 1000 * 1000)
)

func (u *UserManager) StartPaymentWatcher() {
	u.pendingPaymentCache = types.GetAllPendingTxInfo(u.DB)
	for {
		for _, p := range u.pendingPaymentCache {
			now := time.Now().Nanosecond()
			hash, _ := chainhash.NewHashFromStr(p.Txid)
			res, _ := u.bchClient.GetTransaction(hash)
			if res.Confirmations > 0 {
				// pending tx is finalized, update db
			} else if p.Timestamp > int64(now)+timeToMakeTxDead {
				// pending tx is dead, update db
			}
		}
		time.Sleep(30 * time.Second)
	}
}
