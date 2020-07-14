package http

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

var bad struct {
	sync.Mutex
	val bool
}

type connWrapper struct {
	net.Conn
}

func (cw *connWrapper) Write(b []byte) (n int, err error) {
	bad.Lock()
	defer bad.Unlock()
	if bad.val {
		log.Printf("bad=false")
		bad.val = false
		return 0, errors.New("broken pipe")
	}
	return cw.Conn.Write(b)
}

var trace = &httptrace.ClientTrace{
	GotConn: func(info httptrace.GotConnInfo) {
		log.Printf("conn: %#v", info)
	},
	TLSHandshakeDone: func(cfg tls.ConnectionState, err error) {
		bad.Lock()
		defer bad.Unlock()
		log.Printf("bad=true")
		bad.val = true
	},
}

func dial(network, addr string) (net.Conn, error) {
	c, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return &connWrapper{c}, nil
}

func TestBadCaching(t *testing.T) {
	ts := &http.Transport{
		ForceAttemptHTTP2: true,
		Dial:              dial,
		DialContext: func(_ context.Context, network, addr string) (net.Conn, error) {
			return dial(network, addr)
		},
	}
	if err := http2.ConfigureTransport(ts); err != nil {
		t.Fatal(err)
	}

	cli := &http.Client{
		Transport: ts,
	}
	for {
		func() {
			req, err := http.NewRequest("GET", "https://container.googleapis.com", nil)
			if err != nil {
				log.Printf("err: %v\n", err)
				return
			}
			req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
			resp, err := cli.Do(req)
			if err != nil {
				log.Printf("err: %v\n", err)
				return
			}
			fmt.Printf("status: %v\n", resp.Status)
			defer resp.Body.Close()
		}()
		time.Sleep(1 * time.Second)
	}
}
