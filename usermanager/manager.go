package usermanager

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ReneKroon/ttlcache"
	"github.com/dgraph-io/badger/v3"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gcash/bchd/chaincfg"
	"github.com/gcash/bchd/rpcclient"
	"github.com/gcash/bchd/txscript"
	"github.com/gcash/bchd/wire"
	"github.com/gcash/bchutil"
	"github.com/smartbch/stochastic-pay/sdk"
	"github.com/tyler-smith/go-bip32"

	"github.com/smartbch/cashdisk/config"
	"github.com/smartbch/cashdisk/types"
	"github.com/smartbch/cashdisk/utils"
)

var (
	minExpirationBlocksInMainnet int64 = 10
	pointsPerBCHSatochi          int64 = 100_000_000
	minPointsWhenFirstBuy        int64 = 10_000_000
)

type UserManager struct {
	cfg *config.Config

	listenUrl string

	key *bip32.Key
	DB  *badger.DB

	bchClient   *rpcclient.Client
	receiverPkh [20]byte
	pkScript    []byte

	lock                sync.RWMutex
	pendingPaymentCache []*types.PendingPaymentInfo

	unSpentStochasticTxCache *ttlcache.Cache
}

func NewUserManager(listenUrl string, bchRpcUrl string, dbPath string, receiverPubkeyHash string) *UserManager {
	m := &UserManager{
		cfg:       &config.Config{},
		listenUrl: listenUrl,
	}
	client, err := utils.NewBchMainnetClient(bchRpcUrl)
	if err != nil {
		panic(err)
	}
	m.bchClient = client
	db, err := badger.Open(badger.DefaultOptions(dbPath))
	if err != nil {
		panic(err)
	}
	m.DB = db
	seed, err := bip32.NewSeed()
	if err != nil {
		panic(err)
	}
	key, err := bip32.NewMasterKey(seed)
	if err != nil {
		panic(err)
	}
	m.key = key
	hash, err := hex.DecodeString(receiverPubkeyHash)
	if err != nil {
		panic(err)
	}
	if len(hash) != 20 {
		panic("receiverPubkeyHash is not 20 bytes long")
	}
	copy(m.receiverPkh[:], hash)
	pkScript, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).
		AddData(m.receiverPkh[:]).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	if err != nil {
		panic(err)
	}
	m.pkScript = pkScript
	cache := ttlcache.NewCache()
	cache.SetTTL(5 * time.Minute)
	m.unSpentStochasticTxCache = cache
	return m
}

func (u *UserManager) Run() {
	fmt.Printf("start user manager service on %s\n", u.listenUrl)
	go u.StartPaymentWatcher()
	go u.StartDirScanRoutine()
	mux := http.NewServeMux()
	u.registerHttpEndpoint(mux)
	err := http.ListenAndServe(u.listenUrl, mux)
	if err != nil {
		panic(err)
	}
}

func (u *UserManager) registerHttpEndpoint(mux *http.ServeMux) {
	mux.HandleFunc("/getsecrethash", u.handleGetSecretHash)
	mux.HandleFunc("/buypoints", u.handleBuyPoints)
	mux.HandleFunc("/viewhistory", u.handleViewHistory)
	mux.HandleFunc("/setpassword", u.handleSetPassword)
	mux.HandleFunc("/sharedir", u.handleShareDir)
}

func (u *UserManager) handleGetSecretHash(w http.ResponseWriter, r *http.Request) {
	keyBz, err := u.key.Serialize()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	timestamp := utils.GetTimestamp()
	secret := sha256.Sum256(append(keyBz, utils.Int64ToBytes(timestamp)...))
	secretHash := bchutil.Hash160(secret[:])
	res := types.GetSecretHashRes{
		Hash:          secretHash,
		UniqTimestamp: timestamp,
	}
	out, _ := json.Marshal(res)
	w.Write(out)
	return
}

func (u *UserManager) handleBuyPoints(w http.ResponseWriter, r *http.Request) {
	var param types.BuyPointsParam
	body, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(body, &param)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("param parsed failed: " + err.Error()))
		return
	}
	err = u.handleBuyPointsAndAddUser(&param)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("handle buy points failed: " + err.Error()))
		return
	}
	w.Write([]byte("success"))
	return
}

