package ipc

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
)

const unixSocketPath = "/tmp/btc-wallet.sock"
const windowsSocketPort = ":7070" // You can change the port if needed

var commandID int
var osType = runtime.GOOS // Get the operating system type

func generateCommandID() int {
	commandID++
	return commandID
}

func NewServer() (*Server, error) {
	var listener net.Listener
	var err error

	if osType == "windows" {
		// On Windows, use TCP socket
		listener, err = net.Listen("tcp", windowsSocketPort)
	} else {
		// On Unix-like systems, use Unix socket
		// Check if the Unix socket file already exists
		if _, err := os.Stat(unixSocketPath); err == nil {
			// Remove existing Unix socket file
			err = os.Remove(unixSocketPath)
			if err != nil {
				return nil, fmt.Errorf("failed to remove existing socket file: %v", err)
			}
		}
		listener, err = net.Listen("unix", unixSocketPath)
	}

	if err != nil {
		return nil, err
	}

	server := &Server{
		listener:    listener,
		commands:    make(chan Command),
		connections: make(map[int]net.Conn),
		subscribers: make(map[net.Conn]bool),
	}

	go server.accept()

	return server, nil
}

// func NewServer() (*Server, error) {
// 	// Check if the socket file already exists
// 	if _, err := os.Stat(socketPath); err == nil {
// 		// Socket file exists, try to remove it
// 		err = os.Remove(socketPath)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to remove existing socket file: %v", err)
// 		}
// 	}

// 	// Create a new Unix socket
// 	listener, err := net.Listen("unix", socketPath)
// 	if err != nil {
// 		return nil, err
// 	}

// 	server := &Server{
// 		listener:    listener,
// 		commands:    make(chan Command),
// 		connections: make(map[int]net.Conn), // Initialize the connections map here
// 	}

// 	go server.accept()

// 	return server, nil
// }

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
	defer func() {
		s.RemoveSubscriber(conn)
		conn.Close()
	}()

	// Add connection as subscriber for progress updates
	s.AddSubscriber(conn)

	// Handle incoming commands
	buffer := make([]byte, 65536) // 64KB buffer

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("Failed to read from connection: %v", err)
			}
			return
		}

		// Check if this is a command message (has an ID field)
		var message struct {
			ID int `json:"id"`
		}

		if err := json.Unmarshal(buffer[:n], &message); err != nil {
			log.Printf("Failed to parse message: %v", err)
			continue
		}

		// If it has an ID, it's a command
		if message.ID > 0 {
			var cmd Command
			if err := json.Unmarshal(buffer[:n], &cmd); err != nil {
				log.Printf("Failed to unmarshal command: %v", err)
				continue
			}

			// Store the connection with the command ID for response
			s.mutex.Lock()
			s.connections[cmd.ID] = conn
			s.mutex.Unlock()

			s.commands <- cmd
		}
	}
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

func (s *Server) BroadcastProgress(update SyncProgressUpdate) {
	data, err := json.Marshal(update)
	if err != nil {
		log.Printf("Failed to marshal progress update: %v", err)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	for conn := range s.subscribers {
		_, err := conn.Write(data)
		if err != nil {
			log.Printf("Failed to send progress update: %v", err)
			// Remove failed connection from subscribers
			delete(s.subscribers, conn)
		}
	}
}

func (s *Server) AddSubscriber(conn net.Conn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.subscribers[conn] = true
}

func (s *Server) RemoveSubscriber(conn net.Conn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.subscribers, conn)
}

func (s *Server) Close() error {
	return s.listener.Close()
}

func NewClient() (*Client, error) {
	var conn net.Conn
	var err error

	if osType == "windows" {
		conn, err = net.Dial("tcp", windowsSocketPort)
	} else {
		conn, err = net.Dial("unix", unixSocketPath)
	}

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
