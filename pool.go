package pool

import (
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Pool struct {
	mu              sync.Mutex
	wsUrl           string
	connections     []*websocket.Conn
	numActive       uint32
	capacity        uint32
	createConn      func(string) (*websocket.Conn, error)
	maxConnLifetime time.Duration
	pingInterval    time.Duration
	done            chan struct{}
}

type PoolConfig struct {
	wsUrl           string
	capacity        uint32
	createConn      func(string) (*websocket.Conn, error)
	maxConnLifetime time.Duration
	pingInterval    time.Duration
}

const (
	DefaultPingInterval = 30 * time.Second
)

var (
	ErrAllConnectionsAcquired = errors.New("all connections have been acquired")
)

func NewPool(wsUrl string, config *PoolConfig) *Pool {
	pool := DefaultPool(wsUrl)
	pool.maxConnLifetime = config.maxConnLifetime
	pool.capacity = config.capacity
	pool.createConn = config.createConn
	pool.pingInterval = config.pingInterval
	return pool
}

func DefaultPool(wsUrl string) *Pool {
	return &Pool{
		capacity:        5,
		maxConnLifetime: time.Minute * 5,
		createConn:      defaultCreator,
		wsUrl:           wsUrl,
		pingInterval:    DefaultPingInterval,
		done:            make(chan struct{}),
	}
}

func defaultCreator(wsUrl string) (*websocket.Conn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsUrl, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (p *Pool) initializeConnections() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := 0; i < int(p.capacity); i++ {
		conn, err := p.createConn(p.wsUrl)
		if err != nil {
			return err
		}
		p.connections = append(p.connections, conn)
	}
	return nil
}

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

func (p *Pool) ReleaseConnection(conn *websocket.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.connections = append(p.connections, conn)
	p.numActive--
	go p.keepConnectionAlive(conn)
}

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
