package main

import (
	"context"
	"github.com/stadia/re_socket"
	"time"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	socket := re_socket.ReSocket{
	}
	socket.Dial("wss://echo.websocket.org")

	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			go socket.Close()
			return
		default:
			if !socket.IsConnected() {
				continue
			}

			if err := socket.WriteMessage([]byte("Incoming")); err != nil {
				return
			}
		}
	}
}
