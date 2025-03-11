package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
)

type Client struct {
	conn    net.Conn
	msgChan chan []byte
}

type Msg struct {
	client *Client
	msg    []byte
}

var (
	clients    = make(map[*Client]bool)
	clientToID = make(map[*Client]uint32)
	lastID     = uint32(0)
	messages   = make(chan Msg)
	clientsMu  sync.Mutex
	prod       bool = false
)

func main() {
	log := log.Default()
	log.Println("Hello, world!")
	ln, err := net.Listen("tcp", ":927")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	go broadcaster()
	wm := append([]byte{0, 0, 0, 0, byte(EventPing)}, []byte("Welcome To The Beginning Of The Rest Of Your Life")...)
	prependLength(&wm)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetNoDelay(true)
		go greet(conn, wm)
	}
}

func greet(conn net.Conn, wm []byte) {
	conn.Write(wm)
	client := &Client{conn: conn, msgChan: make(chan []byte, 100)}
	clientsMu.Lock()
	clients[client] = true
	clientsMu.Unlock()

	go clientWriter(client)

	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			break
		}
		m := make([]byte, n)
		copy(m, buf)
		fmt.Printf("read %x\n", m)
		err = parseMessages(m, client)
		if err != nil {
			break
		}
	}
	clientsMu.Lock()
	delete(clients, client)
	clientsMu.Unlock()
	conn.Close()
}

func parseMessages(buf []byte, client *Client) error {
	for {
		ml := int(buf[0])
		fmt.Printf("ml = %x\n", buf[0])
		if ml == 0 {
			return errors.New("message length 0")
		}
		msg := make([]byte, ml - 1)
		n := copy(msg, buf[1:])
		if n != ml - 1 {
			return errors.New("message longer than data")
		}
		fmt.Printf("parsed %x\n", msg)
		messages <- Msg{client, msg}
		if len(buf) - ml <= 0 {
			break
		}
		buf = buf[ml:]
	}
	return nil
}

func clientWriter(client *Client) {
	for msg := range client.msgChan {
		client.conn.Write(msg)
	}
}

func broadcaster() {
	for msg := range messages {
		if !prod {
			fmt.Printf("recieved %x from %x\n", msg.msg, msg.client)
		}
		id := clientToID[msg.client]
		if id == 0 {
			if isPing(msg.msg) {
				msg.client.msgChan <- PongCommand
				continue
			}
			if !isInit(msg.msg) {
				fmt.Printf("skipped\n")
				continue
			}
			clientToID[msg.client] = lastID + 1
			lastID += 1
			id = lastID
		}
		if isPub(msg.msg) {
			clientToID[msg.client] = 0
		}
		prependId(&msg.msg, id)
		prependLength(&msg.msg)
		clientsMu.Lock()
		for client := range clients {
			select {
			case client.msgChan <- msg.msg:
				if !prod {
					fmt.Printf("b")
				}
			default:
				if !prod {
					fmt.Print("k")
				}
				close(client.msgChan)
				delete(clients, client)
			}
		}
		fmt.Printf("\n")
		clientsMu.Unlock()
	}
}

func prependLength(data *[]byte) {
	l := len(*data) + 1
	n := make([]byte, 1, l)
	n[0] = byte(l)
	*data = append(n, *data...)
}

func prependId(data *[]byte, id uint32) {
	idData := make([]byte, 4)
	binary.BigEndian.PutUint32(idData, id)
	*data = append(idData, *data...)
}

func isInit(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return data[0] == byte(EventInit)
}

func isPub(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return data[0] == byte(EventPub)
}

func isPing(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return data[0] == byte(EventPing)
}

// EventType determines how a command on the LRC protocol should be interpreted
type EventType uint8

var PongCommand = []byte{0, 0, 0, 0, 1}

const (
	EventPing       EventType = iota // EventPing is a request for a pong, and if it comes from a server, it can also contain a welcome message
	EventPong                        // EventPong determines the latency of the connection, and if the connection has closed
	EventInit                        // EventInit initializes a message
	EventPub                         // EventPub publishes a message
	EventInsert                      // EventInsert inserts a character at a specified position in a message
	EventDelete                      // EventDelete deletes a character at a specified position in a message
	EventMuteUser                    // EventMuteUser mutes a user based on a message id. only works going forward
	EventUnmuteUser                  // EventUnmuteUser unmutes a user based on a post id. only works going forward
)
