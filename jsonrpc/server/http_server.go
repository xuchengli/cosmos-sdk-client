package server

import (
    "encoding/json"
    "fmt"
    "net"
    "net/http"
    "strings"
    "time"

    "golang.org/x/net/netutil"

    types "github.com/tendermint/tendermint/rpc/jsonrpc/types"
)

// Config is a RPC server configuration.
type Config struct {
	// see netutil.LimitListener
	MaxOpenConnections int
	// mirrors http.Server#ReadTimeout
	ReadTimeout time.Duration
	// mirrors http.Server#WriteTimeout
	WriteTimeout time.Duration
	// MaxBodyBytes controls the maximum number of bytes the
	// server will read parsing the request body.
	MaxBodyBytes int64
	// mirrors http.Server#MaxHeaderBytes
	MaxHeaderBytes int
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxOpenConnections: 0, // unlimited
		ReadTimeout:        10 * time.Second,
		WriteTimeout:       10 * time.Second,
		MaxBodyBytes:       int64(1000000), // 1MB
		MaxHeaderBytes:     1 << 20,        // same as the net/http default
	}
}

// Listen starts a new net.Listener on the given address.
func Listen(addr string, maxOpenConnections int) (listener net.Listener, err error) {
    parts := strings.SplitN(addr, "://", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf(
			"invalid listening address %s (use fully formed addresses, including the tcp:// or unix:// prefix)",
			addr,
		)
	}
    proto, addr := parts[0], parts[1]
	listener, err = net.Listen(proto, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %v: %v", addr, err)
	}
    if maxOpenConnections > 0 {
		listener = netutil.LimitListener(listener, maxOpenConnections)
	}

	return listener, nil
}

// Serve creates a http.Server and calls Serve with the given listener.
func Serve(listener net.Listener, handler http.Handler, config *Config) error {
    fmt.Printf("Starting RPC HTTP server on %s\n", listener.Addr())

	s := &http.Server{
		Handler:           handler,
		ReadTimeout:       config.ReadTimeout,
		ReadHeaderTimeout: config.ReadTimeout,
		WriteTimeout:      config.WriteTimeout,
		MaxHeaderBytes:    config.MaxHeaderBytes,
	}
	err := s.Serve(listener)

    fmt.Printf("RPC HTTP server stopped, err: %s\n", err)

	return err
}

// WriteRPCResponseHTTP marshals res as JSON (with indent) and writes it to w.
func WriteRPCResponseHTTP(w http.ResponseWriter, res ...types.RPCResponse) error {
    var v interface{}
	if len(res) == 1 {
		v = res[0]
	} else {
		v = res
	}

    jsonBytes, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, err = w.Write(jsonBytes)
	return err
}
