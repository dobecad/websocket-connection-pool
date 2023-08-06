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
}

type PoolConfig struct {
	wsUrl           string
	capacity        uint32
	createConn      func(string) (*websocket.Conn, error)
	maxConnLifetime time.Duration
}

var (
	ErrAllConnectionsAcquired = errors.New("all connections have been acquired")
)

func NewPool(wsUrl string, config *PoolConfig) *Pool {
	pool := DefaultPool(wsUrl)
	pool.maxConnLifetime = config.maxConnLifetime
	pool.capacity = config.capacity
	pool.createConn = config.createConn

	// pool.initializeConnections()
	return pool
}

func DefaultPool(wsUrl string) *Pool {
	return &Pool{
		capacity:        5,
		maxConnLifetime: time.Minute * 5,
		createConn:      defaultCreator,
		wsUrl:           wsUrl,
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
}

func (p *Pool) ReleaseAllConnections() {
	for _, conn := range p.connections {
		p.ReleaseConnection(conn)
	}
}

func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.connections {
		if err := conn.Close(); err != nil {
			return err
		}
		p.numActive--
	}
	return nil
}