func (u *UserManager) handleBuyPointsAndAddUser(param *types.BuyPointsParam) error {
	sig := param.Sig
	param.Sig = nil
	out, _ := json.Marshal(param)
	hash := sha256.Sum256(out)
	user, err := utils.GetAddressAndCheckSig(hash, sig)
	if err != nil {
		return err
	}
	var zeroAddress = [20]byte{}
	if param.FriendAddress != zeroAddress {
		// pay for friend
		user = param.FriendAddress
	}
	uid := types.GetUID(u.DB, user)
	var isNewUser bool
	if uid < 0 {
		uid = types.AddressToUID(u.DB, user)
		isNewUser = true
	}
	if param.IsMainnetTx {
		var tx wire.MsgTx
		err := tx.Deserialize(bytes.NewReader(param.Tx))
		if err != nil {
			return err
		}
		err = u.handleMainnetUserPayment(user, uid, &tx, isNewUser, param)
		if err != nil {
			return err
		}
	} else {
		panic("not support side chain payment temp")
	}
	if isNewUser {
		err := types.AddNewUser(u.DB, user, uid, param.PasswordHash)
		if err != nil {
			return err
		}
	}
	return types.ConsumePoints(u.DB, uid, types.PointsOfUserManagerAccess, "buyPoints")
}

func (u *UserManager) handleMainnetUserPayment(user common.Address, uid int64, tx *wire.MsgTx, isNewUser bool, param *types.BuyPointsParam) error {
	amount := int64(0)
	isLocked := false
	balance := int64(0)
	if !isNewUser {
		var err error
		isLocked, balance, err = types.IsUserLock(u.DB, uid)
		if err != nil {
			panic(err)
		}
	}
	if param.Expiration == 0 {
		// this is a normal bch mainnet transfer tx
		for _, out := range tx.TxOut {
			if bytes.Equal(out.PkScript, u.pkScript) {
				amount += out.Value
				break
			}
		}
		if isLocked {
			if amount+balance <= 0 {
				return errors.New("amount is not enough to positive the balance")
			}
		}
	} else {
		// this is a stochastic tx
		if param.Timestamp < time.Now().UnixNano()-5*int64(time.Minute) {
			return errors.New("timestamp is too old for stochastic pay")
		}
		txid := tx.TxHash()
		_, exist := u.unSpentStochasticTxCache.Get(txid.String())
		if exist {
			return errors.New("tx already used prev time")
		}
		latestBlock, _ := u.bchClient.GetBlockCount()
		if param.Expiration < latestBlock+minExpirationBlocksInMainnet {
			return errors.New("expiration is too small")
		}
		// build the p2sh address from param
		keyBz, err := u.key.Serialize()
		if err != nil {
			panic(err)
		}
		secret := sha256.Sum256(append(keyBz, utils.Int64ToBytes(param.Timestamp)...))
		var secretHash [20]byte
		copy(secretHash[:], bchutil.Hash160(secret[:]))
		covenant, _ := sdk.NewMainnetCovenant(param.SenderPkh, u.receiverPkh, secretHash, param.Salt, param.Expiration, param.Probability)
		scriptHash, err := covenant.GetRedeemScriptHash()
		if err != nil {
			return err
		}
		address, err := bchutil.NewAddressScriptHashFromHash(scriptHash, &chaincfg.MainNetParams)
		if err != nil {
			return err
		}
		pkScript, err := txscript.PayToAddrScript(address)
		if err != nil {
			return err
		}
		for _, out := range tx.TxOut {
			if bytes.Equal(out.PkScript, pkScript) {
				amount = out.Value
				break
			}
		}
		if isLocked {
			probabilityInRatio := (balance / -1000_000) / 10
			probability := sdk.GetProbabilityByRatio(float64(probabilityInRatio))
			if probability != param.Probability || amount != 10_000_000 {
				return errors.New("probability or amount is not match when user is locked")
			}
		}
	}
	if amount == 0 {
		return errors.New("not pay any bch to me")
	}
	points := amount * pointsPerBCHSatochi
	if isNewUser && points < minPointsWhenFirstBuy {
		return errors.New(fmt.Sprintf("must buy at least %d points when first buy", minPointsWhenFirstBuy))
	}
	// todo: make sure below code return err if tx is repeat
	txHash, err := u.bchClient.SendRawTransaction(tx, false)
	if err != nil {
		return err
	}
	t := utils.GetTimestamp()
	err = types.AddAddPoints(u.DB, uid, t, points, *txHash)
	if err != nil {
		return err
	}
	u.unSpentStochasticTxCache.Set(txHash.String(), true)
	u.lock.Lock()
	u.pendingPaymentCache = append(u.pendingPaymentCache, &types.PendingPaymentInfo{
		Uid:       uid,
		Txid:      *txHash,
		Timestamp: t,
		Value:     points,
	})
	u.lock.Unlock()
	return nil
}

