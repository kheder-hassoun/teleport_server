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

	"teleportServer/auth"
	"teleportServer/local/go-vhost"
	"teleportServer/local/session"
	"time"

	"golang.org/x/time/rate"
)

// Define connection limits based on subscription levels
var connectionLimits = map[string]int{
	"free":     2,
	"moderate": 50,
	"high":     100,
}

// ClientConnection holds the rate limiter and active connection count for each client
type ClientConnection struct {
	limiter *rate.Limiter
	active  int
}

// activeConnections keeps track of all client connections
var activeConnections = struct {
	sync.RWMutex
	connections map[string]*ClientConnection
}{connections: make(map[string]*ClientConnection)}

func main() {
	// Parse command-line flags for server configuration
	var port = flag.String("p", "9999", "server port to use")
	var host = flag.String("h", "teleport.me", "server hostname to use")
	var addr = flag.String("b", "0.0.0.0", "ip to bind [server only]")
	flag.Parse()

	// Start listening on the specified address and port
	l, err := net.Listen("tcp", net.JoinHostPort(*addr, *port))
	fatal(err)
	defer l.Close()

	// Create a new HTTP muxer with a timeout
	vmux, err := vhost.NewHTTPMuxer(l, 3*time.Second)
	fatal(err)

	// Start the HTTP server in a separate goroutine
	go serve(vmux, *host, *port)

	log.Printf("TelePort server [%s] ready!\n", *host)
	for {
		// Handle incoming connections and log errors
		conn, err := vmux.NextError()
		if err != nil {
			fmt.Println(err)
		}
		if conn != nil {
			conn.Close()
		}
	}
}

// serve initializes the HTTP server to handle requests
func serve(vmux *vhost.HTTPMuxer, host, port string) {
	ml, err := vmux.Listen(net.JoinHostPort(host, port))
	fatal(err)

	// Define the HTTP handler function
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := r.Header.Get("X-Username")
		password := r.Header.Get("X-Password")

		if username == "" || password == "" {
			http.Error(w, "Username and password required", http.StatusUnauthorized)
			return
		}

		apiUrl := "http://192.168.184.1:9090/api/v1/auth"
		signInData := auth.SignInDto{
			UserName: username,
			Password: password,
		}

		subscription, err := auth.SignInAndGetSubscriptionType(apiUrl, signInData)
		if err != nil {
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			return
		}

		// Create or retrieve the rate limiter and connection count for this client
		activeConnections.Lock()
		if _, exists := activeConnections.connections[r.RemoteAddr]; !exists {
			activeConnections.connections[r.RemoteAddr] = &ClientConnection{
				limiter: rate.NewLimiter(rate.Limit(connectionLimits[subscription]), connectionLimits[subscription]),
				active:  0,
			}
		}
		clientConn := activeConnections.connections[r.RemoteAddr]
		activeConnections.Unlock()

		// Generate a new subdomain for the client
		publicHost := strings.TrimSuffix(net.JoinHostPort(newSubdomain()+host, port), ":80")
		pl, err := vmux.Listen(publicHost)
		if err != nil {
			http.Error(w, "Server error", http.StatusInternalServerError)
			log.Println("Error creating listener:", err)
			return
		}
		defer pl.Close() // Ensure listener is closed

		// Send the public host back to the client in the response headers
		w.Header().Add("X-Public-Host", publicHost)
		w.Header().Set("Connection", "close") // Force the connection to close after response
		w.WriteHeader(http.StatusOK)

		// Hijack the connection to take control of the TCP connection
		conn, _, _ := w.(http.Hijacker).Hijack()
		sess := session.New(conn)
		defer sess.Close()
		log.Printf("%s: start session", publicHost)

		// Set a timeout for the connection
		conn.SetDeadline(time.Now().Add(5 * time.Minute)) // 5-minute timeout

		// Handle connections in a separate goroutine
		go handleConnections(sess, pl, subscription, publicHost, clientConn)
		sess.Wait() // Wait for the session to finish
		log.Printf("%s: end session", publicHost)

		// Clean up connection state
		activeConnections.Lock()
		delete(activeConnections.connections, r.RemoteAddr)
		activeConnections.Unlock()
	})}
	// Start serving requests
	srv.Serve(ml)
}

// handleConnections manages each client's session and connections
// handleConnections manages each client's session and connections
func handleConnections(sess *session.Session, pl net.Listener, subscription, publicHost string, clientConn *ClientConnection) {
	var wg sync.WaitGroup

	log.Println("Handling connections for:", publicHost, "with subscription:", subscription)

	for {
		// Lock and check the connection limit
		activeConnections.Lock()
		if clientConn.active >= connectionLimits[subscription] {
			log.Println("Connection limit reached for subscription level:", subscription)
			activeConnections.Unlock()
			break
		}
		clientConn.active++ // Increment active connections count
		activeConnections.Unlock()

		// Wait for rate limit to be available
		if err := clientConn.limiter.Wait(context.Background()); err != nil {
			log.Println("Rate limit exceeded:", err)
			// Decrement active connections if rate limit exceeded
			activeConnections.Lock()
			clientConn.active--
			activeConnections.Unlock()
			break
		}

		// Accept a new connection
		conn, err := pl.Accept()
		if err != nil {
			log.Println("Listener accept error:", err)
			// Decrement active connections on accept error
			activeConnections.Lock()
			clientConn.active--
			activeConnections.Unlock()
			break
		}

		// Open a new session
		ch, err := sess.Open(context.Background())
		if err != nil {
			log.Println("Session open error:", err)
			conn.Close()
			// Decrement active connections on session open error
			activeConnections.Lock()
			clientConn.active--
			activeConnections.Unlock()
			break
		}

		// Handle the connection in a separate goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				// Decrement active connections count after handling the connection
				activeConnections.Lock()
				clientConn.active--
				log.Println("******** Decrement active connections", clientConn)
				activeConnections.Unlock()
			}()
			join(ch, conn)
		}()
	}

	wg.Wait()
}

// join connects two io.ReadWriteClosers by copying data between them
func join(a io.ReadWriteCloser, b io.ReadWriteCloser) {
	go io.Copy(b, a)
	io.Copy(a, b)
	a.Close()
	b.Close()
}

// newSubdomain generates a new subdomain for the client
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

// fatal logs a fatal error and terminates the program
func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
