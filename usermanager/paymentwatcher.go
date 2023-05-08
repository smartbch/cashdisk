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
	var stillPendingTxInfos []*types.PendingPaymentInfo
	var pendingTxInfos []*types.PendingPaymentInfo
	for {
		u.lock.RLock()
		pendingTxInfos = u.pendingPaymentCache
		u.lock.RUnlock()
		for _, p := range pendingTxInfos {
			now := time.Now().Nanosecond()
			hash, _ := chainhash.NewHash(p.Txid[:])
			res, _ := u.bchClient.GetTransaction(hash)
			if res.Confirmations > 0 {
				// pending tx is finalized, update db
				err := types.UpdateAddPointRecord(u.DB, p.Uid, p.Timestamp, types.TxFinalized, p.Txid, p.Value)
				if err != nil {
					panic(err)
				}
				err = types.UpdatePoints(u.DB, p.Uid, p.Value)
				if err != nil {
					panic(err)
				}
			} else if p.Timestamp > int64(now)+timeToMakeTxDead {
				// pending tx is dead, update db
				err := types.UpdateAddPointRecord(u.DB, p.Uid, p.Timestamp, types.TxDead, p.Txid, p.Value)
				if err != nil {
					panic(err)
				}
			} else {
				stillPendingTxInfos = append(stillPendingTxInfos, p)
			}
		}
		u.lock.Lock()
		u.pendingPaymentCache = stillPendingTxInfos
		u.lock.Unlock()
		stillPendingTxInfos = nil
		time.Sleep(30 * time.Second)
	}
}
