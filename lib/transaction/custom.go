package transaction

import (
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

	// Select all UTXOs
	var selectedUTXOs []*btcjson.ListUnspentResult
	var totalSelected btcutil.Amount
	for _, utxo := range utxos {
		selectedUTXOs = append(selectedUTXOs, utxo)
		totalSelected += btcutil.Amount(utxo.Amount * btcutil.SatoshiPerBitcoin)
	}

	// Create new transaction
	tx := wire.NewMsgTx(wire.TxVersion)

	// Add inputs
	for _, utxo := range selectedUTXOs {
		prevOutHash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to parse txid: %v", err)
		}
		prevOut := wire.NewOutPoint(prevOutHash, utxo.Vout)
		txIn := wire.NewTxIn(prevOut, nil, nil)
		if enableRBF {
			txIn.Sequence = RBFSequenceNumber
		}
		tx.AddTxIn(txIn)
	}

	// Add recipient output
	recipientAddr, err := btcutil.DecodeAddress(recipientAddress, w.ChainParams())
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to decode recipient address: %v", err)
	}
	pkScript, err := txscript.PayToAddrScript(recipientAddr)
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to create output script: %v", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(amountToSend), pkScript))

	// Add OP_RETURN output with file hash
	opReturnScript, err := txscript.NullDataScript([]byte(fileHash))
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to create OP_RETURN script: %v", err)
	}
	tx.AddTxOut(wire.NewTxOut(0, opReturnScript))

	// Calculate transaction size and required fee
	txSize := tx.SerializeSize()
	requiredFee := btcutil.Amount(txSize * feeRate)

	// Calculate change
	changeAmount := totalSelected - amountToSend - requiredFee

	// Always add a change output
	changeAddr, err := getChangeAddress(w)
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to get change address: %v", err)
	}
	changePkScript, err := txscript.PayToAddrScript(changeAddr)
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to create change script: %v", err)
	}
	tx.AddTxOut(wire.NewTxOut(int64(changeAmount), changePkScript))

	// Recalculate the fee with the change output
	txSize = tx.SerializeSize()
	requiredFee = btcutil.Amount(txSize * feeRate)
	changeAmount = totalSelected - amountToSend - requiredFee
	tx.TxOut[len(tx.TxOut)-1].Value = int64(changeAmount)

	// Sign the transaction
	for i, txIn := range tx.TxIn {
		utxo := selectedUTXOs[i]
		sigScript, witness, err := createSignature(w, tx, i, utxo, pkScript)
		if err != nil {
			return chainhash.Hash{}, false, fmt.Errorf("failed to create signature: %v", err)
		}
		txIn.SignatureScript = sigScript
		txIn.Witness = witness
	}

	// Verify the transaction
	err = validateTransaction(tx, selectedUTXOs)
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("transaction validation failed: %v", err)
	}

	// Log transaction details
	logTransactionDetails(tx, amountToSend, requiredFee, totalSelected, changeAmount, fileHash)

	// Save the transaction in the database
	_, err = walletstatedb.SaveTransactionToDB(tx)
	if err != nil {
		return chainhash.Hash{}, false, err
	}

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

func logTransactionDetails(tx *wire.MsgTx, amountToSend, requiredFee, totalSelected, changeAmount btcutil.Amount, fileHash string) {
	log.Printf("Transaction created successfully. Details:")
	log.Printf("  TxID: %s", tx.TxHash().String())
	log.Printf("  Amount to send: %d satoshis", amountToSend)
	log.Printf("  Fee: %d satoshis", requiredFee)
	log.Printf("  Total input: %d satoshis", totalSelected)
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
}
