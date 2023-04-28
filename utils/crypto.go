package utils

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func GetAddressAndCheckSig(hash [32]byte, sig []byte) (common.Address, error) {
	pubkey, err := crypto.SigToPub(hash[:], sig)
	if err != nil {
		return common.Address{}, err
	}
	address := crypto.PubkeyToAddress(*pubkey)
	return address, nil
}
