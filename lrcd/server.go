package main

import (
	"fmt"
	"log"
	events "lrc"
	"net"
	"sync"
)

// Client is a model for a client's connection, and their evtChannel, the queue of LRCEvents that have yet to be written to the connection
type Client struct {
	conn    net.Conn
	evtChan chan events.LRCEvent
}

// Evt is a model for an lrc event from a specific client
type Evt struct {
	client *Client
	evt    events.LRCEvent
}

var (
	clients      = make(map[*Client]bool)
	clientToID   = make(map[*Client]uint32)
	lastID       = uint32(0)
	eventChannel = make(chan Evt, 100)
	clientsMu    sync.Mutex
	prod         bool = false
)

func main() {
	log := log.Default()
	log.Println("Hello, world!")
	ln, err := net.Listen("tcp", ":927")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()
	log.Println("Listening on 927")

	go broadcaster()
	wm := append([]byte{byte(events.EventPing)}, []byte("Welcome To The Beginning Of The Rest Of Your Life")...)
	wm, _ = events.GenServerEvent(wm, 0)
	greet(ln, wm)
}

// greet accepts new connections, and then passes them to the handler
func greet(ln net.Listener, wm events.LRCServerEvent) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Error accepting connection:", err)
			continue
		}
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetNoDelay(true)
		go handle(conn, wm)
	}
}

// handle handles the lifetime of a connection (pings it, makes a model for the client, reads from it, and then ultimately removes it when they disconnect)
func handle(conn net.Conn, wm []byte) {
	conn.Write(wm)
	logDebug("Greeted new connection!")
	client := &Client{conn: conn, evtChan: make(chan events.LRCEvent, 100)}
	clientsMu.Lock()
	clients[client] = true
	clientsMu.Unlock()

	var wg sync.WaitGroup
	readChan := make(chan []byte, 10)
	outChan := make(chan []byte, 10)

	quit := make(chan struct{}) //can be closed by degunker if something goes wrong

	wg.Add(4)
	go func() { defer wg.Done(); clientWriter(client, quit) }()
	go func() { defer wg.Done(); relayToBroadcaster(client, outChan, quit) }()
	go func() { defer wg.Done(); events.Degunker(10, readChan, outChan, quit) }()
	go func() { defer wg.Done(); listenToClient(client, readChan, quit) }()
	wg.Wait()
	close(outChan)

	clientsMu.Lock()
	delete(clients, client)
	close(client.evtChan)
	clientsMu.Unlock()
	conn.Close()
	logDebug("Closed connection")
}

// relayToBroadcaster wrangles the output of degunker into an event that we can send to broadcaster through the global eventChannel.
// Returns if quit or outChan closes.
func relayToBroadcaster(client *Client, outChan chan events.LRCEvent, quit chan struct{}) {
	for {
		select {
		case <-quit:
			return
		case evt, ok := <-outChan:
			if !ok {
				return
			}
			eventChannel <- Evt{client, evt}
		}
	}
}

// listenToClient polls the clients connection and then sends any daya it recieves to the degunker.
// If the connection closes, it closes readChan, which causes degunker to close quit
func listenToClient(client *Client, readChan chan []byte, quit chan struct{}) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-quit:
			return
		default:
			n, err := client.conn.Read(buf)
			if err != nil {
				close(readChan)
				return
			}
			m := make([]byte, n)
			copy(m, buf)
			logDebug(fmt.Sprintf("read %x", m))
			readChan <- m
		}
	}
}

// clientWriter takes an event from the clients event channel, and writes it to the tcp connection.
// If the degunker runs into an error, or if the client's eventChannel closes, then this returns
func clientWriter(client *Client, quit chan struct{}) {
	for {
		select {
		case <-quit:
			return
		case evt, ok := <-client.evtChan:
			if !ok {
				return
			}
			client.conn.Write(evt)
		}
	}
}

// broadcaster takes an event from the events channel, and broadcasts it to all the connected clients individual event channels
func broadcaster() {
	for evt := range eventChannel {
		logDebug(fmt.Sprintf("recieved %x from %x", evt.evt, evt.client))
		id := clientToID[evt.client]
		if id == 0 {
			if !(events.IsInit(evt.evt) || events.IsPing(evt.evt)) {
				logDebug(fmt.Sprintf("skipped %x",evt.evt))
				continue
			}
			clientToID[evt.client] = lastID + 1
			lastID += 1
			id = lastID
		}
		if events.IsPing(evt.evt) {
			evt.client.evtChan <- events.ServerPong
			continue
		}
		if events.IsPub(evt.evt) {
			clientToID[evt.client] = 0
		}
		bevt, _ := events.GenServerEvent(evt.evt, id)

		clientsMu.Lock()
		for client := range clients {
			select {
			case client.evtChan <- bevt:
				logDebug(fmt.Sprintf("b %x", bevt))
			default:
				logDebug("k")
				err := client.conn.Close()
				if err != nil {
					delete(clients, client)
				}
			}
		}
		clientsMu.Unlock()
	}
}

// logDebug debugs unless in production
func logDebug(s string) {
	if !prod {
		log.Println(s)
	}
}
