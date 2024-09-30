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

type Server struct {
	listener    net.Listener
	commands    chan Command
	mutex       sync.Mutex
	connections map[int]net.Conn // Maps command ID to the client connection
}

type Client struct {
	conn net.Conn
}
