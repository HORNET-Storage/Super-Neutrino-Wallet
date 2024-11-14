package walletstatedb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/atotto/clipboard"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/deroproject/graviton"
)

var Store *graviton.Store

const ChallengeTreeName = "challenges"

const (
	LastScannedBlockKey    = "last_scanned_block_height"
	AddressStatusAvailable = "available"
	AddressStatusAllocated = "allocated"
	AddressStatusUsed      = "used"
	MinAvailableAddresses  = 10
	UnsentTransactionsTree = "unsent_transactions"
)

func InitDB(dbPath string) error {
	var err error
	Store, err = graviton.NewDiskStore(dbPath)
	if err != nil {
		return err
	}
	log.Println("Database initialized successfully")
	return nil
}

func SaveAddress(tree *graviton.Tree, address Address) error {
	key := fmt.Sprintf("%d", address.Index)
	value, err := json.Marshal(address)
	if err != nil {
		return err
	}
	return tree.Put([]byte(key), value)
}

func GetAddresses(tree *graviton.Tree) ([]Address, error) {
	cursor := tree.Cursor()
	var addresses []Address
	for _, v, err := cursor.First(); err == nil; _, v, err = cursor.Next() {
		var addr Address
		if err := json.Unmarshal(v, &addr); err != nil {
			return nil, err
		}
		addresses = append(addresses, addr)
	}
	return addresses, nil
}

func GetUnusedAddress(tree *graviton.Tree) (*Address, error) {
	cursor := tree.Cursor()
	for _, v, err := cursor.First(); err == nil; _, v, err = cursor.Next() {
		var addr Address
		if err := json.Unmarshal(v, &addr); err != nil {
			return nil, err
		}
		if addr.Status == AddressStatusAvailable {
			return &addr, nil
		}
	}
	return nil, fmt.Errorf("no unused address found")
}

func MarkAddressAsUsed(tree *graviton.Tree, address string, blockHeight uint32) error {
	cursor := tree.Cursor()
	for k, v, err := cursor.First(); err == nil; k, v, err = cursor.Next() {
		var addr Address
		if err := json.Unmarshal(v, &addr); err != nil {
			return err
		}
		if addr.Address == address {
			addr.Status = AddressStatusUsed
			now := time.Now()
			addr.UsedAt = &now
			addr.BlockHeight = &blockHeight
			value, err := json.Marshal(addr)
			if err != nil {
				return err
			}
			return tree.Put(k, value)
		}
	}
	return fmt.Errorf("address not found")
}

func AllocateAddress(tree *graviton.Tree) (*Address, error) {
	cursor := tree.Cursor()
	for k, v, err := cursor.First(); err == nil; k, v, err = cursor.Next() {
		var addr Address
		if err := json.Unmarshal(v, &addr); err != nil {
			return nil, err
		}
		if addr.Status == AddressStatusAvailable {
			addr.Status = AddressStatusAllocated
			now := time.Now()
			addr.AllocatedAt = &now
			value, err := json.Marshal(addr)
			if err != nil {
				return nil, err
			}
			if err := tree.Put(k, value); err != nil {
				return nil, err
			}
			return &addr, nil
		}
	}
	return nil, fmt.Errorf("no available addresses")
}

func SaveBlockHeight(tree *graviton.Tree, height int32) error {
	key := []byte("end_height")
	value := []byte(fmt.Sprintf("%d", height))
	return tree.Put(key, value)
}

func GetBlockHeight(tree *graviton.Tree) (int32, error) {
	value, err := tree.Get([]byte("end_height"))
	if err != nil {
		return 0, err
	}
	var height int32
	if _, err := fmt.Sscanf(string(value), "%d", &height); err != nil {
		return 0, err
	}
	return height, nil
}

func RetrieveAddresses() ([]btcutil.Address, []btcutil.Address, error) {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load snapshot: %v", err)
	}

	receiveAddrTree, err := ss.GetTree("receive_addresses")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get receive addresses tree: %v", err)
	}

	changeAddrTree, err := ss.GetTree("change_addresses")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get change addresses tree: %v", err)
	}

	receiveAddrs, err := GetAddresses(receiveAddrTree)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get receive addresses: %v", err)
	}

	changeAddrs, err := GetAddresses(changeAddrTree)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get change addresses: %v", err)
	}

	receiveAddresses := make([]btcutil.Address, len(receiveAddrs))
	changeAddresses := make([]btcutil.Address, len(changeAddrs))

	for i, addr := range receiveAddrs {
		btcAddr, err := btcutil.DecodeAddress(addr.Address, nil)
		if err != nil {
			return nil, nil, err
		}
		receiveAddresses[i] = btcAddr
	}

	for i, addr := range changeAddrs {
		btcAddr, err := btcutil.DecodeAddress(addr.Address, nil)
		if err != nil {
			return nil, nil, err
		}
		changeAddresses[i] = btcAddr
	}

	log.Printf("Retrieved %d receive addresses and %d change addresses from the database", len(receiveAddresses), len(changeAddresses))
	return receiveAddresses, changeAddresses, nil
}

