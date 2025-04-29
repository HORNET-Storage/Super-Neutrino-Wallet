package walletstatedb

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global SQLite database instance
var DB *gorm.DB

// InitSQLiteDB initializes the SQLite database
func InitSQLiteDB(dbPath string) error {
	var err error

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if dir != "." && dir != "" {
		if err := ensureDir(dir); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
	}

	// Configure GORM to be less verbose
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	}

	// Open the database
	DB, err = gorm.Open(sqlite.Open(dbPath), config)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// Auto-migrate schemas
	err = DB.AutoMigrate(
		&SQLiteAddress{},
		&SQLiteTransaction{},
		&SQLiteRawTransaction{},
		&SQLiteChallenge{},
		&SQLiteMetadata{},
		&SQLiteUnsentTransaction{},
	)
	if err != nil {
		return fmt.Errorf("failed to migrate database: %v", err)
	}

	log.Println("SQLite database initialized successfully")
	return nil
}

// ensureDir creates a directory if it doesn't exist
func ensureDir(dir string) error {
	// Import "os" at the top of the file
	return os.MkdirAll(dir, 0755)
}

// SaveAddressToSQLite saves an address to the SQLite database
func SaveAddressToSQLite(addrType string, address Address) error {
	sqliteAddr := SQLiteAddress{
		Index:         address.Index,
		Address:       address.Address,
		Status:        address.Status,
		AllocatedAt:   address.AllocatedAt,
		UsedAt:        address.UsedAt,
		BlockHeight:   address.BlockHeight,
		AddrType:      addrType,
		SentToBackend: address.SentToBackend,
	}

	return DB.Create(&sqliteAddr).Error
}

// GetAddressesFromSQLite retrieves addresses of a specific type from SQLite
func GetAddressesFromSQLite(addrType string) ([]Address, error) {
	var sqliteAddrs []SQLiteAddress

	result := DB.Where("addr_type = ?", addrType).Find(&sqliteAddrs)
	if result.Error != nil {
		return nil, result.Error
	}

	addresses := make([]Address, len(sqliteAddrs))
	for i, addr := range sqliteAddrs {
		addresses[i] = Address{
			Index:         addr.Index,
			Address:       addr.Address,
			Status:        addr.Status,
			AllocatedAt:   addr.AllocatedAt,
			UsedAt:        addr.UsedAt,
			BlockHeight:   addr.BlockHeight,
			SentToBackend: addr.SentToBackend,
		}
	}

	return addresses, nil
}

// GetUnusedAddressFromSQLite retrieves an unused address of a specific type from SQLite
func GetUnusedAddressFromSQLite(addrType string) (*Address, error) {
	var sqliteAddr SQLiteAddress

	result := DB.Where("addr_type = ? AND status = ?", addrType, AddressStatusAvailable).First(&sqliteAddr)
	if result.Error != nil {
		return nil, fmt.Errorf("no unused address found: %v", result.Error)
	}

	addr := Address{
		Index:         sqliteAddr.Index,
		Address:       sqliteAddr.Address,
		Status:        sqliteAddr.Status,
		AllocatedAt:   sqliteAddr.AllocatedAt,
		UsedAt:        sqliteAddr.UsedAt,
		BlockHeight:   sqliteAddr.BlockHeight,
		SentToBackend: sqliteAddr.SentToBackend,
	}

	return &addr, nil
}

