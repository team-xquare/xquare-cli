// Package tunnel implements a TCP-over-WebSocket client compatible with wstunnel v3.
//
// Protocol: each TCP connection accepted on localPort opens a WebSocket to:
//
//	wss://{tunnelHost}/{password}/v1/tcp/{targetService}/{targetPort}
//
// The WebSocket carries raw binary frames both ways.
package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// Tunnel listens on a local TCP port and forwards connections over WebSocket.
type Tunnel struct {
	TunnelHost    string // e.g. xquare-remote-access-dms.dsmhs.kr
	Password      string // HTTP upgrade path prefix = auth token
	TargetService string // e.g. main-prod-mysql
	TargetPort    int    // e.g. 3306
	LocalPort     int    // e.g. 3306
}

// Start listens for TCP connections and proxies them. Blocks until ctx is cancelled.
func (t *Tunnel) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", t.LocalPort))
	if err != nil {
		return fmt.Errorf("listen on :%d: %w", t.LocalPort, err)
	}
	defer listener.Close()

	// Close listener when ctx is done
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("accept: %w", err)
			}
		}
		go t.handle(ctx, conn)
	}
}

func (t *Tunnel) handle(ctx context.Context, local net.Conn) {
	defer local.Close()

	wsURL := fmt.Sprintf("wss://%s/%s/v1/tcp/%s/%d",
		t.TunnelHost, t.Password, t.TargetService, t.TargetPort)

	dialer := websocket.Dialer{
		Subprotocols: []string{"binary"},
	}
	wsConn, _, err := dialer.DialContext(ctx, wsURL, http.Header{})
	if err != nil {
		return
	}
	defer wsConn.Close()

	// Pipe local TCP ↔ WebSocket (binary frames)
	var wg sync.WaitGroup
	wg.Add(2)

	// local → ws
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := local.Read(buf)
			if n > 0 {
				if writeErr := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// ws → local
	go func() {
		defer wg.Done()
		for {
			_, msg, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			if _, err := local.Write(msg); err != nil {
				return
			}
		}
	}()

	wg.Wait()
}

// Pipe copies between two ReadWriters until one closes.
func pipe(a, b io.ReadWriter) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(a, b) }()
	go func() { defer wg.Done(); io.Copy(b, a) }()
	wg.Wait()
}