func PrintAndCopyReceiveAddresses() (Address, error) {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return Address{}, fmt.Errorf("failed to load snapshot: %v", err)
	}

	receiveAddrTree, err := ss.GetTree("receive_addresses")
	if err != nil {
		return Address{}, fmt.Errorf("failed to get receive addresses tree: %v", err)
	}

	receiveAddrs, err := GetAddresses(receiveAddrTree)
	if err != nil {
		return Address{}, fmt.Errorf("failed to get receive addresses: %v", err)
	}

	if len(receiveAddrs) == 0 {
		return Address{}, fmt.Errorf("no receive addresses found")
	}

	firstAddress := receiveAddrs[0]
	addressString := firstAddress.Address

	err = clipboard.WriteAll(addressString)
	if err != nil {
		return Address{}, fmt.Errorf("failed to copy address to clipboard: %v", err)
	}

	fmt.Printf("Receive Address copied to clipboard: %s\n", addressString)

	return firstAddress, nil
}

func GetLastAddressIndex(tree *graviton.Tree) (int, error) {
	cursor := tree.Cursor()
	var lastIndex int
	for k, _, err := cursor.Last(); err == nil; k, _, err = cursor.Prev() {
		lastIndex = int(k[0])
		break
	}
	return lastIndex, nil
}

func CommitTrees(trees ...*graviton.Tree) error {
	_, err := graviton.Commit(trees...)
	return err
}

func SetLastScannedBlockHeight(height int32) error {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %v", err)
	}

	metadataTree, err := ss.GetTree("metadata")
	if err != nil {
		return fmt.Errorf("failed to get metadata tree: %v", err)
	}

	err = metadataTree.Put([]byte(LastScannedBlockKey), []byte(fmt.Sprintf("%d", height)))
	if err != nil {
		return fmt.Errorf("failed to set last scanned block height: %v", err)
	}

	_, err = graviton.Commit(metadataTree)
	if err != nil {
		return fmt.Errorf("failed to commit last scanned block height: %v", err)
	}

	log.Printf("Last scanned block height set to %d", height)
	return nil
}

func GetLastScannedBlockHeight() (int32, error) {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return 0, fmt.Errorf("failed to load snapshot: %v", err)
	}

	metadataTree, err := ss.GetTree("metadata")
	if err != nil {
		return 0, fmt.Errorf("failed to get metadata tree: %v", err)
	}

	heightBytes, err := metadataTree.Get([]byte(LastScannedBlockKey))
	if err != nil {
		if err == graviton.ErrNotFound {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get last scanned block height: %v", err)
	}

	var height int32
	_, err = fmt.Sscanf(string(heightBytes), "%d", &height)
	if err != nil {
		return 0, fmt.Errorf("failed to parse last scanned block height: %v", err)
	}

	return height, nil
}

func UpdateLastScannedBlockHeight(height int32) error {
	return SetLastScannedBlockHeight(height)
}

func GenerateNewAddresses(w *wallet.Wallet, count int) error {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %v", err)
	}

	receiveAddrTree, err := ss.GetTree("receive_addresses")
	if err != nil {
		return fmt.Errorf("failed to get receive addresses tree: %v", err)
	}

	lastIndex, err := GetLastAddressIndex(receiveAddrTree)
	if err != nil {
		return fmt.Errorf("error getting last address index: %v", err)
	}

	for i := 0; i < count; i++ {
		newAddr, err := w.NewAddress(0, waddrmgr.KeyScopeBIP0084)
		if err != nil {
			return fmt.Errorf("failed to generate new address: %v", err)
		}

		addr := Address{
			Index:   uint(lastIndex + i + 1),
			Address: newAddr.EncodeAddress(),
			Status:  AddressStatusAvailable,
		}

		err = SaveAddress(receiveAddrTree, addr)
		if err != nil {
			return fmt.Errorf("failed to save new address: %v", err)
		}
	}

	_, err = graviton.Commit(receiveAddrTree)
	if err != nil {
		return fmt.Errorf("failed to commit new addresses: %v", err)
	}

	return nil
}

