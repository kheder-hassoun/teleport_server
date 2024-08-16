package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"

	"github.com/progrium/qmux/golang/session"
)

func main() {
	var port = flag.String("p", "9999", "server port to use")
	var host = flag.String("h", "teleport.me", "server hostname to use")
	var username = flag.String("u", "test1", "username for authentication")
	var password = flag.String("pwd", "123456", "password for authentication")
	var sharedPort = flag.String("sp", "8000", "shared port to use")
	maxConnections := 100
	flag.Parse()

	if *username == "" || *password == "" {
		log.Fatal("Username and password must be provided")
	}

	conn, err := net.Dial("tcp", net.JoinHostPort(*host, *port))
	fatal(err)
	client := httputil.NewClientConn(conn, bufio.NewReader(conn))
	req, err := http.NewRequest("GET", "/", nil)
	fatal(err)

	// Set the username and password in the header
	req.Header.Set("X-Username", *username)
	req.Header.Set("X-Password", *password)
	req.Host = net.JoinHostPort(*host, *port)
	log.Println("Sending request with username and password")
	client.Write(req)
	resp, _ := client.Read(req)
	fmt.Printf("port %s http available at:\n", *sharedPort)
	fmt.Printf("http://%s\n", resp.Header.Get("X-Public-Host"))
	c, _ := client.Hijack()
	sess := session.New(c)
	defer sess.Close()

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConnections) // semaphore to limit connections

	for {
		ch, err := sess.Accept()
		fatal(err)
		conn, err := net.Dial("tcp", "127.0.0.1:"+*sharedPort)
		fatal(err)
		sem <- struct{}{} // acquire a slot
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }() // release a slot
			join(conn, ch)
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

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
