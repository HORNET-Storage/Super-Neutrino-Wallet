package transaction

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/wtxmgr"
	"github.com/checksum0/go-electrum/electrum"
	"github.com/lightninglabs/neutrino"
)

const (
	RBFSequenceNumber      = 0xffffffff - 2
	DustThreshold          = btcutil.Amount(546) // satoshis
	ConsolidationThreshold = 10000               // satoshis, adjust as needed
	MaxStandardTxWeight    = 400000
)

func CheckBalanceAndCreateTransaction(w *wallet.Wallet, service *neutrino.ChainService, enableRBF bool, spendAmount int64, recipientAddress string, privPass []byte) (chainhash.Hash, bool, error) {
	log.Printf("Starting transaction creation process.")
	// Reset locked outpoints
	log.Printf("Resetting locked outpoints.")
	w.ResetLockedOutpoints()

	// Unlock wallet
	log.Printf("Unlocking wallet.")
	err := w.Unlock(privPass, nil)
	if err != nil {
		log.Printf("Failed to unlock wallet: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to unlock wallet: %v", err)
	}

	// Calculate wallet balance
	balance, err := w.CalculateBalance(1)
	if err != nil {
		log.Printf("Failed to calculate balance: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to calculate balance: %v", err)
	}
	log.Printf("Available balance: %s\n", balance.String())

	// Set recipient address and amount
	amountToSend := btcutil.Amount(spendAmount) // 0.00005 BTC
	log.Printf("Recipient address: %s, Amount to send: %d satoshis", recipientAddress, amountToSend)

	// Check sufficient balance
	if balance < amountToSend {
		log.Printf("Insufficient balance: have %d satoshis, want to send %d satoshis", int64(balance), amountToSend)
		return chainhash.Hash{}, false, fmt.Errorf("insufficient balance: have %d satoshis, want to send %d satoshis", int64(balance), amountToSend)
	}

	// Get fee recommendation
	feeRec, err := getFeeRecommendation()
	if err != nil {
		log.Printf("Failed to get fee recommendation, using default values: %v", err)
		feeRec = FeeRecommendation{FastestFee: 5, HalfHourFee: 4, HourFee: 3, EconomyFee: 2, MinimumFee: 1}
	}
	log.Printf("Fee recommendation: %v", feeRec)

	// Get user fee priority
	feeRate, err := getUserFeePriority(feeRec)
	if err != nil {
		log.Printf("Failed to get user fee priority: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to get user fee priority: %v", err)
	}
	log.Printf("Selected fee rate: %d sat/vB", feeRate)

	// List unspent outputs
	utxos, err := w.ListUnspent(1, 9999999, "")
	if err != nil {
		log.Printf("Failed to list unspent outputs: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to list unspent outputs: %v", err)
	}
	log.Printf("Found %d unspent outputs.", len(utxos))

	// Select suitable UTXO
	var selectedUTXO *btcjson.ListUnspentResult
	for _, utxo := range utxos {
		log.Printf("Checking UTXO: %s:%d", utxo.TxID, utxo.Vout)

		err := VerifyUTXO(utxo.TxID, utxo.Vout)
		if err != nil {
			log.Printf("UTXO %s:%d is invalid: %v", utxo.TxID, utxo.Vout, err)
			continue
		}

		if btcutil.Amount(utxo.Amount*btcutil.SatoshiPerBitcoin) >= amountToSend+btcutil.Amount(feeRate) {
			selectedUTXO = utxo
			break
		}
	}

	if selectedUTXO == nil {
		log.Printf("No suitable UTXO found.")
		return chainhash.Hash{}, false, fmt.Errorf("no suitable UTXO found")
	}
	log.Printf("Selected UTXO: %s:%d", selectedUTXO.TxID, selectedUTXO.Vout)

	// Create new transaction
	tx := wire.NewMsgTx(wire.TxVersion)
	prevOutHash, err := chainhash.NewHashFromStr(selectedUTXO.TxID)
	if err != nil {
		log.Printf("Failed to parse txid: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to parse txid: %v", err)
	}
	prevOut := wire.NewOutPoint(prevOutHash, selectedUTXO.Vout)
	txIn := wire.NewTxIn(prevOut, nil, nil)

	if enableRBF {
		txIn.Sequence = RBFSequenceNumber
		log.Printf("RBF enabled for this transaction (sequence number: %d)", RBFSequenceNumber)
	}

	tx.AddTxIn(txIn)

	// Add recipient output
	recipientAddr, err := btcutil.DecodeAddress(recipientAddress, w.ChainParams())
	if err != nil {
		log.Printf("Failed to decode recipient address: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode recipient address: %v", err)
	}
	pkScript, err := txscript.PayToAddrScript(recipientAddr)
	if err != nil {
		log.Printf("Failed to create output script: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to create output script: %v", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(amountToSend), pkScript))

	// Calculate transaction size
	txSize := tx.SerializeSize()

	// Calculate the required fee
	requiredFee := btcutil.Amount(txSize * int(feeRate))

	// Calculate input and change amounts
	inputAmount := btcutil.Amount(selectedUTXO.Amount * btcutil.SatoshiPerBitcoin)
	changeAmount := inputAmount - amountToSend - requiredFee
	if changeAmount > DustThreshold {
		changeAddr, err := getChangeAddress(w)
		if err != nil {
			log.Printf("Failed to get change address: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to get change address: %v", err)
		}
		changePkScript, err := txscript.PayToAddrScript(changeAddr)
		if err != nil {
			log.Printf("Failed to create change script: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to create change script: %v", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(changeAmount), changePkScript))
	}

	// Sign the transaction
	utxoAddr, err := btcutil.DecodeAddress(selectedUTXO.Address, w.ChainParams())
	if err != nil {
		log.Printf("Failed to decode UTXO address: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode UTXO address: %v", err)
	}
	privKey, err := w.PrivKeyForAddress(utxoAddr)
	if err != nil {
		log.Printf("Failed to get private key: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to get private key: %v", err)
	}
	scriptPubKey, err := hex.DecodeString(selectedUTXO.ScriptPubKey)
	if err != nil {
		log.Printf("Failed to decode scriptPubKey: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode scriptPubKey: %v", err)
	}

	prevOutputs := txscript.NewCannedPrevOutputFetcher(scriptPubKey, int64(inputAmount))

	if isSegWitAddress(utxoAddr) {
		// Create the witness script for SegWit inputs
		witnessScript, err := txscript.WitnessSignature(tx, txscript.NewTxSigHashes(tx, prevOutputs), 0, int64(inputAmount), scriptPubKey, txscript.SigHashAll, privKey, true)
		if err != nil {
			log.Printf("Failed to create witness script: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to create witness script: %v", err)
		}
		tx.TxIn[0].Witness = witnessScript
	} else {
		// Create the signature script for non-SegWit inputs
		sigScript, err := txscript.SignatureScript(tx, 0, scriptPubKey, txscript.SigHashAll, privKey, true)
		if err != nil {
			log.Printf("Failed to create signature script: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to create signature script: %v", err)
		}
		tx.TxIn[0].SignatureScript = sigScript
	}

	// Verify the signature
	valid, err := verifySignature(tx, 0, scriptPubKey, int64(inputAmount))
	if err != nil {
		log.Printf("Failed to verify signature: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to verify signature: %v", err)
	}
	if !valid {
		log.Printf("Signature verification failed")
		return chainhash.Hash{}, false, fmt.Errorf("signature verification failed")
	}
	log.Printf("Signature verification succeeded")

	log.Printf("Transaction created successfully. Details:")
	log.Printf("  TxID: %s", tx.TxHash().String())
	log.Printf("  Amount to send: %d satoshis", amountToSend)
	log.Printf("  Fee: %d satoshis", requiredFee)
	log.Printf("  Total input: %d satoshis", inputAmount)
	log.Printf("  Change amount: %d satoshis", changeAmount)
	log.Printf("  Transaction size: %d vBytes", tx.SerializeSize())
	log.Printf("  Number of inputs: %d", len(tx.TxIn))
	log.Printf("  Number of outputs: %d", len(tx.TxOut))

	log.Println("Detailed Transaction Information:")
	log.Printf("TxID: %s", tx.TxHash())
	log.Printf("Version: %d", tx.Version)
	log.Printf("Locktime: %d", tx.LockTime)

	log.Println("Inputs:")
	for i, input := range tx.TxIn {
		log.Printf("  Input %d:", i)
		log.Printf("    Previous OutPoint: %s", input.PreviousOutPoint)
		log.Printf("    Sequence: %d", input.Sequence)
		log.Printf("    SignatureScript: %x", input.SignatureScript)
		if len(input.Witness) > 0 {
			log.Printf("    Witness: %x", input.Witness)
		}
	}

	log.Println("Outputs:")
	for i, output := range tx.TxOut {
		log.Printf("  Output %d:", i)
		log.Printf("    Value: %d satoshis", output.Value)
		log.Printf("    PkScript: %x", output.PkScript)
	}

	// Save the transaction in the database
	_, err = walletstatedb.SaveTransactionToDB(tx)
	if err != nil {
		return chainhash.Hash{}, false, err
	}

	// Create Electrum client once
	electrumClient, err := CreateElectrumClient(ElectrumConfig{
		ServerAddr: "electrum.blockstream.info:50002",
		UseSSL:     true,
	})
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to create Electrum client: %v", err)
	}
	defer electrumClient.Shutdown()

	txHash, verified, err := broadcastAndVerifyTransaction(tx, service)
	if err != nil {
		// Release the output we tried to spend
		releaseErr := w.ReleaseOutput(wtxmgr.LockID(tx.TxHash()), tx.TxIn[0].PreviousOutPoint)
		if releaseErr != nil {
			log.Printf("Failed to release output: %v", releaseErr)
		}
		return chainhash.Hash{}, false, fmt.Errorf("failed to broadcast and verify transaction: %v", err)
	}

	log.Println("Transaction broadcast and verified successfully.")
	return txHash, verified, nil
}