func EnsureMinimumAvailableAddresses(w *wallet.Wallet) error {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %v", err)
	}

	receiveAddrTree, err := ss.GetTree("receive_addresses")
	if err != nil {
		return fmt.Errorf("failed to get receive addresses tree: %v", err)
	}

	availableCount := 0
	cursor := receiveAddrTree.Cursor()
	for _, v, err := cursor.First(); err == nil; _, v, err = cursor.Next() {
		var addr Address
		if err := json.Unmarshal(v, &addr); err != nil {
			return err
		}
		if addr.Status == AddressStatusAvailable {
			availableCount++
		}
	}

	if availableCount < MinAvailableAddresses {
		count := MinAvailableAddresses - availableCount
		return GenerateNewAddresses(w, count)
	}

	return nil
}

func UpdateAddressUsage(transactions []map[string]interface{}) error {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %v", err)
	}

	receiveAddrTree, err := ss.GetTree("receive_addresses")
	if err != nil {
		return fmt.Errorf("failed to get receive addresses tree: %v", err)
	}

	for _, tx := range transactions {
		address := tx["output"].(string)
		_ = tx["date"].(time.Time)
		blockHeight := uint32(tx["blockHeight"].(int))

		err := MarkAddressAsUsed(receiveAddrTree, address, blockHeight)
		if err != nil {
			return fmt.Errorf("failed to mark address as used: %v", err)
		}
	}

	_, err = graviton.Commit(receiveAddrTree)
	if err != nil {
		return fmt.Errorf("failed to commit address usage updates: %v", err)
	}

	return nil
}

// SaveRawTransaction stores a raw transaction by its hash in the database.
func SaveRawTransaction(tree *graviton.Tree, txHash string, rawTx []byte) error {
	// Store the transaction hash and the raw transaction bytes
	err := tree.Put([]byte(txHash), rawTx)
	if err != nil {
		return fmt.Errorf("failed to save raw transaction: %v", err)
	}
	log.Printf("Transaction %s saved successfully", txHash)
	return nil
}

// GetRawTransaction retrieves a raw transaction by its hash from the database.
func GetRawTransaction(tree *graviton.Tree, txHash string) ([]byte, error) {
	// Fetch the raw transaction bytes by transaction hash
	rawTx, err := tree.Get([]byte(txHash))
	if err != nil {
		if err == graviton.ErrNotFound {
			return nil, fmt.Errorf("transaction %s not found", txHash)
		}
		return nil, fmt.Errorf("failed to retrieve raw transaction: %v", err)
	}

	log.Printf("Transaction %s retrieved successfully", txHash)
	return rawTx, nil
}

// SaveTransactionToDB serializes the transaction and stores it in the database.
func SaveTransactionToDB(tx *wire.MsgTx) (chainhash.Hash, error) {
	// Serialize the transaction into raw bytes
	var buf bytes.Buffer
	err := tx.Serialize(&buf)
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to serialize transaction: %v", err)
	}
	rawTx := buf.Bytes()

	// Load the snapshot from the database
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to load snapshot: %v", err)
	}

	// Get the transactions tree
	txTree, err := ss.GetTree("transactions")
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to get transactions tree: %v", err)
	}

	// Save the serialized transaction in the database
	txHash := tx.TxHash().String()
	err = SaveRawTransaction(txTree, txHash, rawTx)
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to save raw transaction: %v", err)
	}

	// Commit the transaction tree changes
	_, err = graviton.Commit(txTree)
	if err != nil {
		return chainhash.Hash{}, fmt.Errorf("failed to commit transaction to database: %v", err)
	}

	return tx.TxHash(), nil
}

// SaveChallenge saves the generated challenge to Graviton
func SaveChallenge(tree *graviton.Tree, challenge Challenge) error {
	key := challenge.Hash
	value, err := json.Marshal(challenge)
	if err != nil {
		return err
	}
	return tree.Put([]byte(key), value)
}

// GetChallenge retrieves a challenge by its hash
func GetChallenge(tree *graviton.Tree, hash string) (*Challenge, error) {
	value, err := tree.Get([]byte(hash))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve challenge: %v", err)
	}
	var challenge Challenge
	if err := json.Unmarshal(value, &challenge); err != nil {
		return nil, fmt.Errorf("failed to unmarshal challenge: %v", err)
	}
	return &challenge, nil
}

// MarkChallengeAsUsed marks a challenge as used
func MarkChallengeAsUsed(tree *graviton.Tree, hash string) error {
	challenge, err := GetChallenge(tree, hash)
	if err != nil {
		return err
	}
	challenge.Status = "used"
	challenge.UsedAt = time.Now()

	value, err := json.Marshal(challenge)
	if err != nil {
		return err
	}
	return tree.Put([]byte(hash), value)
}

