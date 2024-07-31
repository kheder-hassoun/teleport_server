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
	"time"

	vhost "github.com/inconshreveable/go-vhost" //? for handeling virtual host  actually in http request
	"github.com/progrium/qmux/golang/session"
)

func main() {
	var port = flag.String("p", "9999", "server port to use") //* here we define a command line flage name it p it's specific the server port and defult value is 9999
	//var host = flag.String("h", "vcap.me", "server hostname to use") //* -h specifying the server hostname defult is vcap.me // vcap.me it's a public domain that resolves to 127.0.0.1.
	var host = flag.String("h", "teleport.me", "server hostname to use")
	var addr = flag.String("b", "0.0.0.0", "ip to bind [server only]")
	flag.Parse() //? now we can get value by name ('port','host','addr')

	//---------------------------------------

	l, err := net.Listen("tcp", net.JoinHostPort(*addr, *port)) //*sets up a TCP listener on the specified address and port. l is the listner object
	fatal(err)
	defer l.Close() //* close the listner after return ✔
	//? Creates a new HTTP multiplexer to handle virtual host connections.
	vmux, err := vhost.NewHTTPMuxer(l, 1*time.Second)
	fatal(err)

	go serve(vmux, *host, *port) //Starts the serve function in a new goroutine to handle incoming connections.

	log.Printf("TelePort server [%s] ready!\n", *host)
	//? Continuously handles incoming connections and errors from the HTTP multiplexer.
	for {
		conn, err := vmux.NextError()
		fmt.Println(err)
		if conn != nil {
			conn.Close()
		}
	}
}

func serve(vmux *vhost.HTTPMuxer, host, port string) {
	ml, err := vmux.Listen(net.JoinHostPort(host, port)) //Starts listening for HTTP connections on the specified host and port.
	fatal(err)

	// Creates a new HTTP server with a specific request handler.
	// w: The http.ResponseWriter used to send responses.
	//r: The http.Request representing the client's request.
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//Generates a new subdomain and creates a public host string
		publicHost := strings.TrimSuffix(net.JoinHostPort(newSubdomain()+host, port), ":80")
		pl, err := vmux.Listen(publicHost) //Sets up a new listener for the generated public host.
		fatal(err)
		w.Header().Add("X-Public-Host", publicHost)
		w.Header().Add("Connection", "close")
		w.WriteHeader(http.StatusOK)
		//? Hijacks the HTTP connection, taking control of the underlying TCP connection.
		conn, _, _ := w.(http.Hijacker).Hijack()
		sess := session.New(conn)
		defer sess.Close()
		log.Printf("%s: start session", publicHost)
		//? Handles incoming connections in a separate goroutine.
		go func() {
			for {
				conn, err := pl.Accept()
				if err != nil {
					log.Println(err)
					return
				}
				ch, err := sess.Open(context.Background())
				if err != nil {
					log.Println(err)
					return
				}
				go join(ch, conn)
			}
		}()
		sess.Wait()
		log.Printf("%s: end session", publicHost)
	})}
	srv.Serve(ml)
}

// * This function is crucial for setting up a tunnel where data can flow in both directions
// * between two connections, enabling seamless communication between them.
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