func HttpCheckBalanceAndCreateTransaction(w *wallet.Wallet, service *neutrino.ChainService, enableRBF bool, spendAmount int64, recipientAddress string, privPass []byte, feeRate int) (chainhash.Hash, bool, error) {
	log.Printf("Starting transaction creation process.")
	// Reset locked outpoints
	log.Printf("Resetting locked outpoints.")
	w.ResetLockedOutpoints()

	// Unlock wallet
	log.Printf("Unlocking wallet.")
	err := w.Unlock(privPass, nil)
	if err != nil {
		log.Printf("Failed to unlock wallet: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to unlock wallet: %v", err)
	}

	// Calculate wallet balance
	balance, err := w.CalculateBalance(1)
	if err != nil {
		log.Printf("Failed to calculate balance: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to calculate balance: %v", err)
	}
	log.Printf("Available balance: %s\n", balance.String())

	// Set recipient address and amount
	amountToSend := btcutil.Amount(spendAmount) // 0.00005 BTC
	log.Printf("Recipient address: %s, Amount to send: %d satoshis", recipientAddress, amountToSend)

	// Check sufficient balance
	if balance < amountToSend {
		log.Printf("Insufficient balance: have %d satoshis, want to send %d satoshis", int64(balance), amountToSend)
		return chainhash.Hash{}, false, fmt.Errorf("insufficient balance: have %d satoshis, want to send %d satoshis", int64(balance), amountToSend)
	}

	// List unspent outputs
	utxos, err := w.ListUnspent(1, 9999999, "")
	if err != nil {
		log.Printf("Failed to list unspent outputs: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to list unspent outputs: %v", err)
	}
	log.Printf("Found %d unspent outputs.", len(utxos))

	// Select suitable UTXO
	var selectedUTXO *btcjson.ListUnspentResult
	for _, utxo := range utxos {
		log.Printf("Checking UTXO: %s:%d", utxo.TxID, utxo.Vout)

		err := VerifyUTXO(utxo.TxID, utxo.Vout)
		if err != nil {
			log.Printf("UTXO %s:%d is invalid: %v", utxo.TxID, utxo.Vout, err)
			continue
		}

		if btcutil.Amount(utxo.Amount*btcutil.SatoshiPerBitcoin) >= amountToSend+btcutil.Amount(feeRate) {
			selectedUTXO = utxo
			break
		}
	}

	if selectedUTXO == nil {
		log.Printf("No suitable UTXO found.")
		return chainhash.Hash{}, false, fmt.Errorf("no suitable UTXO found")
	}
	log.Printf("Selected UTXO: %s:%d", selectedUTXO.TxID, selectedUTXO.Vout)

	// Create new transaction
	tx := wire.NewMsgTx(wire.TxVersion)
	prevOutHash, err := chainhash.NewHashFromStr(selectedUTXO.TxID)
	if err != nil {
		log.Printf("Failed to parse txid: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to parse txid: %v", err)
	}
	prevOut := wire.NewOutPoint(prevOutHash, selectedUTXO.Vout)
	txIn := wire.NewTxIn(prevOut, nil, nil)

	if enableRBF {
		txIn.Sequence = RBFSequenceNumber
		log.Printf("RBF enabled for this transaction (sequence number: %d)", RBFSequenceNumber)
	}

	tx.AddTxIn(txIn)

	// Add recipient output
	recipientAddr, err := btcutil.DecodeAddress(recipientAddress, w.ChainParams())
	if err != nil {
		log.Printf("Failed to decode recipient address: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode recipient address: %v", err)
	}
	pkScript, err := txscript.PayToAddrScript(recipientAddr)
	if err != nil {
		log.Printf("Failed to create output script: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to create output script: %v", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(amountToSend), pkScript))

	// Calculate transaction size
	txSize := tx.SerializeSize()

	// Calculate the required fee
	requiredFee := btcutil.Amount(txSize * int(feeRate))

	// Calculate input and change amounts
	inputAmount := btcutil.Amount(selectedUTXO.Amount * btcutil.SatoshiPerBitcoin)
	changeAmount := inputAmount - amountToSend - requiredFee
	if changeAmount > DustThreshold {
		changeAddr, err := getChangeAddress(w)
		if err != nil {
			log.Printf("Failed to get change address: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to get change address: %v", err)
		}
		changePkScript, err := txscript.PayToAddrScript(changeAddr)
		if err != nil {
			log.Printf("Failed to create change script: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to create change script: %v", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(changeAmount), changePkScript))
	}

	// Sign the transaction
	utxoAddr, err := btcutil.DecodeAddress(selectedUTXO.Address, w.ChainParams())
	if err != nil {
		log.Printf("Failed to decode UTXO address: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode UTXO address: %v", err)
	}
	privKey, err := w.PrivKeyForAddress(utxoAddr)
	if err != nil {
		log.Printf("Failed to get private key: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to get private key: %v", err)
	}
	scriptPubKey, err := hex.DecodeString(selectedUTXO.ScriptPubKey)
	if err != nil {
		log.Printf("Failed to decode scriptPubKey: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode scriptPubKey: %v", err)
	}

	prevOutputs := txscript.NewCannedPrevOutputFetcher(scriptPubKey, int64(inputAmount))

	if isSegWitAddress(utxoAddr) {
		// Create the witness script for SegWit inputs
		witnessScript, err := txscript.WitnessSignature(tx, txscript.NewTxSigHashes(tx, prevOutputs), 0, int64(inputAmount), scriptPubKey, txscript.SigHashAll, privKey, true)
		if err != nil {
			log.Printf("Failed to create witness script: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to create witness script: %v", err)
		}
		tx.TxIn[0].Witness = witnessScript
	} else {
		// Create the signature script for non-SegWit inputs
		sigScript, err := txscript.SignatureScript(tx, 0, scriptPubKey, txscript.SigHashAll, privKey, true)
		if err != nil {
			log.Printf("Failed to create signature script: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to create signature script: %v", err)
		}
		tx.TxIn[0].SignatureScript = sigScript
	}

	// Verify the signature
	valid, err := verifySignature(tx, 0, scriptPubKey, int64(inputAmount))
	if err != nil {
		log.Printf("Failed to verify signature: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to verify signature: %v", err)
	}
	if !valid {
		log.Printf("Signature verification failed")
		return chainhash.Hash{}, false, fmt.Errorf("signature verification failed")
	}
	log.Printf("Signature verification succeeded")

	log.Printf("Transaction created successfully. Details:")
	log.Printf("  TxID: %s", tx.TxHash().String())
	log.Printf("  Amount to send: %d satoshis", amountToSend)
	log.Printf("  Fee: %d satoshis", requiredFee)
	log.Printf("  Total input: %d satoshis", inputAmount)
	log.Printf("  Change amount: %d satoshis", changeAmount)
	log.Printf("  Transaction size: %d vBytes", tx.SerializeSize())
	log.Printf("  Number of inputs: %d", len(tx.TxIn))
	log.Printf("  Number of outputs: %d", len(tx.TxOut))

	log.Println("Detailed Transaction Information:")
	log.Printf("TxID: %s", tx.TxHash())
	log.Printf("Version: %d", tx.Version)
	log.Printf("Locktime: %d", tx.LockTime)

	log.Println("Inputs:")
	for i, input := range tx.TxIn {
		log.Printf("  Input %d:", i)
		log.Printf("    Previous OutPoint: %s", input.PreviousOutPoint)
		log.Printf("    Sequence: %d", input.Sequence)
		log.Printf("    SignatureScript: %x", input.SignatureScript)
		if len(input.Witness) > 0 {
			log.Printf("    Witness: %x", input.Witness)
		}
	}

	log.Println("Outputs:")
	for i, output := range tx.TxOut {
		log.Printf("  Output %d:", i)
		log.Printf("    Value: %d satoshis", output.Value)
		log.Printf("    PkScript: %x", output.PkScript)
	}

	// Save the transaction in the database
	_, err = walletstatedb.SaveTransactionToDB(tx)
	if err != nil {
		return chainhash.Hash{}, false, err
	}

	// Create Electrum client once
	electrumClient, err := CreateElectrumClient(ElectrumConfig{
		ServerAddr: "electrum.blockstream.info:50002",
		UseSSL:     false,
	})
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to create Electrum client: %v", err)
	}
	defer electrumClient.Shutdown()

	txHash, verified, err := broadcastAndVerifyTransaction(tx, service)
	if err != nil {
		// Release the output we tried to spend
		releaseErr := w.ReleaseOutput(wtxmgr.LockID(tx.TxHash()), tx.TxIn[0].PreviousOutPoint)
		if releaseErr != nil {
			log.Printf("Failed to release output: %v", releaseErr)
		}
		return chainhash.Hash{}, false, fmt.Errorf("failed to broadcast and verify transaction: %v", err)
	}

	log.Println("Transaction broadcast and verified successfully.")
	return txHash, verified, nil
}

