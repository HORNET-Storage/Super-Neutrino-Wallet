package ipc

import (
	"net"
	"sync"
)

type Command struct {
	ID      int      `json:"id"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Response struct {
	ID     int         `json:"id"`
	Error  error       `json:"error,omitempty"`
	Result interface{} `json:"result,omitempty"`
}

type SyncProgressUpdate struct {
	Type         string  `json:"type"`
	Progress     float64 `json:"progress"`
	CurrentBlock int32   `json:"current_block"`
	TargetBlock  int32   `json:"target_block"`
	ChainSynced  bool    `json:"chain_synced"`
	ScanProgress float64 `json:"scan_progress"`
	Stage        string  `json:"stage"` // "chain_sync", "address_scan", or "complete"
}

type Server struct {
	listener    net.Listener
	commands    chan Command
	mutex       sync.Mutex
	connections map[int]net.Conn  // Maps command ID to the client connection
	subscribers map[net.Conn]bool // Active connections for progress updates
}

type Client struct {
	conn net.Conn
}
