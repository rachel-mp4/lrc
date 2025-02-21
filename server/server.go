package main

import (
	"encoding/binary"
	"log"
	"net"
	"sync"
)

type Client struct {
	conn    net.Conn
	msgChan chan []byte
}

type Msg struct {
	client Client
	msg    []byte
}

var (
	clients    = make(map[*Client]bool)
	clientToID = make(map[*Client]uint32)
	lastID     = uint32(0)
	messages   = make(chan Msg)
	clientsMu  sync.Mutex
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

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetNoDelay(true)
		go greet(conn)
	}
}

func greet(conn net.Conn) {
	conn.Write([]byte("Welcome To The Beginning Of The Rest Of Your Life"))
	client := &Client{conn: conn, msgChan: make(chan []byte)}
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
		msg := Msg{client: *client, msg: m}
		messages <- msg
	}
	clientsMu.Lock()
	delete(clients, client)
	clientsMu.Unlock()
	conn.Close()
}

func clientWriter(client *Client) {
	for data := range client.msgChan {
		id := clientToID[client]
		head := make([]byte, 4)
		binary.BigEndian.PutUint32(head, id)
		msg := append(head, data...)
		client.conn.Write(msg)
	}
}

func broadcaster() {
	for msg := range messages {
		id := clientToID[&msg.client]
		if id == 0 {
			clientToID[&msg.client] = lastID + 1
			lastID += 1
		}
		clientsMu.Lock()
		for client := range clients {
			if msg.client != *client {
				select {
				case client.msgChan <- msg.msg:
				default:
					close(client.msgChan)
					delete(clients, client)
				}
			}
		}
		clientsMu.Unlock()
	}
}
