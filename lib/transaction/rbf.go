package transaction

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"sort"

	walletstatedb "github.com/Maphikza/btc-wallet-btcsuite.git/internal/database"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/checksum0/go-electrum/electrum"
	"github.com/lightninglabs/neutrino"
)

const (
	MaxStandardTxWeight = 400000
)

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
	txHash, verified, err := broadcastAndVerifyTransaction(newTx)
	if err != nil {
		return chainhash.Hash{}, false, fmt.Errorf("failed to broadcast and verify RBF transaction: %v", err)
	}

	log.Printf("RBF transaction successfully broadcast. New TxID: %s", txHash.String())
	return txHash, verified, nil
}

func selectAdditionalUTXOs(w *wallet.Wallet, amount btcutil.Amount) ([]*btcjson.ListUnspentResult, btcutil.Amount, error) {
	utxos, err := w.ListUnspent(1, 9999999, "")
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list unspent outputs: %v", err)
	}

	var selectedUTXOs []*btcjson.ListUnspentResult
	var totalSelected btcutil.Amount

	// Sort UTXOs by amount in descending order
	sort.Slice(utxos, func(i, j int) bool {
		return utxos[i].Amount > utxos[j].Amount
	})

	for _, utxo := range utxos {
		if totalSelected >= amount {
			break
		}
		selectedUTXOs = append(selectedUTXOs, utxo)
		totalSelected += btcutil.Amount(utxo.Amount * btcutil.SatoshiPerBitcoin)
	}

	if totalSelected < amount {
		return nil, 0, fmt.Errorf("insufficient funds to cover additional fee")
	}

	return selectedUTXOs, totalSelected, nil
}

func fetchUTXO(w *wallet.Wallet, outpoint *wire.OutPoint) (*btcjson.ListUnspentResult, error) {
	utxos, err := w.ListUnspent(1, 9999999, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list unspent outputs: %v", err)
	}

	for _, utxo := range utxos {
		if utxo.TxID == outpoint.Hash.String() && utxo.Vout == outpoint.Index {
			return utxo, nil
		}
	}

	return nil, fmt.Errorf("UTXO not found")
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
