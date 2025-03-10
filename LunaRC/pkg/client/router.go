package client

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"time"
)

// EventType determines how a command on the LRC protocol should be interpreted
type EventType int

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

type LRCCommand struct {
	n   int
	buf LRCEvent
}

func ConnectToChannel(url string, quit chan struct{}, send chan LRCEvent) net.Conn {
	conn, err := dial(url)
	if err != nil {
		connectionFailure(url, err)
		return nil
	}
	deNagle(conn)
	go listen(conn, quit, send)
	go chat(conn, quit, send)
	return conn
}

func hangUp(conn net.Conn) {
	conn.Close()
}

func dial(url string) (net.Conn, error) {
	return net.Dial("tcp", ":927")
}

func deNagle(conn net.Conn) {
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetNoDelay(true)
}

func listen(conn net.Conn, quit chan struct{}, send chan []byte) {
	recieve := make(chan LRCEvent, 100)
	go listenAndRelay(conn, recieve, quit)
	for {
		select {
		case <-quit:
			return
		case cmd := <-recieve:
			addToCmdLog(cmd)
			if parseCommand(cmd) {
				send <- PongCommand
			}

		}
	}
}

var PingCommand = LRCEvent([]byte{byte(EventPing)})

var PongCommand = LRCEvent([]byte{byte(EventPong)})

func parseEventType(e LRCEvent) EventType {
	return EventType(e[4])
}

func parseCommand(e LRCEvent) bool {
	switch parseEventType(e) {
	case EventPing:
		if len([]byte(e)) > 5 {
			setWelcomeMessage(string(e[5:]))
		}
		return true
	case EventPong:
		go ponged()
	case EventInit:
		initMsg(parseInitEvent(e))
	case EventPub:
		pubMsg(parsePubEvent(e))
	case EventInsert:
		insertIntoMsg(parseInsertEvent(e))
	case EventDelete:
		deleteFromMessage(parseDeleteEvent(e))
	}
	return false
}

var pingChannel = make(chan struct{})

func ping() {
	t0 := time.Now()
	<-pingChannel
	t1 := time.Now()
	setPingTo(int(t1.Sub(t0).Milliseconds()))
}

func ponged() {
	pingChannel <- struct{}{}
}

func parseInitEvent(e LRCEvent) (uint32, user, bool) {
	return binary.BigEndian.Uint32(e[0:4]), user{e[6], string(e[7:])}, false
}

func parsePubEvent(e LRCEvent) uint32 {
	return binary.BigEndian.Uint32(e[0:4])
}

func parseInsertEvent(e LRCEvent) (uint32, uint16, string) {
	return binary.BigEndian.Uint32(e[0:4]), binary.BigEndian.Uint16(e[5:7]), string(e[7])
}

func parseDeleteEvent(e LRCEvent) (uint32, uint16) {
	return binary.BigEndian.Uint32(e[0:4]), binary.BigEndian.Uint16(e[5:7])
}

func chat(conn net.Conn, quit chan struct{}, send chan []byte) {
	for {
		select {
		case <-quit:
			return
		case msg := <-send:
			conn.Write(msg)
		}
	}
}

type LRCEvent = []byte

func listenAndRelay(conn net.Conn, recieve chan LRCEvent, quit chan struct{}) {
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Fatal("Read error:", err)
			} else {
				log.Println("Server closed")
			}
			close(quit)
		}
		e := make(LRCEvent, n)
		copy(e, buf)
		recieve <- e
	}
}