func (u *UserManager) handleViewHistory(w http.ResponseWriter, r *http.Request) {
	var param types.ViewHistoryParam
	body, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(body, &param)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("param parsed failed: " + err.Error()))
		return
	}
	sig := param.Sig
	param.Sig = nil
	out, _ := json.Marshal(param)
	hash := sha256.Sum256(out)
	user, err := utils.GetAddressAndCheckSig(hash, sig)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user address parsed failed: " + err.Error()))
		return
	}
	uid := types.GetUID(u.DB, user)
	if uid < 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user not register"))
		return
	}
	isLocked, _, err := types.IsUserLock(u.DB, uid)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("get user lock status error" + err.Error()))
		return
	}
	if isLocked {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user is locked"))
		return
	}
	err = types.ConsumePoints(u.DB, uid, types.PointsOfUserManagerAccess, "viewHistory")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("deduct points failed: " + err.Error()))
		return
	}
	startTime := param.BeginTimestamp
	endTime := param.EndTimestamp
	var res types.ViewHistoryRes
	err = u.DB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		//todo: collect DeductPoints also
		prefix := append([]byte{types.AddPoints}, utils.Int64ToBytes(uid)...)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			timestamp := utils.BytesToInt64(k[len(prefix)+1:])
			if timestamp > endTime || timestamp < startTime {
				continue
			}
			err := item.Value(func(v []byte) error {
				record := types.OperationRecord{
					Timestamp: timestamp,
					Amount:    utils.BytesToInt64(v[:8]),
					Operation: string(v[8:]), //todo: add tx finalized or pending or dead in Operation
				}
				res.Records = append(res.Records, record)
				return nil
			})
			if err != nil {
				continue
			}
		}
		return nil
	})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("view history failed: " + err.Error()))
		return
	}
	out, _ = json.Marshal(res)
	w.Write(out)
	return
}

func (u *UserManager) handleSetPassword(w http.ResponseWriter, r *http.Request) {
	var param types.SetPasswordHashParam
	body, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(body, &param)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("param parsed failed: " + err.Error()))
		return
	}
	sig := param.Sig
	param.Sig = nil
	out, _ := json.Marshal(param)
	hash := sha256.Sum256(out)
	user, err := utils.GetAddressAndCheckSig(hash, sig)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user address parsed failed: " + err.Error()))
		return
	}
	uid := types.GetUID(u.DB, user)
	if uid < 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user not register"))
		return
	}
	isLocked, _, err := types.IsUserLock(u.DB, uid)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("get user lock status error" + err.Error()))
		return
	}
	if isLocked {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user is locked"))
		return
	}
	err = types.ConsumePoints(u.DB, uid, types.PointsOfUserManagerAccess, "setPassword")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("deduct points failed: " + err.Error()))
		return
	}
	err = types.UpdateUserPasswordHash(u.DB, user, param.NewPasswordHash)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("set password failed: " + err.Error()))
		return
	}
	w.Write([]byte("success"))
	return
}

func (u *UserManager) handleShareDir(w http.ResponseWriter, r *http.Request) {
	var param types.ShareDirParam
	body, _ := io.ReadAll(r.Body)
	err := json.Unmarshal(body, &param)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("param parsed failed: " + err.Error()))
		return
	}
	sig := param.Sig
	param.Sig = nil
	out, _ := json.Marshal(param)
	hash := sha256.Sum256(out)
	user, err := utils.GetAddressAndCheckSig(hash, sig)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user address parsed failed: " + err.Error()))
		return
	}
	uid := types.GetUID(u.DB, user)
	if uid < 0 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user not register"))
		return
	}
	isLocked, _, err := types.IsUserLock(u.DB, uid)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("get user lock status error" + err.Error()))
		return
	}
	if isLocked {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("user is locked"))
		return
	}
	err = types.ConsumePoints(u.DB, uid, types.PointsOfUserManagerAccess, "shareDir")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("deduct points failed: " + err.Error()))
		return
	}
	fUid := types.GetUID(u.DB, param.Friend)
	if fUid < 0 {
		fUid = types.AddressToUID(u.DB, param.Friend)
		err = types.AddNewUser(u.DB, param.Friend, fUid, param.PasswordHash)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("add new user failed: " + err.Error()))
			return
		}
	}
	err = types.UpdateSharedDir(u.DB, uid, fUid, param.Dir, param.ExpiredTime)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("share directory failed: " + err.Error()))
		return
	}
	w.Write([]byte("success"))
	return
}
