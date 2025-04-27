package walletstatedb

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/deroproject/graviton"
)

// MigrateFromGravitonToSQLite migrates data from Graviton to SQLite
func MigrateFromGravitonToSQLite(gravitonDBPath, sqliteDBPath string) error {
	log.Printf("Starting migration from Graviton DB at %s to SQLite DB at %s", gravitonDBPath, sqliteDBPath)

	// Initialize SQLite DB
	if err := InitSQLiteDB(sqliteDBPath); err != nil {
		return fmt.Errorf("failed to initialize SQLite DB: %v", err)
	}

	// Initialize Graviton DB to read from it
	gravitonStore, err := graviton.NewDiskStore(gravitonDBPath)
	if err != nil {
		return fmt.Errorf("failed to open Graviton store: %v", err)
	}

	ss, err := gravitonStore.LoadSnapshot(0)
	if err != nil {
		return fmt.Errorf("failed to load Graviton snapshot: %v", err)
	}

	// Migrate addresses
	if err := migrateAddresses(ss); err != nil {
		return fmt.Errorf("failed to migrate addresses: %v", err)
	}

	// Migrate transactions
	if err := migrateTransactions(ss); err != nil {
		return fmt.Errorf("failed to migrate transactions: %v", err)
	}

	// Migrate challenges
	if err := migrateChallenges(ss); err != nil {
		return fmt.Errorf("failed to migrate challenges: %v", err)
	}

	// Migrate metadata (including last scanned block height)
	if err := migrateMetadata(ss); err != nil {
		return fmt.Errorf("failed to migrate metadata: %v", err)
	}

	log.Println("Migration completed successfully")
	return nil
}

func migrateAddresses(ss *graviton.Snapshot) error {
	// Migrate receive addresses
	receiveAddrTree, err := ss.GetTree("receive_addresses")
	if err != nil {
		return fmt.Errorf("failed to get receive addresses tree: %v", err)
	}

	receiveAddrs, err := getGravitonAddresses(receiveAddrTree)
	if err != nil {
		return fmt.Errorf("failed to get receive addresses: %v", err)
	}

	log.Printf("Migrating %d receive addresses", len(receiveAddrs))
	for _, addr := range receiveAddrs {
		if err := SaveAddressToSQLite("receive", addr); err != nil {
			return fmt.Errorf("failed to save receive address to SQLite: %v", err)
		}
	}

	// Migrate change addresses
	changeAddrTree, err := ss.GetTree("change_addresses")
	if err != nil {
		return fmt.Errorf("failed to get change addresses tree: %v", err)
	}

	changeAddrs, err := getGravitonAddresses(changeAddrTree)
	if err != nil {
		return fmt.Errorf("failed to get change addresses: %v", err)
	}

	log.Printf("Migrating %d change addresses", len(changeAddrs))
	for _, addr := range changeAddrs {
		if err := SaveAddressToSQLite("change", addr); err != nil {
			return fmt.Errorf("failed to save change address to SQLite: %v", err)
		}
	}

	return nil
}

func getGravitonAddresses(tree *graviton.Tree) ([]Address, error) {
	cursor := tree.Cursor()
	var addresses []Address
	
	for _, v, err := cursor.First(); err == nil; _, v, err = cursor.Next() {
		var addr Address
		if err := unmarshalGravitonValue(v, &addr); err != nil {
			return nil, err
		}
		addresses = append(addresses, addr)
	}
	
	return addresses, nil
}