// ExpireOldChallenges marks expired challenges that are older than 2 minutes as expired
func ExpireOldChallenges(tree *graviton.Tree) error {
	cursor := tree.Cursor()
	now := time.Now()
	for _, value, err := cursor.First(); err == nil; _, value, err = cursor.Next() {
		var challenge Challenge
		if err := json.Unmarshal(value, &challenge); err != nil {
			return err
		}
		if challenge.Status == "unused" && now.Sub(challenge.CreatedAt) > 2*time.Minute {
			challenge.Status = "expired"
			challenge.ExpiredAt = now
			newValue, err := json.Marshal(challenge)
			if err != nil {
				return err
			}
			if err := tree.Put([]byte(challenge.Hash), newValue); err != nil {
				return err
			}
		}
	}
	return nil
}

// SaveNewTransaction saves a transaction only if it doesn't already exist
func SaveNewTransaction(tx *Transaction) error {
	exists, err := TransactionExists(tx.TxID, tx.Vout)
	if err != nil {
		return fmt.Errorf("error checking transaction existence: %v", err)
	}

	if exists {
		// Transaction already exists, skip saving
		log.Printf("Transaction %s:%d already exists, skipping", tx.TxID, tx.Vout)
		return nil
	}

	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %v", err)
	}

	// Get both transaction trees
	txTree, err := ss.GetTree("transactions")
	if err != nil {
		return fmt.Errorf("failed to get transactions tree: %v", err)
	}

	unsentTxTree, err := ss.GetTree(UnsentTransactionsTree)
	if err != nil {
		return fmt.Errorf("failed to get unsent transactions tree: %v", err)
	}

	// Create unique key using TxID and Vout
	key := fmt.Sprintf("%s:%d", tx.TxID, tx.Vout)
	value, err := json.Marshal(tx)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction: %v", err)
	}

	// Save to both trees
	if err := txTree.Put([]byte(key), value); err != nil {
		return fmt.Errorf("failed to save to main transaction tree: %v", err)
	}

	if err := unsentTxTree.Put([]byte(key), value); err != nil {
		return fmt.Errorf("failed to save to unsent transaction tree: %v", err)
	}

	_, err = graviton.Commit(txTree, unsentTxTree)
	if err != nil {
		return fmt.Errorf("failed to commit transaction trees: %v", err)
	}

	log.Printf("Saved new transaction %s:%d", tx.TxID, tx.Vout)
	return nil
}

// GetUnsentTransactions retrieves all unsent transactions
func GetUnsentTransactions() ([]Transaction, error) {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return nil, fmt.Errorf("failed to load snapshot: %v", err)
	}

	unsentTxTree, err := ss.GetTree(UnsentTransactionsTree)
	if err != nil {
		return nil, fmt.Errorf("failed to get unsent transactions tree: %v", err)
	}

	var transactions []Transaction
	cursor := unsentTxTree.Cursor()

	for _, v, err := cursor.First(); err == nil; _, v, err = cursor.Next() {
		var tx Transaction
		if err := json.Unmarshal(v, &tx); err != nil {
			return nil, fmt.Errorf("failed to unmarshal transaction: %v", err)
		}
		transactions = append(transactions, tx)
	}

	return transactions, nil
}

// ClearUnsentTransactions removes transactions from the unsent tree
func ClearUnsentTransactions() error {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return fmt.Errorf("failed to load snapshot: %v", err)
	}

	unsentTxTree, err := ss.GetTree(UnsentTransactionsTree)
	if err != nil {
		return fmt.Errorf("failed to get unsent transactions tree: %v", err)
	}

	cursor := unsentTxTree.Cursor()
	for k, _, err := cursor.First(); err == nil; k, _, err = cursor.Next() {
		if err := unsentTxTree.Delete(k); err != nil {
			return fmt.Errorf("failed to delete transaction: %v", err)
		}
	}

	_, err = graviton.Commit(unsentTxTree)
	if err != nil {
		return fmt.Errorf("failed to commit cleared transactions: %v", err)
	}

	return nil
}

func TransactionExists(txID string, vout uint32) (bool, error) {
	ss, err := Store.LoadSnapshot(0)
	if err != nil {
		return false, fmt.Errorf("failed to load snapshot: %v", err)
	}

	txTree, err := ss.GetTree("transactions")
	if err != nil {
		return false, fmt.Errorf("failed to get transactions tree: %v", err)
	}

	key := fmt.Sprintf("%s:%d", txID, vout)

	// Use cursor to iterate instead of direct Get to avoid hash collision issues
	cursor := txTree.Cursor()
	for k, _, err := cursor.First(); err == nil; k, _, err = cursor.Next() {
		if string(k) == key {
			return true, nil
		}
	}

	unsentTxTree, err := ss.GetTree(UnsentTransactionsTree)
	if err != nil {
		return false, fmt.Errorf("failed to get unsent transactions tree: %v", err)
	}

	cursor = unsentTxTree.Cursor()
	for k, _, err := cursor.First(); err == nil; k, _, err = cursor.Next() {
		if string(k) == key {
			return true, nil
		}
	}

	return false, nil
}
