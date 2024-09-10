package types

type Config struct {
	BackendURL string   `mapstructure:"backend_url"`
	AddPeers   []string `mapstructure:"add_peers"`
}
