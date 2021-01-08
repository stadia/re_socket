// that will automatically reconnect if the connection is dropped.
package resc

import (
	"errors"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/jpillora/backoff"
)

// ErrNotConnected is returned when the application read/writes
// a message and the connection is closed
var ErrNotConnected = errors.New("socket: not connected")

// The ReSocket type represents a Reconnecting Socket connection.
type ReSocket struct {
	// RecIntvlMin specifies the initial reconnecting interval,
	// default to 2 seconds
	RecIntvlMin time.Duration
	// RecIntvlMax specifies the maximum reconnecting interval,
	// default to 30 seconds
	RecIntvlMax time.Duration
	// RecIntvlFactor specifies the rate of increase of the reconnection
	// interval, default to 1.5
	RecIntvlFactor float64
	// NonVerbose suppress connecting/reconnecting messages.
	NonVerbose bool
	// KeepAliveTimeout is an interval for sending ping/pong messages
	// disabled if 0
	KeepAliveTimeout time.Duration
	DialTimeOut time.Duration

	isConnected bool
	mu          sync.RWMutex
	ipAddress	string
	dialErr     error
	close       chan (bool)

	conn net.Conn
	connected chan (struct{})
}

// CloseAndReconnect will try to reconnect.
func (rc *ReSocket) CloseAndReconnect() {
	rc.Close()
	<-rc.close
	go rc.connect()
}

// setIsConnected sets state for isConnected
func (rc *ReSocket) setIsConnected(state bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.isConnected = state
}

// Close closes the underlying network connection without
// sending or waiting for a close frame.
func (rc *ReSocket) Close() {
	if rc.conn != nil {
		rc.mu.Lock()
		rc.conn.Close()
		rc.mu.Unlock()
	}
	rc.close <- true
	rc.setIsConnected(false)
}

// WriteMessage is a helper method for getting a writer using NextWriter,
// writing the message and closing the writer.
//
// If the connection is closed ErrNotConnected is returned
func (rc *ReSocket) WriteMessage(data []byte) error {
	err := ErrNotConnected
	if rc.IsConnected() {
		rc.mu.Lock()
		_, err = rc.conn.Write(data)
		rc.mu.Unlock()
		if err != nil {
			rc.CloseAndReconnect()
		}
	}

	return err
}

func (rc *ReSocket) setIPAddress(ipAddress string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.ipAddress = ipAddress
}

func (rc *ReSocket) setDefaultRecIntvlMin() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.RecIntvlMin == 0 {
		rc.RecIntvlMin = 2 * time.Second
	}
}

func (rc *ReSocket) setDefaultRecIntvlMax() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.RecIntvlMax == 0 {
		rc.RecIntvlMax = 30 * time.Second
	}
}

func (rc *ReSocket) setDefaultRecIntvlFactor() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.RecIntvlFactor == 0 {
		rc.RecIntvlFactor = 1.5
	}
}

func (rc *ReSocket) setDefaultDialTimeOut() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.DialTimeOut == 0 {
		rc.DialTimeOut = 15 * time.Second
	}
}

// Dial creates a new client connection.
func (rc *ReSocket) Dial(ipAddress string) {
	// Close channel
	rc.close = make(chan bool, 1)
	// Connected channel
	rc.connected = make(chan struct{})
	
	// Config
	rc.setIPAddress(ipAddress)
	rc.setDefaultRecIntvlMin()
	rc.setDefaultRecIntvlMax()
	rc.setDefaultRecIntvlFactor()
	rc.setDefaultDialTimeOut()

	// Connect
	go rc.connect()
}

// GetIPAddress returns current connection IPAddress
func (rc *ReSocket) GetIPAddress() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.ipAddress
}

// GetConn returns current connection connection
func (rc *ReSocket) GetConn() net.Conn {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.conn
}

func (rc *ReSocket) getNonVerbose() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.NonVerbose
}

func (rc *ReSocket) getBackoff() *backoff.Backoff {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return &backoff.Backoff{
		Min:    rc.RecIntvlMin,
		Max:    rc.RecIntvlMax,
		Factor: rc.RecIntvlFactor,
		Jitter: true,
	}
}

func (rc *ReSocket) getKeepAliveTimeout() time.Duration {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.KeepAliveTimeout
}

func (rc *ReSocket) connect() {
	b := rc.getBackoff()
	rand.Seed(time.Now().UTC().UnixNano())

	for {
		select {
		case <-rc.close:
			return
		default:
			nextItvl := b.Duration()
			conn, err := net.DialTimeout("tcp", rc.ipAddress, rc.DialTimeOut)

			rc.mu.Lock()
			rc.conn = conn
			rc.dialErr = err
			rc.isConnected = err == nil
			rc.mu.Unlock()

			if err == nil {
				conn.(*net.TCPConn).SetKeepAlive(true)
				if !rc.getNonVerbose() {
					log.Printf("Dial: connection was successfully established with %s\n", rc.ipAddress)
				}
				if rc.getKeepAliveTimeout() != 0 {
					rc.conn.(*net.TCPConn).SetKeepAlive(true)
					rc.conn.(*net.TCPConn).SetKeepAlivePeriod(rc.getKeepAliveTimeout())
				}
				rc.connected <- struct {
				}{}

				return
			}

			if !rc.getNonVerbose() {
				log.Println(err)
				log.Println("Dial: will try again in", nextItvl, "seconds.")
			}

			time.Sleep(nextItvl)
		}
	}
}

// GetDialError returns the last dialer error.
// nil on successful connection.
func (rc *ReSocket) GetDialError() error {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.dialErr
}

// IsConnected returns the WebSocket connection state
func (rc *ReSocket) IsConnected() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.isConnected
}

func (rc *ReSocket) Connected() <-chan struct{} {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.connected == nil {
		rc.connected = make(chan struct{})
	}

	return rc.connected
}

func (rc *ReSocket) Closed() <-chan bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.close == nil {
		rc.close = make(chan bool)
	}

	return rc.close
}