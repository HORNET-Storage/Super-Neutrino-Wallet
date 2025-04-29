package walletstatedb

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet"
)

const (
	LastScannedBlockKey    = "last_scanned_block_height"
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
	MinAvailableAddresses  = 10
	UnsentTransactionsTree = "unsent_transactions"
	ChallengeTreeName      = "challenges"
)

// Helper wrapper functions that redirect to SQLite implementations

func RetrieveAddresses() ([]btcutil.Address, []btcutil.Address, error) {
	return RetrieveAddressesFromSQLite()
}

func PrintAndCopyReceiveAddresses() (Address, error) {
	return PrintAndCopyReceiveAddressesFromSQLite()
}

func SetLastScannedBlockHeight(height int32) error {
	return SetLastScannedBlockHeightInSQLite(height)
}

func GetLastScannedBlockHeight() (int32, error) {
	return GetLastScannedBlockHeightFromSQLite()
}

func UpdateLastScannedBlockHeight(height int32) error {
	return SetLastScannedBlockHeightInSQLite(height)
}

func GenerateNewAddresses(w *wallet.Wallet, count int) error {
	return GenerateNewAddressesInSQLite(w, count)
}

func EnsureMinimumAvailableAddresses(w *wallet.Wallet) error {
	return EnsureMinimumAvailableAddressesInSQLite(w)
}

func UpdateAddressUsage(transactions []map[string]interface{}) error {
	return UpdateAddressUsageInSQLite(transactions)
}

func SaveTransactionToDB(tx *wire.MsgTx) (chainhash.Hash, error) {
	return SaveTransactionToSQLiteDB(tx)
}

func SaveNewTransaction(tx *Transaction) error {
	return SaveNewTransactionToSQLite(tx)
}

func GetUnsentTransactions() ([]Transaction, error) {
	return GetUnsentTransactionsFromSQLite()
}

func ClearUnsentTransactions() error {
	return ClearUnsentTransactionsFromSQLite()
}

func GetUnsentTransactionsUsingSentToBackend() ([]Transaction, error) {
	return GetUnsentTransactionsFromSQLiteUsingSentToBackend()
}

func MarkTransactionsAsSent() error {
	return MarkTransactionsAsSentInSQLite()
}

func TransactionExists(txID string, vout uint32) (bool, error) {
	return TransactionExistsInSQLite(txID, vout)
}

// Challenge functions
func SaveChallenge(challenge Challenge) error {
	return SaveChallengeToSQLite(challenge)
}

func GetChallenge(hash string) (*Challenge, error) {
	return GetChallengeFromSQLite(hash)
}

func MarkChallengeAsUsed(hash string) error {
	return MarkChallengeAsUsedInSQLite(hash)
}

func ExpireOldChallenges() error {
	return ExpireOldChallengesInSQLite()
}
