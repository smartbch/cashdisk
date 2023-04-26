package usermanager

import "github.com/smartbch/cashdisk/config"

type BillingMeter struct {
	cfg *config.Config
}

func (b *BillingMeter) Charge(username string, fixedFee int64, contentSize int64) error {
	if fixedFee != 0 {
		//todo: charge fixed fee
		return nil
	}
	// todo: charge contentSize related fee
	return nil
}