func HttpCalculateTransactionSize(w *wallet.Wallet, spendAmount int64, recipientAddress string, feeRate int) (int, error) {
	log.Printf("Starting transaction size calculation process.")

	// Set recipient address and amount
	amountToSend := btcutil.Amount(spendAmount)

	// List unspent outputs
	utxos, err := w.ListUnspent(1, 9999999, "")
	if err != nil {
		log.Printf("Failed to list unspent outputs: %v", err)
		return 0, fmt.Errorf("failed to list unspent outputs: %v", err)
	}

	// Select all UTXOs, including dust
	var selectedUTXOs []*btcjson.ListUnspentResult
	var totalSelected btcutil.Amount

	for _, utxo := range utxos {
		utxoAmount := btcutil.Amount(utxo.Amount * btcutil.SatoshiPerBitcoin)
		selectedUTXOs = append(selectedUTXOs, utxo)
		totalSelected += utxoAmount
	}

	// Create new transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs
	for _, utxo := range selectedUTXOs {
		prevOutHash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return 0, fmt.Errorf("failed to parse txid: %v", err)
		}
		prevOut := wire.NewOutPoint(prevOutHash, utxo.Vout)
		txIn := wire.NewTxIn(prevOut, nil, nil)
		tx.AddTxIn(txIn)
	}

	// Add recipient output
	recipientAddr, err := btcutil.DecodeAddress(recipientAddress, w.ChainParams())
	if err != nil {
		return 0, fmt.Errorf("failed to decode recipient address: %v", err)
	}
	pkScript, err := txscript.PayToAddrScript(recipientAddr)
	if err != nil {
		return 0, fmt.Errorf("failed to create output script: %v", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(amountToSend), pkScript))

	// Calculate initial transaction size and fee
	initialSize := tx.SerializeSize()
	initialFee := btcutil.Amount(initialSize * feeRate)

	// Calculate change
	changeAmount := totalSelected - amountToSend - initialFee

	// Add change output if it's not dust
	if changeAmount > DustThreshold {
		changeAddr, err := getChangeAddress(w)
		if err != nil {
			return 0, fmt.Errorf("failed to get change address: %v", err)
		}
		changePkScript, err := txscript.PayToAddrScript(changeAddr)
		if err != nil {
			return 0, fmt.Errorf("failed to create change script: %v", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(changeAmount), changePkScript))
	}

	// Calculate the final size of the transaction
	finalSize := tx.SerializeSize()
	log.Printf("Calculated transaction size: %d bytes", finalSize)
	return finalSize, nil
}

