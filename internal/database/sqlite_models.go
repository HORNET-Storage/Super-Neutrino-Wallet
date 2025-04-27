package walletstatedb

import (
	"time"

	"gorm.io/gorm"
)

// SQLiteAddress represents an address in the wallet
type SQLiteAddress struct {
	gorm.Model
	Index       uint       `gorm:"uniqueIndex"`
	Address     string     `gorm:"uniqueIndex"`
	Status      string     `gorm:"index"` // available, allocated, used
	AllocatedAt *time.Time
	UsedAt      *time.Time
	BlockHeight *uint32
	AddrType    string     `gorm:"index"` // receive or change
}

// SQLiteTransaction represents a transaction in the wallet
type SQLiteTransaction struct {
	gorm.Model
	TxID          string     `gorm:"uniqueIndex:idx_txid_vout"`
	WalletName    string     `gorm:"index"`
	Address       string     `gorm:"index"`
	Output        string
	Value         string
	Date          time.Time  `gorm:"index"`
	BlockHeight   *int32     `gorm:"index"`
	Vout          uint32     `gorm:"uniqueIndex:idx_txid_vout"`
	SentToBackend bool       `gorm:"index"`
	RawTx         []byte     // storing the serialized transaction
}

// SQLiteRawTransaction stores raw transaction data by hash
type SQLiteRawTransaction struct {
	gorm.Model
	TxHash string `gorm:"uniqueIndex"`
	RawTx  []byte
}

// SQLiteChallenge represents an auth challenge
type SQLiteChallenge struct {
	gorm.Model
	Challenge string    `gorm:"uniqueIndex"`
	Hash      string    `gorm:"uniqueIndex"`
	Status    string    `gorm:"index"` // unused, used, expired
	Npub      string    `gorm:"index"`
	CreatedAt time.Time `gorm:"index"`
	UsedAt    *time.Time
	ExpiredAt *time.Time
}

// SQLiteMetadata stores miscellaneous metadata about the wallet
type SQLiteMetadata struct {
	gorm.Model
	Key   string `gorm:"uniqueIndex"`
	Value string
}

// SQLiteUnsentTransaction tracks transactions that need to be sent
type SQLiteUnsentTransaction struct {
	gorm.Model
	TransactionID uint `gorm:"index"`
}