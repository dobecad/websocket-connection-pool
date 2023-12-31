package pool

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}
var once sync.Once

func newTestWebSocketPool() *Pool {
	return DefaultPool("ws://localhost:8080/ws")
}

func TestNewWebSocketPool(t *testing.T) {
	poolOpts := &PoolConfig {
		Capacity: 5,
		PingInterval: time.Minute,
		CreateConn: func (wsUrl string) (*websocket.Conn, error) {
			conn, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
	}
	pool := NewPool("ws://localhost:8080/ws", poolOpts)

	if pool.capacity != 5 {
		t.Fatalf("Pool capacity does not match config: %d vs %d", pool.capacity, poolOpts.Capacity)
	}

	if pool.pingInterval != time.Minute {
		t.Fatalf("Pool pingInterval does not match config")
	}
}

func TestWebSocketServer(t *testing.T) {
	once.Do(func() {
		http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
			// Set up WebSocket connection
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Fatalf("Failed to upgrade WebSocket connection: %v", err)
			}
			defer conn.Close()
	
			// Simple echo server behavior
			for {
				msgType, msg, err := conn.ReadMessage()
				if err != nil {
					break
				}
	
				// Respond with a Pong message to keep the connection alive
				if msgType == websocket.PingMessage {
					conn.WriteMessage(websocket.PongMessage, nil)
					continue
				}
	
				// Echo the received message back to the client
				conn.WriteMessage(msgType, msg)
			}
		})
	
		// Start the test WebSocket server in a goroutine
		go func() {
			err := http.ListenAndServe(":8080", nil)
			if err != nil {
				panic(fmt.Sprintf("Failed to start test WebSocket server: %v", err))
			}
		}()
	
		// Wait for the test WebSocket server to start
		time.Sleep(time.Second)
	})
}

func TestConnectionPool(t *testing.T) {
	TestWebSocketServer(t)
	pool := newTestWebSocketPool()
	defer pool.Close()

	for i := 0; i < int(pool.capacity); i++ {
		conn, err := pool.GetConnection()
		if err != nil {
			t.Errorf("Failed to acquire connection: %v", err)
		}

		time.Sleep(100 * time.Millisecond)
		pool.ReleaseConnection(conn)
	}

	for i := 0; i < int(pool.capacity); i++ {
		if _, err := pool.GetConnection(); err != nil {
			t.Errorf("Failed to acquire connection: %v", err)
		}
	}

	if _, err := pool.GetConnection(); err == nil {
		t.Errorf("Expected an error, but got connection")
	}
}

func TestMaxConnections(t *testing.T) {
	TestWebSocketServer(t)
	pool := newTestWebSocketPool()
	defer pool.Close()

	// Acquire all connections
	for i := 0; i < int(pool.capacity); i++ {
		_, err := pool.GetConnection()
		if err != nil {
			t.Errorf("Failed to acquire connection: %v", err)
		}
	}

	// Test that exceeding max connections returns an error
	_, err := pool.GetConnection()
	if err == nil {
		t.Errorf("Expected an error, but got connection")
	}
	if err != nil && err != ErrAllConnectionsAcquired {
		t.Errorf("Expected ErrBadHandshake, but got: %v", err)
	}
}

func TestClose(t *testing.T) {
	TestWebSocketServer(t)
	pool := newTestWebSocketPool()

	if pool.numActive != 0 {
		t.Errorf("Expected 0 active connection, but got: %d", pool.numActive)
	}

	// Acquire a connection and check the active count
	conn, err := pool.GetConnection()
	if err != nil {
		t.Errorf("Failed to acquire connection: %v", err)
	}

	if pool.numActive != 1 {
		t.Errorf("Expected 1 active connection, but got: %d", pool.numActive)
	}

	// Close the pool and check if the connection is closed
	pool.ReleaseConnection(conn)

	if pool.numActive != 0 {
		t.Errorf("Expected 0 active connection after closing, but got: %d", pool.numActive)
	}
}

func TestConcurrentAccess(t *testing.T) {
	TestWebSocketServer(t)
	pool := newTestWebSocketPool()
	defer pool.Close()

	numAcquire := 100
	numRelease := 50
	conns := make([]*websocket.Conn, 0)

	// Concurrently acquire connections
	acquireDone := make(chan struct{})
	for i := 0; i < numAcquire; i++ {
		go func() {
			conn, err := pool.GetConnection()
			if err == nil {
				conns = append(conns, conn)
			}
			acquireDone <- struct{}{}
		}()
	}

	// Concurrently release connections
	for i := 0; i < numRelease; i++ {
		go func() {
			if len(conns) > 1 {
				pool.ReleaseConnection(conns[0])
				conns = conns[1:]
			}
		}()
	}

	// Wait for all acquisitions to complete
	for i := 0; i < numAcquire; i++ {
		<-acquireDone
	}

	// Ensure that the number of active connections is within the pool capacity
	if pool.numActive > pool.capacity {
		t.Errorf("Expected number of active connections (%d) to be within the pool capacity (%d)", pool.numActive, pool.capacity)
	}
}
