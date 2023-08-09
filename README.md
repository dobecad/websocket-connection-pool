# websocket-connection-pool

Websocket connection pooling for gorilla/websocket

## Import

```bash
go get github.com/dobecad/websocket-connection-pool
```

## Usage
```go
// Create a new Pool using default settings
pool := DefaultPool("ws://localhost:8080/ws")

// Grab a connection from the Pool
conn, err := pool.GetConnection()

// Release the connection to put the connection back in the pool
defer pool.ReleaseConnection(conn)
```
