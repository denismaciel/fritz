package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

type SocketConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func DialSocket(ctx context.Context, url string) (*SocketConn, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	return &SocketConn{conn: conn}, nil
}

func (s *SocketConn) ReadEnvelope() (SocketEnvelope, error) {
	if s == nil || s.conn == nil {
		return SocketEnvelope{}, fmt.Errorf("socket not connected")
	}
	_, data, err := s.conn.ReadMessage()
	if err != nil {
		return SocketEnvelope{}, err
	}
	var envelope SocketEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return SocketEnvelope{}, err
	}
	return envelope, nil
}

func (s *SocketConn) Ack(envelopeID string) error {
	if s == nil || s.conn == nil {
		return fmt.Errorf("socket not connected")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteJSON(map[string]string{"envelope_id": envelopeID})
}

func (s *SocketConn) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}
