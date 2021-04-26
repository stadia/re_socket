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
	// DialTimeOut specifies the duration for the dial to complete,
	// default to 2 seconds
	DialTimeOut time.Duration
	// KeepAliveTimeout is an interval for sending ping/pong messages
	// disabled if 0
	KeepAliveTimeout time.Duration
	// NonVerbose suppress connecting/reconnecting messages.
	NonVerbose bool

	isConnected bool
	mu          sync.RWMutex
	ipAddress	string
	dialErr     error
	dialer *net.Dialer

	conn net.Conn
}

// CloseAndReconnect will try to reconnect.
func (rc *ReSocket) CloseAndReconnect() {
	rc.Close()
	go rc.connect()
}

// setIsConnected sets state for isConnected
func (rc *ReSocket) setIsConnected(state bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.isConnected = state
}

func (rc *ReSocket) getConn() *net.Conn {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return &rc.conn
}

// Close closes the underlying network connection without
// sending or waiting for a close frame.
func (rc *ReSocket) Close() {
	if rc.getConn() != nil {
		rc.mu.Lock()
		rc.conn.Close()
		rc.mu.Unlock()
	}

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

func (rc *ReSocket) setDefaultDialer(dialTimeout time.Duration) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.dialer = &net.Dialer{
		Timeout: dialTimeout,
	}
}

func (rc *ReSocket) getDialTimeout() time.Duration {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.DialTimeOut
}

// Dial creates a new client connection.
func (rc *ReSocket) Dial(ipAddress string) {
	// Config
	rc.setIPAddress(ipAddress)
	rc.setDefaultRecIntvlMin()
	rc.setDefaultRecIntvlMax()
	rc.setDefaultRecIntvlFactor()
	rc.setDefaultDialTimeOut()
	rc.setDefaultDialer(rc.getDialTimeout())

	// Connect
	go rc.connect()

	// wait on first attempt
	time.Sleep(rc.getDialTimeout())
}

// GetIPAddress returns current connection IPAddress
func (rc *ReSocket) GetIPAddress() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.ipAddress
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
		nextItvl := b.Duration()
		conn, err := rc.dialer.Dial("tcp", rc.ipAddress)

		rc.mu.Lock()
		rc.conn = conn
		rc.dialErr = err
		rc.isConnected = err == nil
		rc.mu.Unlock()

		if err == nil {
			if !rc.getNonVerbose() {
				log.Printf("Dial: connection was successfully established with %s\n", rc.ipAddress)
			}

			if rc.getKeepAliveTimeout() != 0 {
				rc.conn.(*net.TCPConn).SetKeepAlive(true)
				rc.conn.(*net.TCPConn).SetKeepAlivePeriod(rc.getKeepAliveTimeout())
			}

			return
		}

		if !rc.getNonVerbose() {
			log.Println(err)
			log.Println("Dial: will try again in", nextItvl, "seconds.")
		}

		time.Sleep(nextItvl)
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