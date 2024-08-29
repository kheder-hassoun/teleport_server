package main

///   ***************************************  I hate Go lang ***************************************
///** Bring your blood pressure, diabetes, and any other medications before reading the code ☠ ⚠ **

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"teleportServer/auth"
	"teleportServer/local/go-vhost"
	"teleportServer/local/session"
	"teleportServer/utilities"
	"time"

	"golang.org/x/time/rate"
)

///   *************************************** conigrations ***************************************

type Config struct {
	Port          string `json:"port"`
	Host          string `json:"host"`
	Addr          string `json:"addr"`
	ApiUrlAuth    string `json:"apiUrlAuth"`
	ApiUrlDetails string `json:"apiUrlDetails"`
	Token         string `json:"token"`
	Free          int    `json:"free"`
	Moderate      int    `json:"moderate"`
	High          int    `json:"high"`
}

var config Config

var connectionLimits = map[string]int{}

type ClientConnection struct {
	limiter *rate.Limiter
	active  int
}

var activeConnections = struct {
	sync.RWMutex
	connections map[string]*ClientConnection
}{connections: make(map[string]*ClientConnection)}

///   *************************************** main  ***************************************

func main() {

	file, err := os.Open("config.json")
	if err != nil {
		log.Fatalf("--------- error opening config file: %v", err)
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&config)
	if err != nil {
		log.Fatalf("--------- error decoding config file: %v", err)
	}

	connectionLimits["free"] = config.Free
	connectionLimits["moderate"] = config.Moderate
	connectionLimits["high"] = config.High

	port := config.Port
	host := config.Host
	addr := config.Addr

	l, err := net.Listen("tcp", net.JoinHostPort(addr, port))
	utilities.Fatal(err)
	defer l.Close()

	vmux, err := vhost.NewHTTPMuxer(l, 3*time.Second)
	utilities.Fatal(err)

	go ConnectionManager(vmux, host, port)

	log.Printf("TelePort server [%s] ready!\n", host)
	for {
		conn, err := vmux.NextError()
		if err != nil {
			log.Println(err)
		}
		if conn != nil {
			conn.Close()
		}
	}
}

///   *************************************** ConnectionManager  ***************************************