// MarkAddressAsUsedInSQLite marks an address as used in SQLite
func MarkAddressAsUsedInSQLite(address string, blockHeight uint32) error {
	now := time.Now()

	result := DB.Model(&SQLiteAddress{}).
		Where("address = ?", address).
		Updates(map[string]interface{}{
			"status":       AddressStatusUsed,
			"used_at":      now,
			"block_height": blockHeight,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("address not found")
	}

	return nil
}

// AllocateAddressFromSQLite allocates an unused address from SQLite
func AllocateAddressFromSQLite(addrType string) (*Address, error) {
	var sqliteAddr SQLiteAddress

	// Use a transaction to ensure atomicity
	tx := DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Find an available address
	if err := tx.Where("addr_type = ? AND status = ?", addrType, AddressStatusAvailable).First(&sqliteAddr).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("no available addresses: %v", err)
	}

	// Mark it as allocated
	now := time.Now()
	if err := tx.Model(&sqliteAddr).Updates(map[string]interface{}{
		"status":       AddressStatusAllocated,
		"allocated_at": now,
	}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// Convert to the Address type
	addr := Address{
		Index:         sqliteAddr.Index,
		Address:       sqliteAddr.Address,
		Status:        AddressStatusAllocated,
		AllocatedAt:   &now,
		UsedAt:        sqliteAddr.UsedAt,
		BlockHeight:   sqliteAddr.BlockHeight,
		SentToBackend: sqliteAddr.SentToBackend,
	}

	return &addr, nil
}

// SetLastScannedBlockHeightInSQLite sets the last scanned block height in SQLite
func SetLastScannedBlockHeightInSQLite(height int32) error {
	var metadata SQLiteMetadata

	// Check if the key already exists
	result := DB.Where("key = ?", LastScannedBlockKey).First(&metadata)

	if result.Error == nil {
		// Update existing record
		return DB.Model(&metadata).Update("value", fmt.Sprintf("%d", height)).Error
	} else {
		// Create new record
		metadata = SQLiteMetadata{
			Key:   LastScannedBlockKey,
			Value: fmt.Sprintf("%d", height),
		}
		return DB.Create(&metadata).Error
	}
}

// GetLastScannedBlockHeightFromSQLite gets the last scanned block height from SQLite
func GetLastScannedBlockHeightFromSQLite() (int32, error) {
	var metadata SQLiteMetadata

	result := DB.Where("key = ?", LastScannedBlockKey).First(&metadata)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, result.Error
	}

	height, err := strconv.ParseInt(metadata.Value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("failed to parse block height: %v", err)
	}

	return int32(height), nil
}

// UpdateLastScannedBlockHeightInSQLite updates the last scanned block height in SQLite
func UpdateLastScannedBlockHeightInSQLite(height int32) error {
	return SetLastScannedBlockHeightInSQLite(height)
}

// RetrieveAddressesFromSQLite retrieves all addresses from the SQLite database
func RetrieveAddressesFromSQLite() ([]btcutil.Address, []btcutil.Address, error) {
	// Get receive addresses
	receiveAddrs, err := GetAddressesFromSQLite("receive")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get receive addresses: %v", err)
	}

	// Get change addresses
	changeAddrs, err := GetAddressesFromSQLite("change")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get change addresses: %v", err)
	}

	// Convert to btcutil.Address
	receiveAddresses := make([]btcutil.Address, len(receiveAddrs))
	for i, addr := range receiveAddrs {
		btcAddr, err := btcutil.DecodeAddress(addr.Address, nil)
		if err != nil {
			return nil, nil, err
		}
		receiveAddresses[i] = btcAddr
	}

	changeAddresses := make([]btcutil.Address, len(changeAddrs))
	for i, addr := range changeAddrs {
		btcAddr, err := btcutil.DecodeAddress(addr.Address, nil)
		if err != nil {
			return nil, nil, err
		}
		changeAddresses[i] = btcAddr
	}

	log.Printf("Retrieved %d receive addresses and %d change addresses from the database",
		len(receiveAddresses), len(changeAddresses))

	return receiveAddresses, changeAddresses, nil
}

// PrintAndCopyReceiveAddressesFromSQLite gets and prints the first receive address from SQLite
func PrintAndCopyReceiveAddressesFromSQLite() (Address, error) {
	var sqliteAddr SQLiteAddress

	if err := DB.Where("addr_type = ?", "receive").First(&sqliteAddr).Error; err != nil {
		return Address{}, fmt.Errorf("no receive addresses found: %v", err)
	}

	addr := Address{
		Index:         sqliteAddr.Index,
		Address:       sqliteAddr.Address,
		Status:        sqliteAddr.Status,
		AllocatedAt:   sqliteAddr.AllocatedAt,
		UsedAt:        sqliteAddr.UsedAt,
		BlockHeight:   sqliteAddr.BlockHeight,
		SentToBackend: sqliteAddr.SentToBackend,
	}

	return addr, nil
}

