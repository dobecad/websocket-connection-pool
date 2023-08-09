# websocket-connection-pool

Minimalistic Websocket connection pool library for gorilla/websocket

## Import

```bash
go get github.com/dobecad/websocket-connection-pool
```

## Usage

```go
// Create a new Pool using default settings
pool := DefaultPool("ws://localhost:8080/ws")

// or use the following PoolConfig to customize the default pool
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

// Grab a connection from the Pool
conn, err := pool.GetConnection()

// Release the connection to put the connection back in the pool
defer pool.ReleaseConnection(conn)
```
