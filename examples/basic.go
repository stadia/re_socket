package main

import (
	"context"
	"github.com/stadia/re_socket"
	"log"
	"time"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	socket := resc.ReSocket{
		KeepAliveTimeout: 10 * time.Second,
	}
	socket.Dial("127.0.0.1:9090")

	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			go socket.Close()
			log.Printf("Websocket closed %s", socket.GetIPAddress())
			return
		default:
			if !socket.IsConnected() {
				log.Printf("Websocket disconnected %s", socket.GetIPAddress())
				continue
			}

			if err := socket.WriteMessage([]byte("Incoming")); err != nil {
				log.Printf("Error: WriteMessage %s", socket.GetIPAddress())
				return
			}
		}
	}
}