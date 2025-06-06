package walletstatedb

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet"
)

// DatabaseType represents the type of database backend to use
type DatabaseType string

const (
	// DBTypeSQLite represents the SQLite database backend
	DBTypeSQLite DatabaseType = "sqlite"
)

// DBBackend is the global variable that holds the active database backend
var DBBackend DatabaseType = DBTypeSQLite

// SetDatabaseBackend sets the database backend type
func SetDatabaseBackend(dbType DatabaseType) {
	DBBackend = dbType
}

// InitializeDatabase initializes the database using the specified backend
func InitializeDatabase(dbPath string) error {
	return InitSQLiteDB(dbPath)
}

// DatabaseInterface defines the interface for database operations
type DatabaseInterface interface {
	// Address operations
	SaveAddress(address Address) error
	GetAddresses() ([]Address, error)
	GetUnusedAddress() (*Address, error)
	MarkAddressAsUsed(address string, blockHeight uint32) error
	AllocateAddress() (*Address, error)
	GetLastAddressIndex() (int, error)

	// Block height operations
	SaveBlockHeight(height int32) error
	GetBlockHeight() (int32, error)

	// Transaction operations
	SaveRawTransaction(txHash string, rawTx []byte) error
	GetRawTransaction(txHash string) ([]byte, error)
	SaveTransactionToDB(tx *wire.MsgTx) (chainhash.Hash, error)
	SaveNewTransaction(tx *Transaction) error
	GetUnsentTransactions() ([]Transaction, error)
	ClearUnsentTransactions() error
	TransactionExists(txID string, vout uint32) (bool, error)

	// Challenge operations
	SaveChallenge(challenge Challenge) error
	GetChallenge(hash string) (*Challenge, error)
	MarkChallengeAsUsed(hash string) error
	ExpireOldChallenges() error

	// Address generation
	GenerateNewAddresses(w *wallet.Wallet, count int) error
	EnsureMinimumAvailableAddresses(w *wallet.Wallet) error

	// Address retrieval
	RetrieveAddresses() ([]btcutil.Address, []btcutil.Address, error)

	// Last scanned block height
	SetLastScannedBlockHeight(height int32) error
	GetLastScannedBlockHeight() (int32, error)
	UpdateLastScannedBlockHeight(height int32) error
}

// SaveAddress saves an address to the database
func SaveAddressWithType(addrType string, address Address) error {
	return SaveAddressToSQLite(addrType, address)
}

// GetAddressesWithType retrieves addresses of a specific type
func GetAddressesWithType(addrType string) ([]Address, error) {
	return GetAddressesFromSQLite(addrType)
}

// GetLastAddressIndexWithType gets the last address index for a specific type
func GetLastAddressIndexWithType(addrType string) (int, error) {
	return GetLastAddressIndexFromSQLite(addrType)
}