func ReplaceTransactionWithHigherFee(w *wallet.Wallet, service *neutrino.ChainService, originalTxID string, newFeeRate int64, electrumClient *electrum.Client, privPass []byte) (chainhash.Hash, bool, error) {
	log.Printf("Starting RBF process for transaction %s with new fee rate %d sat/vB", originalTxID, newFeeRate)

	// Unlock wallet
	log.Printf("Unlocking wallet.")
	err := w.Unlock(privPass, nil)
	if err != nil {
		log.Printf("Failed to unlock wallet: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to unlock wallet: %v", err)
	}
	defer w.Lock()

	// First attempt to fetch the transaction from the local database
	txDetails, err := RetrieveTransaction(originalTxID)
	if err != nil {
		// If retrieval from the database fails, try Electrum
		log.Printf("Error fetching transaction from local database: %v", err)
		log.Printf("Attempting to fetch transaction from Electrum")

		txDetails, err = GetAndPrintTransaction(electrumClient, originalTxID)
		if err != nil {
			log.Printf("Error fetching transaction from Electrum: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("error fetching transaction from both local database and Electrum: %v", err)
		}
		log.Printf("Successfully retrieved transaction from Electrum")
	} else {
		log.Printf("Successfully retrieved transaction from local database")
	}

	// Decode the original transaction
	txBytes, err := hex.DecodeString(txDetails)
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode transaction hex: %v", err)
	}
	originalTx := wire.NewMsgTx(wire.TxVersion)
	if err := originalTx.Deserialize(bytes.NewReader(txBytes)); err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to deserialize original transaction: %v", err)
	}

	log.Printf("Original transaction decoded. TxID: %s", originalTx.TxHash().String())

	// Create new transaction
	newTx := wire.NewMsgTx(wire.TxVersion)

	// Copy inputs from the original transaction
	var totalIn int64
	for _, txIn := range originalTx.TxIn {
		newTxIn := wire.NewTxIn(&txIn.PreviousOutPoint, nil, nil)
		newTxIn.Sequence = RBFSequenceNumber // Enable RBF
		newTx.AddTxIn(newTxIn)

		// Fetch UTXO details
		utxo, err := fetchUTXO(w, &txIn.PreviousOutPoint)
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to get UTXO for input: %v", err)
		}
		totalIn += int64(utxo.Amount * btcutil.SatoshiPerBitcoin)
	}

	// Copy outputs from the original transaction
	var totalOut int64
	for _, txOut := range originalTx.TxOut {
		newTx.AddTxOut(txOut)
		totalOut += txOut.Value
	}

	// Calculate new fee
	txSize := newTx.SerializeSize()
	newFee := btcutil.Amount(txSize * int(newFeeRate))
	oldFee := btcutil.Amount(totalIn - totalOut)
	extraFee := newFee - oldFee

	if extraFee <= 0 {
		return chainhash.Hash{}, false, fmt.Errorf("new fee is not higher than the original fee")
	}

	// Check if the change output can cover the extra fee
	lastOutputIndex := len(newTx.TxOut) - 1
	changeOutput := newTx.TxOut[lastOutputIndex]
	if btcutil.Amount(changeOutput.Value) < extraFee {
		log.Printf("Change output insufficient to cover new fee. Selecting additional UTXOs.")

		// Calculate how much more we need
		additionalFundsNeeded := extraFee - btcutil.Amount(changeOutput.Value)

		// Select additional UTXOs
		additionalUTXOs, additionalAmount, err := selectAdditionalUTXOs(w, additionalFundsNeeded)
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to select additional UTXOs: %v", err)
		}

		// Add new inputs
		for _, utxo := range additionalUTXOs {
			txHash, err := chainhash.NewHashFromStr(utxo.TxID)
			if err != nil {
				return chainhash.Hash{}, false, fmt.Errorf("failed to parse txid: %v", err)
			}
			outpoint := wire.NewOutPoint(txHash, utxo.Vout)
			txIn := wire.NewTxIn(outpoint, nil, nil)
			txIn.Sequence = RBFSequenceNumber
			newTx.AddTxIn(txIn)
		}

		// Adjust the total input amount
		totalIn += int64(additionalAmount)

		// Recalculate fee based on new transaction size
		txSize = newTx.SerializeSize()
		newFee = btcutil.Amount(txSize * int(newFeeRate))
		extraFee = newFee - oldFee

		// Create a new change output
		changeAddr, err := getChangeAddress(w)
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to generate new change address: %v", err)
		}
		changePkScript, err := txscript.PayToAddrScript(changeAddr)
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to create change script: %v", err)
		}

		newChangeAmount := btcutil.Amount(totalIn) - btcutil.Amount(totalOut) - newFee
		if newChangeAmount > DustThreshold {
			newTx.TxOut[lastOutputIndex] = wire.NewTxOut(int64(newChangeAmount), changePkScript)
		} else {
			// If there's no change, remove the change output
			newTx.TxOut = newTx.TxOut[:lastOutputIndex]
		}
	} else {
		// Adjust the last output (change) to accommodate the new fee
		newTx.TxOut[lastOutputIndex].Value -= int64(extraFee)
	}

	// Check transaction weight
	txWeight := newTx.SerializeSizeStripped()*3 + newTx.SerializeSize()
	if txWeight > MaxStandardTxWeight {
		return chainhash.Hash{}, false, fmt.Errorf("transaction weight (%d) exceeds maximum allowed (%d)", txWeight, MaxStandardTxWeight)
	}

	// Sign the transaction
	for i, txIn := range newTx.TxIn {
		utxo, err := fetchUTXO(w, &txIn.PreviousOutPoint)
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to get UTXO for input %d: %v", i, err)
		}

		scriptPubKey, err := hex.DecodeString(utxo.ScriptPubKey)
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to decode scriptPubKey for input %d: %v", i, err)
		}

		inputAmount := int64(utxo.Amount * btcutil.SatoshiPerBitcoin)

		_, addrs, _, err := txscript.ExtractPkScriptAddrs(scriptPubKey, w.ChainParams())
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to extract address for input %d: %v", i, err)
		}

		if len(addrs) == 0 {
			return chainhash.Hash{}, false, fmt.Errorf("no addresses found for input %d", i)
		}

		log.Printf("Processing input %d: Address: %s, Amount: %d", i, addrs[0].String(), inputAmount)

		privKey, err := w.PrivKeyForAddress(addrs[0])
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to get private key for input %d: %v", i, err)
		}

		prevOutputFetcher := txscript.NewCannedPrevOutputFetcher(scriptPubKey, inputAmount)

		if txscript.IsPayToWitnessPubKeyHash(scriptPubKey) {
			log.Printf("Input %d is SegWit", i)
			witnessScript, err := txscript.WitnessSignature(newTx, txscript.NewTxSigHashes(newTx, prevOutputFetcher), i, inputAmount, scriptPubKey, txscript.SigHashAll, privKey, true)
			if err != nil {
				return chainhash.Hash{}, false, fmt.Errorf("failed to create witness script for input %d: %v", i, err)
			}
			newTx.TxIn[i].Witness = witnessScript
			log.Printf("Witness script created for input %d", i)
		} else {
			log.Printf("Input %d is non-SegWit", i)
			sigScript, err := txscript.SignatureScript(newTx, i, scriptPubKey, txscript.SigHashAll, privKey, true)
			if err != nil {
				return chainhash.Hash{}, false, fmt.Errorf("failed to create signature script for input %d: %v", i, err)
			}
			newTx.TxIn[i].SignatureScript = sigScript
			log.Printf("Signature script created for input %d", i)
		}

		// Verify the signature
		valid, err := verifySignature(newTx, i, scriptPubKey, inputAmount)
		if err != nil {
			log.Printf("Signature verification failed for input %d: %v", i, err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to verify signature for input %d: %v", i, err)
		}
		if !valid {
			log.Printf("Invalid signature for input %d", i)
			return chainhash.Hash{}, false, fmt.Errorf("signature verification failed for input %d", i)
		}
		log.Printf("Signature verification succeeded for input %d", i)
	}

	log.Printf("RBF Transaction created successfully. Details:")
	log.Printf("  TxID: %s", newTx.TxHash().String())
	log.Printf("  New fee: %d satoshis", newFee)
	log.Printf("  Extra fee: %d satoshis", extraFee)
	log.Printf("  Transaction size: %d vBytes", newTx.SerializeSize())
	log.Printf("  Transaction weight: %d weight units", txWeight)
	log.Printf("  Number of inputs: %d", len(newTx.TxIn))
	log.Printf("  Number of outputs: %d", len(newTx.TxOut))

	// Save the transaction in the database
	_, err = walletstatedb.SaveTransactionToDB(newTx)
	if err != nil {
		return chainhash.Hash{}, false, err
	}

	// Broadcast and verify the transaction
	txHash, verified, err := broadcastAndVerifyTransaction(newTx, service)
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to broadcast and verify RBF transaction: %v", err)
	}

	log.Printf("RBF transaction successfully broadcast. New TxID: %s", txHash.String())
	return txHash, verified, nil
}

