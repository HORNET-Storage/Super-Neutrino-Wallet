package wallet

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"golang.org/x/crypto/ripemd160"
)

// deriveKeyFromPath derives the extended key from the given path
func deriveKeyFromPath(rootKey *hdkeychain.ExtendedKey, path string) (*hdkeychain.ExtendedKey, error) {
	parts := strings.Split(path, "/")
	key := rootKey
	for _, part := range parts {
		var index uint32
		var err error
		if strings.HasSuffix(part, "'") {
			index64, err := strconv.ParseUint(part[:len(part)-1], 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid path component %s: %v", part, err)
			}
			index = hdkeychain.HardenedKeyStart + uint32(index64)
		} else {
			index64, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("invalid path component %s: %v", part, err)
			}
			index = uint32(index64)
		}
		key, err = key.Derive(index)
		if err != nil {
			return nil, fmt.Errorf("failed to derive key: %v", err)
		}
	}
	return key, nil
}

// getMasterFingerprint calculates the master fingerprint from the root key
func GetMasterFingerprint(rootKey *hdkeychain.ExtendedKey) (uint32, error) {
	pubKey, err := rootKey.ECPubKey()
	if err != nil {
		return 0, fmt.Errorf("failed to get public key from root key: %v", err)
	}

	sha := sha256.New()
	_, err = sha.Write(pubKey.SerializeCompressed())
	if err != nil {
		return 0, fmt.Errorf("failed to write sha256: %v", err)
	}
	hash160 := ripemd160.New()
	_, err = hash160.Write(sha.Sum(nil))
	if err != nil {
		return 0, fmt.Errorf("failed to write ripemd160: %v", err)
	}
	fingerprint := hash160.Sum(nil)[:4]

	return binary.BigEndian.Uint32(fingerprint), nil
}

// getExtendedPubKey converts an extended key to its string representation with the given version bytes
func GetExtendedPubKey(extendedKey *hdkeychain.ExtendedKey, version []byte) (string, error) {
	neuteredKey, err := extendedKey.Neuter()
	if err != nil {
		return "", err
	}
	clonedKey, err := neuteredKey.CloneWithVersion(version)
	if err != nil {
		return "", err
	}
	return clonedKey.String(), nil
}

func estimateBlockHeight(targetDate time.Time) int32 {
	genesisDate := time.Date(2009, time.January, 3, 18, 15, 5, 0, time.UTC)
	daysSinceGenesis := targetDate.Sub(genesisDate).Hours() / 24
	estimatedHeight := int32(daysSinceGenesis * 144)
	return estimatedHeight
}

func gracefulShutdown() error {
	time.Sleep(1 * time.Second)
	fmt.Println("Shutdown complete. Goodbye!")
	err := setWalletLive(false)
	if err != nil {
		log.Printf("Error setting wallet live state: %v", err)
	}

	err = setWalletSync(false)
	if err != nil {
		log.Printf("Error setting wallet sync state: %v", err)
	}

	time.Sleep(2 * time.Second) // Give user time to read the message
	os.Exit(0)
	return nil
}
