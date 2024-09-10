package transaction

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/wallet"
	"github.com/btcsuite/btcwallet/walletdb"
)

// Verify the signature of a transaction input
func verifySignature(tx *wire.MsgTx, index int, scriptPubKey []byte, amount int64) (bool, error) {
	flags := txscript.StandardVerifyFlags

	// Create the PrevOutputFetcher with the previous output's information
	prevOutputs := txscript.NewCannedPrevOutputFetcher(scriptPubKey, amount)

	engine, err := txscript.NewEngine(scriptPubKey, tx, index, flags, nil, nil, amount, prevOutputs)
	if err != nil {
		return false, fmt.Errorf("failed to create script engine: %v", err)
	}
	err = engine.Execute()
	if err != nil {
		return false, fmt.Errorf("failed to execute script: %v", err)
	}
	return true, nil
}

// Helper function to check if an address is SegWit
func isSegWitAddress(addr btcutil.Address) bool {
	switch addr.(type) {
	case *btcutil.AddressWitnessPubKeyHash, *btcutil.AddressWitnessScriptHash:
		return true
	default:
		return false
	}
}

func UpdateUTXOs(w *wallet.Wallet) error {
	unspent, err := w.ListUnspent(1, 999999999, "")
	if err != nil {
		return fmt.Errorf("failed to list unspent outputs: %v", err)
	}

	log.Println("Unspent outputs:")
	for i, utxo := range unspent {
		log.Printf("  UTXO %d:", i)
		log.Printf("    TxID: %s", utxo.TxID)
		log.Printf("    Vout: %d", utxo.Vout)
		log.Printf("    Amount: %f", utxo.Amount)
		log.Printf("    Address: %s", utxo.Address)
		log.Printf("    ScriptPubKey: %s", utxo.ScriptPubKey)
		log.Printf("    Confirmations: %d", utxo.Confirmations)
	}

	return nil
}

func VerifyUTXO(txID string, vout uint32) error {
	url := fmt.Sprintf("https://mempool.space/api/tx/%s/outspends", txID)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch UTXO status: %v", err)
	}
	defer resp.Body.Close()

	var outspends []struct {
		Spent bool `json:"spent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&outspends); err != nil {
		return fmt.Errorf("failed to decode UTXO status: %v", err)
	}

	if vout >= uint32(len(outspends)) {
		return fmt.Errorf("invalid vout: %d", vout)
	}

	if outspends[vout].Spent {
		return fmt.Errorf("UTXO is already spent")
	}

	return nil
}

func getFeeRecommendation() (FeeRecommendation, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://mempool.space/api/v1/fees/recommended")
	if err != nil {
		return FeeRecommendation{}, err
	}
	defer resp.Body.Close()

	var feeRec FeeRecommendation
	err = json.NewDecoder(resp.Body).Decode(&feeRec)
	return feeRec, err
}

func getUserFeePriority(feeRec FeeRecommendation) (int, error) {
	fmt.Println("Choose your fee priority:")
	fmt.Printf("1. Fastest (%.2f sat/vB)\n", float64(feeRec.FastestFee))
	fmt.Printf("2. Half Hour (%.2f sat/vB)\n", float64(feeRec.HalfHourFee))
	fmt.Printf("3. Hour (%.2f sat/vB)\n", float64(feeRec.HourFee))
	fmt.Printf("4. Economy (%.2f sat/vB)\n", float64(feeRec.EconomyFee))
	fmt.Printf("5. Minimum (%.2f sat/vB)\n", float64(feeRec.MinimumFee))
	fmt.Print("Enter your choice (1-5): ")

	var input string
	fmt.Scanln(&input)
	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > 5 {
		return 0, fmt.Errorf("invalid choice")
	}

	switch choice {
	case 1:
		return feeRec.FastestFee, nil
	case 2:
		return feeRec.HalfHourFee, nil
	case 3:
		return feeRec.HourFee, nil
	case 4:
		return feeRec.EconomyFee, nil
	case 5:
		return feeRec.MinimumFee, nil
	default:
		return 0, fmt.Errorf("unexpected error")
	}
}

func findUnusedChangeAddress(w *wallet.Wallet) (btcutil.Address, error) {
	var changeAddr btcutil.Address
	var maxAddressesToCheck uint32

	err := walletdb.View(w.Database(), func(tx walletdb.ReadTx) error {
		scopedMgr, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeBIP0084)
		if err != nil {
			return fmt.Errorf("failed to fetch scoped key manager: %v", err)
		}

		addrmgrNs := tx.ReadBucket([]byte("waddrmgr"))
		props, err := scopedMgr.AccountProperties(addrmgrNs, 0)
		if err != nil {
			return fmt.Errorf("failed to get account properties: %v", err)
		}

		maxAddressesToCheck = props.InternalKeyCount
		return nil
	})

	if err != nil {
		return nil, err
	}

	transactions, err := w.ListTransactions(0, 1<<31-1)
	if err != nil {
		return nil, fmt.Errorf("error listing transactions: %v", err)
	}

	usedAddresses := make(map[string]bool)
	for _, tx := range transactions {
		usedAddresses[tx.Address] = true
	}

	for i := uint32(0); i < maxAddressesToCheck; i++ {
		err = walletdb.View(w.Database(), func(tx walletdb.ReadTx) error {
			scopedMgr, err := w.Manager.FetchScopedKeyManager(waddrmgr.KeyScopeBIP0084)
			if err != nil {
				return fmt.Errorf("failed to fetch scoped key manager: %v", err)
			}

			addrmgrNs := tx.ReadBucket([]byte("waddrmgr"))
			maddr, err := scopedMgr.DeriveFromKeyPath(addrmgrNs, waddrmgr.DerivationPath{
				Account: 0,
				Branch:  1, // Internal addresses
				Index:   i,
			})
			if err != nil {
				return fmt.Errorf("failed to derive address: %v", err)
			}

			addr := maddr.Address()
			if !usedAddresses[addr.String()] {
				changeAddr = addr
				return nil
			}
			return nil
		})

		if err != nil {
			return nil, err
		}

		if changeAddr != nil {
			log.Printf("Using existing unused change address: %s", changeAddr.String())
			return changeAddr, nil
		}
	}

	// If no unused address found, create a new one
	newAddr, err := w.NewChangeAddress(0, waddrmgr.KeyScopeBIP0084)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new change address: %v", err)
	}
	log.Printf("Generated new change address: %s", newAddr.String())
	return newAddr, nil
}

func getChangeAddress(w *wallet.Wallet) (btcutil.Address, error) {
	changeAddr, err := findUnusedChangeAddress(w)
	if err != nil {
		return nil, fmt.Errorf("failed to find or generate change address: %v", err)
	}
	return changeAddr, nil
}
