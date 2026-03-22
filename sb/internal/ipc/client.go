package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/marcusguttenplan/sb/internal/config"
)

// Command is a JSON message sent to Saddlebag.app over Unix socket
type Command struct {
	Action string `json:"action"`
	Desk   string `json:"desk,omitempty"`
}

// Response is a JSON message received from Saddlebag.app
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Send sends a command to Saddlebag.app and returns the response
func Send(ctx context.Context, cmd Command) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", config.SocketPath())
	if err != nil {
		return nil, fmt.Errorf("connecting to saddlebag (is the app running?): %w", err)
	}
	defer conn.Close()

	// Set deadline from context
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Send command
	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(cmd); err != nil {
		return nil, fmt.Errorf("sending command: %w", err)
	}

	// Read response
	decoder := json.NewDecoder(conn)
	var resp Response
	if err := decoder.Decode(&resp); err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.Error != "" {
		return &resp, fmt.Errorf("app error: %s", resp.Error)
	}

	return &resp, nil
}