// GetLastAddressIndexFromSQLite gets the last address index for a specific type from SQLite
func GetLastAddressIndexFromSQLite(addrType string) (int, error) {
	var sqliteAddr SQLiteAddress

	if err := DB.Raw("SELECT * FROM sq_lite_addresses WHERE addr_type = ? AND deleted_at IS NULL ORDER BY \"index\" DESC LIMIT 1", addrType).Scan(&sqliteAddr).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return 0, nil
		}
		return 0, err
	}

	return int(sqliteAddr.Index), nil
}

// GenerateNewAddressesInSQLite generates new addresses and saves them to the SQLite database
func GenerateNewAddressesInSQLite(w *wallet.Wallet, count int) error {
	// Get last index
	lastIndex, err := GetLastAddressIndexFromSQLite("receive")
	if err != nil {
		return fmt.Errorf("error getting last address index: %v", err)
	}

	// Generate and save new addresses
	for i := 0; i < count; i++ {
		newAddr, err := w.NewAddress(0, waddrmgr.KeyScopeBIP0084)
		if err != nil {
			return fmt.Errorf("failed to generate new address: %v", err)
		}

		addr := Address{
			Index:         uint(lastIndex + i + 1),
			Address:       newAddr.EncodeAddress(),
			Status:        AddressStatusAvailable,
			SentToBackend: false,
		}

		if err := SaveAddressToSQLite("receive", addr); err != nil {
			return fmt.Errorf("failed to save new address: %v", err)
		}
	}

	return nil
}

// EnsureMinimumAvailableAddressesInSQLite ensures there are enough available addresses in SQLite
func EnsureMinimumAvailableAddressesInSQLite(w *wallet.Wallet) error {
	// Count available addresses
	var count int64
	if err := DB.Model(&SQLiteAddress{}).
		Where("addr_type = ? AND status = ?", "receive", AddressStatusAvailable).
		Count(&count).Error; err != nil {
		return err
	}

	// Generate more if needed
	if int(count) < MinAvailableAddresses {
		return GenerateNewAddressesInSQLite(w, MinAvailableAddresses-int(count))
	}

	return nil
}

// UpdateAddressUsageInSQLite marks addresses as used based on transactions in SQLite
func UpdateAddressUsageInSQLite(transactions []map[string]interface{}) error {
	for _, tx := range transactions {
		address := tx["output"].(string)
		blockHeight := uint32(tx["blockHeight"].(int))

		if err := MarkAddressAsUsedInSQLite(address, blockHeight); err != nil {
			return fmt.Errorf("failed to mark address as used: %v", err)
		}
	}

	return nil
}

// SaveRawTransactionToSQLite stores a raw transaction by its hash in SQLite
func SaveRawTransactionToSQLite(txHash string, rawTx []byte) error {
	txRecord := SQLiteRawTransaction{
		TxHash: txHash,
		RawTx:  rawTx,
	}

	return DB.Create(&txRecord).Error
}

// GetRawTransactionFromSQLite retrieves a raw transaction by its hash from SQLite
func GetRawTransactionFromSQLite(txHash string) ([]byte, error) {
	var txRecord SQLiteRawTransaction

	if err := DB.Where("tx_hash = ?", txHash).First(&txRecord).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("transaction %s not found", txHash)
		}
		return nil, err
	}

	return txRecord.RawTx, nil
}

