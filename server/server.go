package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	
	"server/local/go-vhost"
	"server/local/session"
	"golang.org/x/time/rate"
)

var connectionLimits = map[string]int{
	"free":     1,
	"moderate": 50,
	"high":     100,
}

type ClientConnection struct {
	limiter *rate.Limiter
	active  int
}

var activeConnections = struct {
	sync.RWMutex
	connections map[string]*ClientConnection
}{connections: make(map[string]*ClientConnection)}

func main() {
	var port = flag.String("p", "9999", "server port to use")
	var host = flag.String("h", "teleport.me", "server hostname to use")
	var addr = flag.String("b", "0.0.0.0", "ip to bind [server only]")
	flag.Parse()

	l, err := net.Listen("tcp", net.JoinHostPort(*addr, *port))
	fatal(err)
	defer l.Close()

	vmux, err := vhost.NewHTTPMuxer(l, 1*time.Second)
	fatal(err)

	go serve(vmux, *host, *port)

	log.Printf("TelePort server [%s] ready!\n", *host)
	for {
		conn, err := vmux.NextError()
		if err != nil {
			fmt.Println(err)
		}
		if conn != nil {
			conn.Close()
		}
	}
}

func serve(vmux *vhost.HTTPMuxer, host, port string) {
	ml, err := vmux.Listen(net.JoinHostPort(host, port))
	fatal(err)

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subscription := r.Header.Get("X-Subscription-Level")
		log.Println("Received subscription header:", subscription)
		if subscription == "" {
			subscription = "free"
		}

		// Create or get the rate limiter and connection count for this host
		activeConnections.Lock()
		if _, exists := activeConnections.connections[r.RemoteAddr]; !exists {
			activeConnections.connections[r.RemoteAddr] = &ClientConnection{
				limiter: rate.NewLimiter(rate.Limit(connectionLimits[subscription]), connectionLimits[subscription]),
				active:  0,
			}
		}
		clientConn := activeConnections.connections[r.RemoteAddr]
		activeConnections.Unlock()

		publicHost := strings.TrimSuffix(net.JoinHostPort(newSubdomain()+host, port), ":80")
		pl, err := vmux.Listen(publicHost)
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			log.Println("Error creating listener:", err)
			return
		}
		defer pl.Close() // Ensure listener is closed

		w.Header().Add("X-Public-Host", publicHost)
		w.Header().Add("Connection", "close")
		w.WriteHeader(http.StatusOK)
		conn, _, _ := w.(http.Hijacker).Hijack()
		sess := session.New(conn)
		defer sess.Close()
		log.Printf("%s: start session", publicHost)

		go handleConnections(sess, pl, subscription, publicHost, clientConn)
		sess.Wait()
		log.Printf("%s: end session", publicHost)

		activeConnections.Lock()
		delete(activeConnections.connections, r.RemoteAddr)
		activeConnections.Unlock()
	})}
	srv.Serve(ml)
}

func handleConnections(sess *session.Session, pl net.Listener, subscription, publicHost string, clientConn *ClientConnection) {
	var wg sync.WaitGroup

	log.Println("Handling connections for:", publicHost, "with subscription:", subscription)
	for {
		activeConnections.Lock()
		if clientConn.active >= connectionLimits[subscription] {
			log.Println("Connection limit reached for subscription level:", subscription)
			activeConnections.Unlock()
			break
		}
		log.Println("Current active connections for", publicHost, ":", clientConn.active)
		clientConn.active++
		activeConnections.Unlock()

		if err := clientConn.limiter.Wait(context.Background()); err != nil {
			log.Println("Rate limit exceeded:", err)
			break
		}

		conn, err := pl.Accept()
		if err != nil {
			log.Println("Listener accept error:", err)
			break
		}

		ch, err := sess.Open(context.Background())
		if err != nil {
			log.Println("Session open error:", err)
			conn.Close()
			break
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				activeConnections.Lock()
				clientConn.active--
				activeConnections.Unlock()
			}()
			join(ch, conn)
		}()
	}

	wg.Wait()
}

func join(a io.ReadWriteCloser, b io.ReadWriteCloser) {
	go io.Copy(b, a)
	io.Copy(a, b)
	a.Close()
	b.Close()
}

func newSubdomain() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	letters := []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	r := make([]rune, 10)
	for i := range r {
		r[i] = letters[int(b[i])*len(letters)/256]
	}
	return "khederTeleport_" + string(r) + "_Go_" + "."
}

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
