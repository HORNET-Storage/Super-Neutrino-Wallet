package transaction

type FeeRecommendation struct {
	FastestFee  int `json:"fastestFee"`
	HalfHourFee int `json:"halfHourFee"`
	HourFee     int `json:"hourFee"`
	EconomyFee  int `json:"economyFee"`
	MinimumFee  int `json:"minimumFee"`
}

// Config holds the configuration for the Electrum broadcaster
type ElectrumConfig struct {
	ServerAddr string
	UseSSL     bool
}
