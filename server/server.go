package server

import (
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
)

var key string

func numericPort(port string) int {
	p, _ := strconv.Atoi(port)
	if p >= 6000 && p <= 10000 {
		return p
	}
	return 0
}

// Start teleport server
func Start(k, address string) {
	key = k
	server, err := net.Listen("tcp4", address)
	if err != nil {
		log.Println(err)
		return
	}
	for {
		conn, err := server.Accept()
		if err != nil {
			log.Println(err)
			return
		}
		go runTcp(&conn)
	}
}

var listenPool map[int]*net.TCPListener = make(map[int]*net.TCPListener)

func handShake(conn *net.Conn) (int, error) {
	buf := make([]byte, 128)
	n, err := (*conn).Read(buf)
	if err != nil {
		log.Println(err)
		return 0, err
	}
	hash, port, err := net.SplitHostPort(string(buf[:n]))
	if err != nil {
		log.Println(err)
	}
	if hash != key {
		(*conn).Write([]byte("0"))
		closeConn(conn)
		return 0, errors.New("connection refused")
	}

	portNumber := numericPort(port)
	if 0 >= portNumber {
		(*conn).Write([]byte("2"))
		closeConn(conn)
		return 0, errors.New("connection refused, wrong port")
	}
	(*conn).Write([]byte("1"))
	return portNumber, nil
}
func runTcp(conn *net.Conn) {
	portNumber, err := handShake(conn)
	if err != nil {
		log.Println(err)
		return
	}
	// listener, err := net.Listen("tcp4", ":0")

	// port := "6000"
	// portNumber := numericPort(port)

	addr, err := net.ResolveTCPAddr("tcp4", "0.0.0.0:"+strconv.Itoa(portNumber))
	if err != nil {
		log.Println(err)
		return
	}

	if listenPool[portNumber] == nil {
		c, err := net.ListenTCP("tcp4", addr)
		// if listener != nil {
		// 	defer listener.Close()
		// }
		if err != nil {
			log.Print("net.Listen:", err)
			//w.WriteHeader(http.StatusInternalServerError)
			return
		}
		listenPool[portNumber] = c
	}

	log.Println(listenPool[portNumber].Addr())

	session, err := yamux.Server(*conn, nil)
	if err != nil {
		log.Print("yamux.Server:", err)
		return
	}
	defer session.Close()
	//(*conn).Close()
	go forwardFromNetListenerToYamuxSession(listenPool[portNumber], session)
	<-session.CloseChan()
	// session.Close()
	if listenPool[portNumber] != nil {
		listenPool[portNumber].Close()
	}
	listenPool[portNumber] = nil
	log.Print("yamux.Session closed")
}

func forwardFromNetListenerToYamuxSession(listener *net.TCPListener, session *yamux.Session) {
	for {

		clientConn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				// Considerado como terminação normal
				(*listener).Close()
				session.Close()
				return
			}
			log.Printf("error Accept: %s", err)
			return
		}
		go pipeSession(listener, &clientConn, session)
	}
}
func pipeSession(listener *net.TCPListener, clientConn *net.Conn, session *yamux.Session) {
	defer closeConn(clientConn)
	serverConn, err := session.Open()
	if err != nil {
		if strings.Contains(err.Error(), "session shutdown") {
			listener.Close()
		}
		session.Close()
		log.Printf("error session.Open: %s", err)
		return
	}
	defer closeConn(&serverConn)
	err = passthrough(*clientConn, serverConn)
	//log.Println("passthrough finalizado")
	if err != nil {
		return
	}

}

func passthrough(a, b net.Conn) error {
	//errCh := make(chan error)
	d := sync.WaitGroup{}
	d.Add(2)
	copy := func(dest, src net.Conn) {
		//_, err := io.Copy(dest, src)
		defer d.Done()
		io.Copy(dest, src)
		//errCh <- err
	}
	go copy(a, b)
	go copy(b, a)

	d.Wait()

	// Aguarde até que um seja concluído.
	//return <-errCh
	return nil
}

func closeConn(conn *net.Conn) {
	(*conn).SetDeadline(time.Now())
	(*conn).Close()
}
