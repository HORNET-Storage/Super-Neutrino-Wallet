package transaction

import (
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
	"github.com/lightninglabs/neutrino"
)

const (
	RBFSequenceNumber      = 0xffffffff - 2
	DustThreshold          = 546   // satoshis
	ConsolidationThreshold = 10000 // satoshis, adjust as needed
)

// var mempoolSpaceConfig = ElectrumConfig{
// 	ServerAddr: "electrum.blockstream.info:50002",
// 	UseSSL:     true,
// }

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

// HttpCalculateTransactionSize will estimate the transaction size based on the inputs.
func HttpCalculateTransactionSize(w *wallet.Wallet, spendAmount int64, recipientAddress string, feeRate int) (int, error) {
	log.Printf("Starting transaction size estimation process.")

	// Set recipient address and amount
	amountToSend := btcutil.Amount(spendAmount)

	// List unspent outputs
	utxos, err := w.ListUnspent(1, 9999999, "")
	if err != nil {
		log.Printf("Failed to list unspent outputs: %v", err)
		return 0, fmt.Errorf("failed to list unspent outputs: %v", err)
	}

	// Select suitable UTXO
	var selectedUTXO *btcjson.ListUnspentResult
	for _, utxo := range utxos {
		if btcutil.Amount(utxo.Amount*btcutil.SatoshiPerBitcoin) >= amountToSend+btcutil.Amount(feeRate) {
			selectedUTXO = utxo
			break
		}
	}

	if selectedUTXO == nil {
		log.Printf("No suitable UTXO found.")
		return 0, fmt.Errorf("no suitable UTXO found")
	}

	// Create new transaction
	tx := wire.NewMsgTx(wire.TxVersion)
	prevOutHash, err := chainhash.NewHashFromStr(selectedUTXO.TxID)
	if err != nil {
		return 0, fmt.Errorf("failed to parse txid: %v", err)
	}
	prevOut := wire.NewOutPoint(prevOutHash, selectedUTXO.Vout)
	txIn := wire.NewTxIn(prevOut, nil, nil)
	tx.AddTxIn(txIn)

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

	// Estimate the size of the transaction
	txSize := tx.SerializeSize()
	return txSize, nil
}