func RetrieveTransaction(txHash string) (string, error) {
	ss, err := walletstatedb.Store.LoadSnapshot(0)
	if err != nil {
		return "", fmt.Errorf("failed to load snapshot: %v", err)
	}

	txTree, err := ss.GetTree("transactions")
	if err != nil {
		return "", fmt.Errorf("failed to get transactions tree: %v", err)
	}

	// Get the raw transaction from the database
	rawTx, err := walletstatedb.GetRawTransaction(txTree, txHash)
	if err != nil {
		return "", err
	}

	// Convert raw transaction bytes to a hex string for readability
	rawTxHex := hex.EncodeToString(rawTx)
	log.Printf("Retrieved transaction: %s", rawTxHex)
	return rawTxHex, nil
}

func CreateTransactionWithHash(w *wallet.Wallet, service *neutrino.ChainService, enableRBF bool, spendAmount int64, recipientAddress string, fileHash string, privPass []byte) (chainhash.Hash, bool, error) {
	log.Printf("Starting transaction creation process with file hash.")
	// Reset locked outpoints
	log.Printf("Resetting locked outpoints.")
	w.ResetLockedOutpoints()

	// Unlock wallet
	log.Printf("Unlocking wallet.")
	err := w.Unlock(privPass, nil)
	if err != nil {
		log.Printf("Failed to unlock wallet: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to unlock wallet: %v", err)
	}

	// Calculate wallet balance
	balance, err := w.CalculateBalance(1)
	if err != nil {
		log.Printf("Failed to calculate balance: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to calculate balance: %v", err)
	}
	log.Printf("Available balance: %s\n", balance.String())

	// Set recipient address and amount
	amountToSend := btcutil.Amount(spendAmount)
	log.Printf("Recipient address: %s, Amount to send: %d satoshis", recipientAddress, amountToSend)

	// Check sufficient balance
	if balance < amountToSend {
		log.Printf("Insufficient balance: have %d satoshis, want to send %d satoshis", int64(balance), amountToSend)
		return chainhash.Hash{}, false, fmt.Errorf("insufficient balance: have %d satoshis, want to send %d satoshis", int64(balance), amountToSend)
	}

	// Get fee recommendation
	feeRec, err := getFeeRecommendation()
	if err != nil {
		log.Printf("Failed to get fee recommendation, using default values: %v", err)
		feeRec = FeeRecommendation{FastestFee: 5, HalfHourFee: 4, HourFee: 3, EconomyFee: 2, MinimumFee: 1}
	}
	log.Printf("Fee recommendation: %v", feeRec)

	// Get user fee priority
	feeRate, err := getUserFeePriority(feeRec)
	if err != nil {
		log.Printf("Failed to get user fee priority: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to get user fee priority: %v", err)
	}
	log.Printf("Selected fee rate: %d sat/vB", feeRate)

	// List unspent outputs
	utxos, err := w.ListUnspent(1, 9999999, "")
	if err != nil {
		log.Printf("Failed to list unspent outputs: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to list unspent outputs: %v", err)
	}
	log.Printf("Found %d unspent outputs.", len(utxos))

	// Select suitable UTXO
	var selectedUTXO *btcjson.ListUnspentResult
	for _, utxo := range utxos {
		log.Printf("Checking UTXO: %s:%d", utxo.TxID, utxo.Vout)

		err := VerifyUTXO(utxo.TxID, utxo.Vout)
		if err != nil {
			log.Printf("UTXO %s:%d is invalid: %v", utxo.TxID, utxo.Vout, err)
			continue
		}

		if btcutil.Amount(utxo.Amount*btcutil.SatoshiPerBitcoin) >= amountToSend+btcutil.Amount(feeRate) {
			selectedUTXO = utxo
			break
		}
	}

	if selectedUTXO == nil {
		log.Printf("No suitable UTXO found.")
		return chainhash.Hash{}, false, fmt.Errorf("no suitable UTXO found")
	}
	log.Printf("Selected UTXO: %s:%d", selectedUTXO.TxID, selectedUTXO.Vout)

	// Create new transaction
	tx := wire.NewMsgTx(wire.TxVersion)
	prevOutHash, err := chainhash.NewHashFromStr(selectedUTXO.TxID)
	if err != nil {
		log.Printf("Failed to parse txid: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to parse txid: %v", err)
	}
	prevOut := wire.NewOutPoint(prevOutHash, selectedUTXO.Vout)
	txIn := wire.NewTxIn(prevOut, nil, nil)

	if enableRBF {
		txIn.Sequence = RBFSequenceNumber
		log.Printf("RBF enabled for this transaction (sequence number: %d)", RBFSequenceNumber)
	}

	tx.AddTxIn(txIn)

	// Add recipient output
	recipientAddr, err := btcutil.DecodeAddress(recipientAddress, w.ChainParams())
	if err != nil {
		log.Printf("Failed to decode recipient address: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode recipient address: %v", err)
	}
	pkScript, err := txscript.PayToAddrScript(recipientAddr)
	if err != nil {
		log.Printf("Failed to create output script: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to create output script: %v", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(amountToSend), pkScript))

	// Add OP_RETURN output with file hash
	opReturnScript, err := txscript.NullDataScript([]byte(fileHash))
	if err != nil {
		log.Printf("Failed to create OP_RETURN script: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to create OP_RETURN script: %v", err)
	}
	tx.AddTxOut(wire.NewTxOut(0, opReturnScript))

	// Calculate transaction size
	txSize := tx.SerializeSize()

	// Calculate the required fee
	requiredFee := btcutil.Amount(txSize * int(feeRate))

	// Calculate input and change amounts
	inputAmount := btcutil.Amount(selectedUTXO.Amount * btcutil.SatoshiPerBitcoin)
	changeAmount := inputAmount - amountToSend - requiredFee
	if changeAmount > 0 {
		changeAddr, err := getChangeAddress(w)
		if err != nil {
			log.Printf("Failed to get change address: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to get change address: %v", err)
		}
		changePkScript, err := txscript.PayToAddrScript(changeAddr)
		if err != nil {
			log.Printf("Failed to create change script: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to create change script: %v", err)
		}
		tx.AddTxOut(wire.NewTxOut(int64(changeAmount), changePkScript))
	}

	// Sign the transaction
	utxoAddr, err := btcutil.DecodeAddress(selectedUTXO.Address, w.ChainParams())
	if err != nil {
		log.Printf("Failed to decode UTXO address: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode UTXO address: %v", err)
	}
	privKey, err := w.PrivKeyForAddress(utxoAddr)
	if err != nil {
		log.Printf("Failed to get private key: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to get private key: %v", err)
	}
	scriptPubKey, err := hex.DecodeString(selectedUTXO.ScriptPubKey)
	if err != nil {
		log.Printf("Failed to decode scriptPubKey: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode scriptPubKey: %v", err)
	}

	prevOutputs := txscript.NewCannedPrevOutputFetcher(scriptPubKey, int64(inputAmount))

	if isSegWitAddress(utxoAddr) {
		// Create the witness script for SegWit inputs
		witnessScript, err := txscript.WitnessSignature(tx, txscript.NewTxSigHashes(tx, prevOutputs), 0, int64(inputAmount), scriptPubKey, txscript.SigHashAll, privKey, true)
		if err != nil {
			log.Printf("Failed to create witness script: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to create witness script: %v", err)
		}
		tx.TxIn[0].Witness = witnessScript
	} else {
		// Create the signature script for non-SegWit inputs
		sigScript, err := txscript.SignatureScript(tx, 0, scriptPubKey, txscript.SigHashAll, privKey, true)
		if err != nil {
			log.Printf("Failed to create signature script: %v", err)
			return chainhash.Hash{}, false, fmt.Errorf("failed to create signature script: %v", err)
		}
		tx.TxIn[0].SignatureScript = sigScript
	}

	// Verify the signature
	valid, err := verifySignature(tx, 0, scriptPubKey, int64(inputAmount))
	if err != nil {
		log.Printf("Failed to verify signature: %v", err)
		return chainhash.Hash{}, false, fmt.Errorf("failed to verify signature: %v", err)
	}
	if !valid {
		log.Printf("Signature verification failed")
		return chainhash.Hash{}, false, fmt.Errorf("signature verification failed")
	}
	log.Printf("Signature verification succeeded")

	log.Printf("Transaction created successfully. Details:")
	log.Printf("  TxID: %s", tx.TxHash().String())
	log.Printf("  Amount to send: %d satoshis", amountToSend)
	log.Printf("  Fee: %d satoshis", requiredFee)
	log.Printf("  Total input: %d satoshis", inputAmount)
	log.Printf("  Change amount: %d satoshis", changeAmount)
	log.Printf("  Transaction size: %d vBytes", tx.SerializeSize())
	log.Printf("  Number of inputs: %d", len(tx.TxIn))
	log.Printf("  Number of outputs: %d", len(tx.TxOut))
	log.Printf("  File hash included: %s", fileHash)

	log.Println("Detailed Transaction Information:")
	log.Printf("TxID: %s", tx.TxHash())
	log.Printf("Version: %d", tx.Version)
	log.Printf("Locktime: %d", tx.LockTime)

	log.Println("Inputs:")
	for i, input := range tx.TxIn {
		log.Printf("  Input %d:", i)
		log.Printf("    Previous OutPoint: %s", input.PreviousOutPoint)
		log.Printf("    Sequence: %d", input.Sequence)
		log.Printf("    SignatureScript: %x", input.SignatureScript)
		if len(input.Witness) > 0 {
			log.Printf("    Witness: %x", input.Witness)
		}
	}

	log.Println("Outputs:")
	for i, output := range tx.TxOut {
		log.Printf("  Output %d:", i)
		log.Printf("    Value: %d satoshis", output.Value)
		log.Printf("    PkScript: %x", output.PkScript)
	}

	// Save the transaction in the database
	_, err = walletstatedb.SaveTransactionToDB(tx)
	if err != nil {
		return chainhash.Hash{}, false, err
	}
	// Create Electrum client once
	electrumClient, err := CreateElectrumClient(ElectrumConfig{
		ServerAddr: "electrum.blockstream.info:50002",
		UseSSL:     true,
	})
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to create Electrum client: %v", err)
	}
	defer electrumClient.Shutdown()

	txHash, verified, err := broadcastAndVerifyTransaction(tx, service)
	if err != nil {
		// Release the output we tried to spend
		releaseErr := w.ReleaseOutput(wtxmgr.LockID(tx.TxHash()), tx.TxIn[0].PreviousOutPoint)
		if releaseErr != nil {
			log.Printf("Failed to release output: %v", releaseErr)
		}
		return chainhash.Hash{}, false, fmt.Errorf("failed to broadcast and verify transaction: %v", err)
	}

	log.Println("Transaction broadcast and verified successfully.")
	return txHash, verified, nil
}