// SaveTransactionToSQLiteDB serializes and stores a transaction in SQLite
func SaveTransactionToSQLiteDB(tx *wire.MsgTx) (chainhash.Hash, error) {
	// Serialize the transaction
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to serialize transaction: %v", err)
	}
	rawTx := buf.Bytes()

	// Save the raw transaction
	txHash := tx.TxHash().String()
	if err := SaveRawTransactionToSQLite(txHash, rawTx); err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to save raw transaction: %v", err)
	}

	hash, err := chainhash.NewHashFromStr(txHash)
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to parse hash string: %v", err)
	}

	return *hash, nil
}

// SaveChallengeToSQLite saves an authentication challenge to SQLite
func SaveChallengeToSQLite(challenge Challenge) error {
	sqliteChallenge := SQLiteChallenge{
		Challenge: challenge.Challenge,
		Hash:      challenge.Hash,
		Status:    challenge.Status,
		Npub:      challenge.Npub,
		CreatedAt: challenge.CreatedAt,
	}

	if !challenge.UsedAt.IsZero() {
		sqliteChallenge.UsedAt = &challenge.UsedAt
	}

	if !challenge.ExpiredAt.IsZero() {
		sqliteChallenge.ExpiredAt = &challenge.ExpiredAt
	}

	return DB.Create(&sqliteChallenge).Error
}

// GetChallengeFromSQLite retrieves a challenge by its hash from SQLite
func GetChallengeFromSQLite(hash string) (*Challenge, error) {
	var sqliteChallenge SQLiteChallenge

	if err := DB.Where("hash = ?", hash).First(&sqliteChallenge).Error; err != nil {
		return nil, err
	}

	challenge := Challenge{
		Challenge: sqliteChallenge.Challenge,
		Hash:      sqliteChallenge.Hash,
		Status:    sqliteChallenge.Status,
		Npub:      sqliteChallenge.Npub,
		CreatedAt: sqliteChallenge.CreatedAt,
	}

	if sqliteChallenge.UsedAt != nil {
		challenge.UsedAt = *sqliteChallenge.UsedAt
	}

	if sqliteChallenge.ExpiredAt != nil {
		challenge.ExpiredAt = *sqliteChallenge.ExpiredAt
	}

	return &challenge, nil
}

