package utilities

import (
	"crypto/cipher"
	"crypto/rand"
	"io"
	"log"
	"time"
)

func GetCurrentTime() string {
	t := time.Now().UTC()
	return t.Format(time.RFC3339)
}
func JoinEncrypted(ch io.ReadWriteCloser, conn io.ReadWriteCloser, aesGCM cipher.AEAD) {
	buffer := make([]byte, 4096)

	toConn := make(chan []byte)
	fromConn := make(chan []byte)

	go func() {
		defer close(toConn)
		for {
			n, err := ch.Read(buffer)
			if err != nil {
				log.Println("the stupid channel read error:", err)
				return
			}

			nonce := make([]byte, aesGCM.NonceSize())
			if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
				log.Println("errorrr generating nonce:", err)
				return
			}
			//log.Printf("+++++++++ generated nonce for encryption: %x\n", nonce)

			encryptedData := aesGCM.Seal(nil, nonce, buffer[:n], nil)

			dataToSend := append(nonce, encryptedData...)
			toConn <- dataToSend
		}
	}()

	go func() {
		defer close(fromConn)
		for {
			n, err := conn.Read(buffer)
			if err != nil {
				log.Println("--------- connection read error:", err)
				return
			}

			nonceSize := aesGCM.NonceSize()
			if n < nonceSize {
				log.Println("--------- insufficient data for nonce")
				return
			}
			nonce := buffer[:nonceSize]
			encryptedData := buffer[nonceSize:n]

			decryptedData, err := aesGCM.Open(nil, nonce, encryptedData, nil)
			if err != nil {
				log.Println("--------- decryption error:", err)
				return
			}
			//log.Printf("+++++++++ decrypted data: %x\n", decryptedData)

			fromConn <- decryptedData
		}
	}()

	go func() {
		for data := range toConn {
			if _, err := conn.Write(data); err != nil {
				log.Println("--------- connection write error:", err)
				return
			}
		}
	}()

	for data := range fromConn {
		if _, err := ch.Write(data); err != nil {
			log.Println("--------- channel write error:", err)
			return
		}
	}

	ch.Close()
	conn.Close()
}

func NewSubdomain(userName string) string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	letters := []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	r := make([]rune, 10)
	for i := range r {
		r[i] = letters[int(b[i])%len(letters)]
	}
	return "Teleport_" + userName + "_" + string(r) + "."
}

func Fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
