package pool

import (
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Pool struct {
	mu           sync.Mutex
	wsUrl        string
	connections  []*websocket.Conn
	numActive    uint32
	capacity     uint32
	createConn   func(string) (*websocket.Conn, error)
	pingInterval time.Duration
	done         chan struct{}
}

type PoolConfig struct {
	// Max number of connections to keep in the Pool. Minimum is 2 connections
	Capacity uint32

	// Function for creating the websocket connection.
	//
	// Allows you to define how your websocket connection should be made
	CreateConn func(string) (*websocket.Conn, error)

	// How often your websocket should ping the server to
	// keep the connection alive
	PingInterval time.Duration
}

const (
	DefaultPingInterval = 30 * time.Second
	MinCapacity         = 2
	DefaultCapacity     = 5
)

var (
	ErrAllConnectionsAcquired = errors.New("all connections have been acquired")
)

// Create a new Websocket Connection Pool
func NewPool(wsUrl string, config *PoolConfig) *Pool {
	pool := DefaultPool(wsUrl)
	if config.Capacity < MinCapacity {
		config.Capacity = MinCapacity
	}
	pool.capacity = config.Capacity
	pool.createConn = config.CreateConn
	pool.pingInterval = config.PingInterval
	return pool
}

// Create a new Websocket Connection Pool using default settings
func DefaultPool(wsUrl string) *Pool {
	return &Pool{
		capacity:     DefaultCapacity,
		createConn:   defaultCreator,
		wsUrl:        wsUrl,
		pingInterval: DefaultPingInterval,
		done:         make(chan struct{}),
	}
}

// Create a simple WS connection
func defaultCreator(wsUrl string) (*websocket.Conn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Grab a connection from the Pool
func (p *Pool) GetConnection() (*websocket.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.connections) > 0 {
		conn := p.connections[0]
		p.connections = p.connections[1:]
		p.numActive++
		return conn, nil
	}

	if p.numActive < p.capacity {
		conn, err := p.createConn(p.wsUrl)
		if err != nil {
			return nil, err
		}
		p.numActive++
		return conn, nil
	}

	if p.numActive == p.capacity {
		return nil, ErrAllConnectionsAcquired
	}

	conn := p.connections[0]
	p.connections = p.connections[1:]
	p.numActive++
	return conn, nil
}

// Release a connection so that it goes back to the Pool
func (p *Pool) ReleaseConnection(conn *websocket.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.connections = append(p.connections, conn)
	p.numActive--
	go p.keepConnectionAlive(conn)
}

// Pings the WS server to keep the connection alive
func (pool *Pool) keepConnectionAlive(conn *websocket.Conn) {
	ticker := time.NewTicker(pool.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Send a Ping message to keep the connection alive
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				// If the Ping fails, assume the connection is broken and close it
				conn.Close()
				return
			}
		case <-pool.done:
			return
		}
	}
}

// Close all of the connections in the connection Pool
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop the keepConnectionAlive routine for the connection
	close(p.done)

	for _, conn := range p.connections {
		if err := conn.Close(); err != nil {
			return err
		}
		p.numActive--
	}
	return nil
}
