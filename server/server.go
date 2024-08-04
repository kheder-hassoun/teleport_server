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
	"github.com/inconshreveable/go-vhost"
	"github.com/progrium/qmux/golang/session"
)

var connectionLimits = map[string]int{
	"free":     1,
	"moderate": 50,
	"high":     100,
}

var activeConnections = struct {
	sync.RWMutex
	connections map[string]int
}{connections: make(map[string]int)}

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
		fmt.Println(err)
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
		log.Println("supscription : ", subscription)
		if subscription == "" {
			subscription = "free"
		}

		publicHost := strings.TrimSuffix(net.JoinHostPort(newSubdomain()+host, port), ":80")
		pl, err := vmux.Listen(publicHost)
		fatal(err)
		w.Header().Add("X-Public-Host", publicHost)
		w.Header().Add("Connection", "close")
		w.WriteHeader(http.StatusOK)
		conn, _, _ := w.(http.Hijacker).Hijack()
		sess := session.New(conn)
		defer sess.Close()
		log.Printf("%s: start session", publicHost)

		go handleConnections(sess, pl, subscription, publicHost)
		sess.Wait()
		log.Printf("%s: end session", publicHost)
	})}
	srv.Serve(ml)
}

func handleConnections(sess *session.Session, pl net.Listener, subscription, publicHost string) {
	var wg sync.WaitGroup

	//log.Println("handelConnection function :  subscripation type : " , subscripation)
	for {

		activeConnections.Lock()
		if activeConnections.connections[publicHost] >= connectionLimits[subscription] {
			log.Println("activeConnections.connections[publicHost] >= connectionLimits[subscription]")
			activeConnections.Unlock()
			break
		}
		log.Println("activeConnections.connections[publicHost] : ",activeConnections.connections[publicHost])
		activeConnections.connections[publicHost]++
		activeConnections.Unlock()

		conn, err := pl.Accept()
		if err != nil {
			log.Println(err)
			break
		}

		ch, err := sess.Open(context.Background())
		if err != nil {
			log.Println(err)
			break
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				activeConnections.Lock()
				activeConnections.connections[publicHost]--
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
