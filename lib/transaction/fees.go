package transaction

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