func migrateTransactions(ss *graviton.Snapshot) error {
	// Migrate main transactions
	txTree, err := ss.GetTree("transactions")
	if err != nil {
		return fmt.Errorf("failed to get transactions tree: %v", err)
	}

	cursor := txTree.Cursor()
	var txCount int
	
	for k, v, err := cursor.First(); err == nil; k, v, err = cursor.Next() {
		txHash := string(k)
		
		// For raw transactions
		if isRawTransactionKey(txHash) {
			// Save as raw transaction
			if err := SaveRawTransactionToSQLite(txHash, v); err != nil {
				return fmt.Errorf("failed to save raw transaction: %v", err)
			}
		} else {
			// For structured transactions
			var tx Transaction
			if err := unmarshalGravitonValue(v, &tx); err != nil {
				return fmt.Errorf("failed to unmarshal transaction: %v", err)
			}
			
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
			
			if err := DB.Create(&sqliteTx).Error; err != nil {
				return fmt.Errorf("failed to save transaction to SQLite: %v", err)
			}
			
			txCount++
		}
	}
	
	log.Printf("Migrated %d transactions", txCount)
	
	// Migrate unsent transactions
	unsentTxTree, err := ss.GetTree(UnsentTransactionsTree)
	if err != nil {
		log.Printf("Unsent transactions tree not found, skipping")
		return nil
	}
	
	cursor = unsentTxTree.Cursor()
	var unsentCount int
	
	for _, v, err := cursor.First(); err == nil; _, v, err = cursor.Next() {
		var tx Transaction
		if err := unmarshalGravitonValue(v, &tx); err != nil {
			return fmt.Errorf("failed to unmarshal unsent transaction: %v", err)
		}
		
		// Find the transaction in SQLite
		var sqliteTx SQLiteTransaction
		if err := DB.Where("tx_id = ? AND vout = ?", tx.TxID, tx.Vout).First(&sqliteTx).Error; err != nil {
			log.Printf("Transaction %s:%d not found, creating", tx.TxID, tx.Vout)
			
			// Create it if not found
			sqliteTx = SQLiteTransaction{
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
			
			if err := DB.Create(&sqliteTx).Error; err != nil {
				return fmt.Errorf("failed to save transaction to SQLite: %v", err)
			}
		}
		
		// Create an unsent transaction record
		unsentTx := SQLiteUnsentTransaction{
			TransactionID: sqliteTx.ID,
		}
		
		if err := DB.Create(&unsentTx).Error; err != nil {
			return fmt.Errorf("failed to save unsent transaction to SQLite: %v", err)
		}
		
		unsentCount++
	}
	
	log.Printf("Migrated %d unsent transactions", unsentCount)
	
	return nil
}

func isRawTransactionKey(key string) bool {
	// This function would determine if a key in the transactions tree
	// belongs to a raw transaction or a structured transaction
	// For now, we'll use a simple heuristic: raw transactions are typically hex strings of length 64
	return len(key) == 64
}

func migrateChallenges(ss *graviton.Snapshot) error {
	challengeTree, err := ss.GetTree(ChallengeTreeName)
	if err != nil {
		log.Printf("Challenge tree not found, skipping")
		return nil
	}

	cursor := challengeTree.Cursor()
	var count int
	
	for _, v, err := cursor.First(); err == nil; _, v, err = cursor.Next() {
		var challenge Challenge
		if err := unmarshalGravitonValue(v, &challenge); err != nil {
			return fmt.Errorf("failed to unmarshal challenge: %v", err)
		}
		
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
		
		if err := DB.Create(&sqliteChallenge).Error; err != nil {
			return fmt.Errorf("failed to save challenge to SQLite: %v", err)
		}
		
		count++
	}
	
	log.Printf("Migrated %d challenges", count)
	
	return nil
}

func migrateMetadata(ss *graviton.Snapshot) error {
	metadataTree, err := ss.GetTree("metadata")
	if err != nil {
		log.Printf("Metadata tree not found, skipping")
		return nil
	}

	// Get last scanned block height
	lastScannedBytes, err := metadataTree.Get([]byte(LastScannedBlockKey))
	if err != nil && err != graviton.ErrNotFound {
		return fmt.Errorf("failed to get last scanned block height: %v", err)
	}

	if err == nil {
		// Save to SQLite
		metadata := SQLiteMetadata{
			Key:   LastScannedBlockKey,
			Value: string(lastScannedBytes),
		}
		
		if err := DB.Create(&metadata).Error; err != nil {
			return fmt.Errorf("failed to save metadata to SQLite: %v", err)
		}
		
		log.Printf("Migrated last scanned block height: %s", string(lastScannedBytes))
	}

	// Iterate through other metadata if any
	cursor := metadataTree.Cursor()
	var count int
	
	for k, v, err := cursor.First(); err == nil; k, v, err = cursor.Next() {
		key := string(k)
		if key == LastScannedBlockKey {
			continue // Already handled
		}
		
		metadata := SQLiteMetadata{
			Key:   key,
			Value: string(v),
		}
		
		if err := DB.Create(&metadata).Error; err != nil {
			return fmt.Errorf("failed to save metadata to SQLite: %v", err)
		}
		
		count++
	}
	
	log.Printf("Migrated %d additional metadata items", count)
	
	return nil
}

// Helper function to unmarshal Graviton values
func unmarshalGravitonValue(v []byte, target interface{}) error {
	// Graviton typically stores values as JSON in the current implementation
	return json.Unmarshal(v, target)
}

// RunMigration is a helper function to run the migration
func RunMigration(baseDir, walletName string) error {
	gravitonDBName := fmt.Sprintf("%s_wallet_graviton.db", walletName)
	sqliteDBName := fmt.Sprintf("%s_wallet.db", walletName)
	
	gravitonDBPath := filepath.Join(baseDir, gravitonDBName)
	sqliteDBPath := filepath.Join(baseDir, sqliteDBName)
	
	start := time.Now()
	err := MigrateFromGravitonToSQLite(gravitonDBPath, sqliteDBPath)
	if err != nil {
		return fmt.Errorf("migration failed: %v", err)
	}
	
	log.Printf("Migration completed in %v", time.Since(start))
	return nil
}