func ConnectionManager(vmux *vhost.HTTPMuxer, host, port string) {
	myStupidListner, err := vmux.Listen(net.JoinHostPort(host, port))
	utilities.Fatal(err)

	srv := &http.Server{Handler: http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		username := request.Header.Get("X-Username")
		password := request.Header.Get("X-Password")

		if username == "" || password == "" {
			http.Error(responseWriter, "Username and password required", http.StatusUnauthorized)
			return
		}

		apiUrl := config.ApiUrlAuth
		signInData := auth.SignInDto{
			UserName: username,
			Password: password,
		}

		subscription, err := auth.SignInAndGetSubscriptionType(apiUrl, signInData)
		if err != nil {
			http.Error(responseWriter, "--------- Authentication failed", http.StatusUnauthorized)
			return
		}

		activeConnections.Lock()
		if _, exists := activeConnections.connections[request.RemoteAddr]; !exists {
			activeConnections.connections[request.RemoteAddr] = &ClientConnection{
				limiter: rate.NewLimiter(rate.Limit(connectionLimits[subscription]), connectionLimits[subscription]),
				active:  0,
			}
		}
		clientConn := activeConnections.connections[request.RemoteAddr]
		activeConnections.Unlock()

		publicHost := strings.TrimSuffix(net.JoinHostPort(utilities.NewSubdomain(username)+host, port), ":80")
		pl, err := vmux.Listen(publicHost)
		if err != nil {
			http.Error(responseWriter, "--------- server error", http.StatusInternalServerError)
			log.Println("--------- error creating listener:", err)
			return
		}
		defer pl.Close()

		apiUrladd := config.ApiUrlDetails
		token := config.Token
		userName := fmt.Sprintf("%v", username)
		url := fmt.Sprintf("%v", publicHost)
		timetemp := utilities.GetCurrentTime()

		err = auth.AddUserUrlDetails(apiUrladd, token, userName, url, timetemp)
		if err != nil {
			log.Fatalf("--------- error adding user URL details: %v", err)
		}

		curve := elliptic.P256()
		privKey, x, y, err := elliptic.GenerateKey(curve, rand.Reader)
		if err != nil {
			log.Fatal("--------- failed to generate DH key: I hate gooooooooooooo ", err)
		}
		pubKey := elliptic.Marshal(curve, x, y)

		responseWriter.Header().Set("X-Server-Public-Key", fmt.Sprintf("%x", pubKey))
		responseWriter.Header().Set("X-Public-Host", publicHost)
		responseWriter.Header().Set("Connection", "close")
		responseWriter.WriteHeader(http.StatusOK)

		clientPubKeyHex := request.Header.Get("X-Client-Public-Key")
		if clientPubKeyHex == "" {
			http.Error(responseWriter, "stupid client public key required", http.StatusBadRequest)
			return
		}
		clientPubKey, err := hex.DecodeString(clientPubKeyHex)
		if err != nil {
			http.Error(responseWriter, "invalid  stupid client public key", http.StatusBadRequest)
			return
		}

		clientX, clientY := elliptic.Unmarshal(curve, clientPubKey)
		if clientX == nil || clientY == nil {
			http.Error(responseWriter, "invalid stupid client public key", http.StatusBadRequest)
			return
		}

		sharedX, _ := curve.ScalarMult(clientX, clientY, privKey)
		sharedSecret := sharedX.Bytes()

		aesKey := sha256.Sum256(sharedSecret)
		block, err := aes.NewCipher(aesKey[:])
		if err != nil {
			log.Fatal(err)
		}
		aesGCM, err := cipher.NewGCM(block)
		if err != nil {
			log.Fatal(err)
		}

		conn, _, err := responseWriter.(http.Hijacker).Hijack()
		if err != nil {
			log.Fatal("failed to hijack connection I hate gooo :", err)
		}
		sess := session.New(conn)
		defer sess.Close()
		log.Printf("%s: start session", publicHost)

		conn.SetDeadline(time.Now().Add(60 * time.Minute))

		go handleConnections(sess, pl, subscription, publicHost, userName, token, clientConn, aesGCM)

		sess.Wait()
		log.Printf("%s: end session", publicHost)

		activeConnections.Lock()
		delete(activeConnections.connections, request.RemoteAddr)
		activeConnections.Unlock()
	})}
	srv.Serve(myStupidListner)
}

///   *************************************** handleConnections  ***************************************

func handleConnections(sess *session.Session, pl net.Listener, subscription, publicHost, userName, token string, clientConn *ClientConnection, aesGCM cipher.AEAD) {
	var wg sync.WaitGroup

	log.Println("Handling connections for:", publicHost, "with subscription:", subscription)

	for {
		activeConnections.Lock()
		if clientConn.active >= connectionLimits[subscription] {
			log.Println("Connection limit reached for subscription level:", subscription)
			activeConnections.Unlock()
			break
		}
		clientConn.active++
		activeConnections.Unlock()

		err2 := auth.SendIncrementRequest(userName, publicHost, config.ApiUrlDetails, token)
		if err2 != nil {
			log.Fatalf("--------- error adding user URL details: %v", err2)
		}

		if err := clientConn.limiter.Wait(context.Background()); err != nil {
			log.Println("Rate limit exceeded:", err)
			activeConnections.Lock()
			clientConn.active--
			activeConnections.Unlock()
			break
		}

		conn, err := pl.Accept()
		if err != nil {
			log.Println("---------- listener accept error:", err)
			activeConnections.Lock()
			clientConn.active--
			activeConnections.Unlock()
			break
		}

		ch, err := sess.Open(context.Background())
		if err != nil {
			log.Println("----------- session open error:", err)
			conn.Close()
			activeConnections.Lock()
			clientConn.active--
			activeConnections.Unlock()
			break
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				activeConnections.Lock()
				clientConn.active--
				log.Println("******** Decrement active connections", clientConn)
				activeConnections.Unlock()
			}()

			utilities.JoinEncrypted(ch, conn, aesGCM)
		}()
	}

	wg.Wait()
}

///   ***************************************  I hate Go lang again ***************************************
