package types

type GetSecretHashRes struct {
	Hash          []byte `json:"hash"`
	UniqTimestamp int64  `json:"uniqTimestamp"`
}

type BuyPointsParam struct {
	Timestamp     int64    `json:"timestamp"`
	IsMainnetTx   bool     `json:"isMainnetTx"`
	Tx            []byte   `json:"tx"`
	PasswordHash  [32]byte `json:"passwordHash"`
	Salt          [4]byte  `json:"salt"`
	SenderPkh     [20]byte `json:"senderPkh"`
	Expiration    int64    `json:"expiration"`
	Probability   int64    `json:"probability"`
	FriendAddress [20]byte `json:"friendAddress"`
	Sig           []byte   `json:"signature"`
}

type ViewHistoryParam struct {
	BeginTimestamp int64  `json:"beginTimestamp"`
	EndTimestamp   int64  `json:"endTimestamp"`
	Sig            []byte `json:"signature"`
}

type OperationRecord struct {
	Timestamp int64  `json:"timestamp"`
	Amount    int64  `json:"amount"`
	Operation string `json:"operation"`
}

type ViewHistoryRes struct {
	Records []OperationRecord `json:"records"`
}

type SetPasswordHashParam struct {
	NewPasswordHash [32]byte `json:"newPasswordHash"`
	Sig             []byte   `json:"signature"`
}

type ShareDirParam struct {
	Friend       [20]byte `json:"friend"`
	PasswordHash [32]byte `json:"passwordHash"`
	Dir          string   `json:"dir"`
	ExpiredTime  int64    `json:"expiredTime"`
	Sig          []byte   `json:"signature"`
}
