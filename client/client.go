package client

import (
	"errors"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
)

// Client structure
type Client struct {
	Key           string
	BindPort      string
	LocalAddress  string
	ServerAddress string

	serverConn   *net.Conn
	yamuxSession *yamux.Session
	closedConn   bool
}

// ListenAndServe teleport client
func ListenAndServe(c Client) error {
	var err error
START:
	serverConn, err := net.DialTimeout("tcp4", c.ServerAddress, 10*time.Second)
	if err != nil {
		log.Println(err)
		time.Sleep(5 * time.Second)
		goto START
	}
	c.serverConn = &serverConn
	if err := c.Run(); err != nil {
		return err
	}
	c.Stop()
	log.Println("Conexão encerrada, aguardando 5 segundos")
	time.Sleep(5 * time.Second)
	goto START
}
func (c *Client) Stop() {
	c.yamuxSession.Close()
	closeConn(c.serverConn)
}
func (c *Client) handShake() error {
	(*c.serverConn).Write([]byte(c.Key + ":" + c.BindPort))
	buf := make([]byte, 1)
	n, err := (*c.serverConn).Read(buf)
	if err != nil {
		log.Println(err)
		return err
	}
	status := string(buf[:n])

	if status == "2" {
		return errors.New("connection refused, invalid port")
	}
	if status != "1" {
		return errors.New("connection refused")
	}
	return nil
}
func (c *Client) Run() error {
	var err error
	defer closeConn(c.serverConn)

	if err := c.handShake(); err != nil {
		return err
	}

	c.yamuxSession, err = yamux.Client(*c.serverConn, nil)
	if err != nil {
		log.Println(err)
		return err
	}
	defer c.yamuxSession.Close()
	go c.handleMuxSession()
	<-c.yamuxSession.CloseChan()
	return err
}
func closeConn(conn *net.Conn) {
	(*conn).SetDeadline(time.Now())
	(*conn).Close()
}
func (c *Client) handleMuxSession() {
	// Tenta conectar uma vez para verificar se a conexão existe e fecha
	firstTargetConn, err := c.createTargetConn()
	if err != nil {
		return
	}
	closeConn(firstTargetConn)
	for {
		if c.closedConn {
			c.yamuxSession.Close()
			return
		}
		yamuxClientConn, err := c.yamuxSession.Accept()
		if err != nil {
			// if errors.Is(err, io.EOF) {

			// }
			return
		}
		go c.tcpForwardToTarget(&yamuxClientConn)
	}
}
func (c *Client) tcpForwardToTarget(yamuxClientConn *net.Conn) {
	defer closeConn(yamuxClientConn)
	targetConn, err := c.createTargetConn()
	if err != nil {
		c.closedConn = true
		return
	}

	defer closeConn(targetConn)
	err = passthrough(yamuxClientConn, targetConn)
	//log.Println("passthrough finalizado")
	if err != nil {
		log.Printf("%s\n", err)
		return
	}
}
func (c *Client) createTargetConn() (*net.Conn, error) {
	//log.Println("createTargetConn")
	targetConn, err := net.DialTimeout("tcp4", c.LocalAddress, 2*time.Second)
	//targetConn, err := net.Dial("tcp4", c.localAddress)
	if err != nil {
		if targetConn != nil {
			closeConn(&targetConn)
		}
		// connectex: No connection could be made because the target machine actively refused it.
		// A porta local não está respondendo, feche a conexão com o servidor e tente em 5 segundos
		if strings.Contains(err.Error(), "No connection could be made") {
			c.yamuxSession.Close()
			return nil, err
		}
		// A porta local não está respondendo, feche a conexão com o servidor e tente em 5 segundos
		if errors.Is(err, net.ErrClosed) {
			c.yamuxSession.Close()
			return nil, err
		}
		if os.IsTimeout(err) {
			c.yamuxSession.Close()
			return nil, err
		}

		log.Println(err)
		return &targetConn, err
	}
	return &targetConn, nil
}
func passthrough(a, b *net.Conn) error {
	//errCh := make(chan error)
	d := sync.WaitGroup{}
	d.Add(2)
	copy := func(dest, src *net.Conn) {
		//_, err := io.Copy(dest, src)
		defer d.Done()
		io.Copy(*dest, *src)
		//errCh <- err
	}
	go copy(a, b)
	go copy(b, a)

	d.Wait()

	// Aguarde até que um seja concluído.
	//return <-errCh
	return nil
}
