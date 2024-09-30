package ipc

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
)

const socketPath = "/tmp/btc-wallet.sock"

var commandID int

func generateCommandID() int {
	commandID++
	return commandID
}

func NewServer() (*Server, error) {
	// Check if the socket file already exists
	if _, err := os.Stat(socketPath); err == nil {
		// Socket file exists, try to remove it
		err = os.Remove(socketPath)
		if err != nil {
			return nil, fmt.Errorf("failed to remove existing socket file: %v", err)
		}
	}

	// Create a new Unix socket
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener:    listener,
		commands:    make(chan Command),
		connections: make(map[int]net.Conn), // Initialize the connections map here
	}

	go server.accept()

	return server, nil
}

func (s *Server) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	// Don't use defer conn.Close() here
	var cmd Command
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&cmd); err != nil {
		fmt.Printf("Error decoding command: %v\n", err)
		return
	}

	// Store the connection with the command ID
	s.mutex.Lock()
	s.connections[cmd.ID] = conn
	s.mutex.Unlock()

	s.commands <- cmd
}

func (s *Server) Commands() <-chan Command {
	return s.commands
}

func (s *Server) SendResponse(id int, response Response) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	conn, exists := s.connections[id]
	if !exists {
		fmt.Printf("Connection for command ID %d not found\n", id)
		return
	}

	// Marshal the Response type into JSON
	responseData, err := json.Marshal(response)
	if err != nil {
		fmt.Printf("Error marshaling response for command ID %d: %v\n", id, err)
		return
	}

	// Write the marshaled response to the connection
	_, err = conn.Write(responseData)
	if err != nil {
		fmt.Printf("Error writing response for command ID %d: %v\n", id, err)
		return
	}

	// Close the connection after sending the response
	conn.Close()
	delete(s.connections, id)
}

func (s *Server) Close() error {
	return s.listener.Close()
}

func NewClient() (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}

	return &Client{conn: conn}, nil
}

func (c *Client) SendCommand(command string, args []string) (interface{}, error) {
	cmd := Command{
		ID:      generateCommandID(),
		Command: command,
		Args:    args,
	}

	// Marshal the Command type directly to JSON
	cmdData, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("error marshaling command: %v", err)
	}

	// Write the marshaled Command JSON data to the connection
	_, err = c.conn.Write(cmdData)
	if err != nil {
		return nil, fmt.Errorf("error writing command to connection: %v", err)
	}

	// Use io.ReadAll to read the complete response
	responseData, err := io.ReadAll(c.conn)
	if err != nil {
		return nil, fmt.Errorf("error reading response from connection: %v", err)
	}

	// Unmarshal the response into the Response struct
	var response Response
	err = json.Unmarshal(responseData, &response)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	// Check for errors in the response
	if response.Error != nil {
		return nil, response.Error
	}

	return response.Result, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}