// MarkChallengeAsUsedInSQLite marks a challenge as used in SQLite
func MarkChallengeAsUsedInSQLite(hash string) error {
	now := time.Now()

	result := DB.Model(&SQLiteChallenge{}).
		Where("hash = ?", hash).
		Updates(map[string]interface{}{
			"status":  "used",
			"used_at": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("challenge not found")
	}

	return nil
}

// ExpireOldChallengesInSQLite marks expired challenges in SQLite
func ExpireOldChallengesInSQLite() error {
	now := time.Now()
	twoMinutesAgo := now.Add(-2 * time.Minute)

	return DB.Model(&SQLiteChallenge{}).
		Where("status = ? AND created_at < ?", "unused", twoMinutesAgo).
		Updates(map[string]interface{}{
			"status":     "expired",
			"expired_at": now,
		}).Error
}

// SaveNewTransactionToSQLite saves a transaction if it doesn't already exist to SQLite
func SaveNewTransactionToSQLite(tx *Transaction) error {
	// Check if transaction exists
	exists, err := TransactionExistsInSQLite(tx.TxID, tx.Vout)
	if err != nil {
		return fmt.Errorf("error checking transaction existence: %v", err)
	}

	if exists {
		log.Printf("Transaction %s:%d already exists, skipping", tx.TxID, tx.Vout)
		return nil
	}

	// Save transaction
	sqliteTx := SQLiteTransaction{
		TxID:          tx.TxID,
		WalletName:    tx.WalletName,
		Address:       tx.Address,
		Output:        tx.Output,
		Value:         tx.Value,
		Date:          tx.Date,
		BlockHeight:   tx.BlockHeight,
		Vout:          tx.Vout,
		SentToBackend: tx.SentToBackend,
	}

	// Begin a transaction
	dbTx := DB.Begin()

	// Save to main transaction table
	if err := dbTx.Create(&sqliteTx).Error; err != nil {
		dbTx.Rollback()
		return fmt.Errorf("failed to save transaction: %v", err)
	}

	// Save to unsent transactions table
	unsentTx := SQLiteUnsentTransaction{
		TransactionID: sqliteTx.ID,
	}

	if err := dbTx.Create(&unsentTx).Error; err != nil {
		dbTx.Rollback()
		return fmt.Errorf("failed to save unsent transaction: %v", err)
	}

	// Commit transaction
	if err := dbTx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	log.Printf("Saved new transaction %s:%d", tx.TxID, tx.Vout)
	return nil
}

// GetUnsentTransactionsFromSQLite retrieves all unsent transactions from SQLite
func GetUnsentTransactionsFromSQLite() ([]Transaction, error) {
	var unsentTxs []SQLiteUnsentTransaction
	var transactions []Transaction

	// Get IDs from unsent transactions table
	if err := DB.Find(&unsentTxs).Error; err != nil {
		return nil, err
	}

	// No unsent transactions
	if len(unsentTxs) == 0 {
		return transactions, nil
	}

	// Extract transaction IDs
	var txIDs []uint
	for _, tx := range unsentTxs {
		txIDs = append(txIDs, tx.TransactionID)
	}

	// Get transaction details
	var sqliteTxs []SQLiteTransaction
	if err := DB.Where("id IN ?", txIDs).Find(&sqliteTxs).Error; err != nil {
		return nil, err
	}

	// Convert to Transaction type
	for _, tx := range sqliteTxs {
		transactions = append(transactions, Transaction{
			TxID:          tx.TxID,
			WalletName:    tx.WalletName,
			Address:       tx.Address,
			Output:        tx.Output,
			Value:         tx.Value,
			Date:          tx.Date,
			BlockHeight:   tx.BlockHeight,
			Vout:          tx.Vout,
			SentToBackend: tx.SentToBackend,
		})
	}

	return transactions, nil
}

// ClearUnsentTransactionsFromSQLite removes all unsent transactions from SQLite
func ClearUnsentTransactionsFromSQLite() error {
	return DB.Where("1 = 1").Delete(&SQLiteUnsentTransaction{}).Error
}

// TransactionExistsInSQLite checks if a transaction exists in SQLite
func TransactionExistsInSQLite(txID string, vout uint32) (bool, error) {
	var count int64

	if err := DB.Model(&SQLiteTransaction{}).
		Where("tx_id = ? AND vout = ?", txID, vout).
		Count(&count).Error; err != nil {
		return false, err
	}

	return count > 0, nil
}

// No Graviton migration code needed anymore; SQLite is the only database backend

// GetUnsentAddressesFromSQLite retrieves addresses that haven't been sent to the backend
func GetUnsentAddressesFromSQLite() ([]Address, error) {
	var sqliteAddrs []SQLiteAddress

	result := DB.Where("addr_type = ? AND sent_to_backend = ?", "receive", false).Find(&sqliteAddrs)
	if result.Error != nil {
		return nil, result.Error
	}

	addresses := make([]Address, len(sqliteAddrs))
	for i, addr := range sqliteAddrs {
		addresses[i] = Address{
			Index:         addr.Index,
			Address:       addr.Address,
			Status:        addr.Status,
			AllocatedAt:   addr.AllocatedAt,
			UsedAt:        addr.UsedAt,
			BlockHeight:   addr.BlockHeight,
			SentToBackend: addr.SentToBackend,
		}
	}

	return addresses, nil
}

// MarkAddressesAsSentInSQLite marks addresses as sent to the backend
func MarkAddressesAsSentInSQLite() error {
	result := DB.Model(&SQLiteAddress{}).
		Where("addr_type = ? AND sent_to_backend = ?", "receive", false).
		Updates(map[string]interface{}{
			"sent_to_backend": true,
		})

	if result.Error != nil {
		return result.Error
	}

	log.Printf("Marked %d addresses as sent to backend", result.RowsAffected)
	return nil
}
