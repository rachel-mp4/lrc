package client

import (
	"io"
	"log"
	events "lrc"
	"net"
	"time"
)

var (
	pingChannel = make(chan struct{})
)

type LRCCommand struct {
	n   int
	buf events.LRCEvent
}

// ConnectToChannel attempts to connect to a url, and if it succeeds, it sets up a listener, chatter, and pinger, and returns the connection
func ConnectToChannel(url string, quit chan struct{}, send chan events.LRCEvent) net.Conn {
	conn, err := dial(url)
	if err != nil {
		connectionFailure(url, err)
		return nil
	}
	deNagle(conn)

	outChan := make(chan events.LRCEvent)
	readChan := make(chan events.LRCEvent)

	go chat(conn, send, quit)
	go relayToParser(outChan, quit)
	go events.Degunker(100, readChan, outChan, quit)
	go listen(conn, readChan, quit)
	go pinger(send, quit)
	return conn
}

func relayToParser(outChan chan events.LRCEvent, quit chan struct{}) {
	for {
		select {
		case <-quit:
			close(outChan)
			return
		case evt, ok := <-outChan:
			if !ok {
				return
			}
			addToCmdLog(evt)
			parseCommand(evt)
		}
	}

}

func chat(conn net.Conn, send chan []byte, quit chan struct{}) {
	for {
		select {
		case <-quit:
			return
		case msg, ok := <-send:
			if !ok {
				return
			}
			conn.Write(msg)
		}
	}
}

// listen listens for LRCEvents and then acts on them accordingly
func listen(conn net.Conn, readChan chan []byte, quit chan struct{}) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-quit:
			return
		default:
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Fatal("Read error:", err)
				} else {
					log.Println("Server closed")
				}
				close(readChan)
			}
			e := make([]byte, n)
			copy(e, buf)
			readChan <- e
		}
	}
}

func parseCommand(e events.LRCEvent) {
	switch events.ParseEventType(e) {
	case events.EventPing:
		if len([]byte(e)) > 5 {
			setWelcomeMessage(string(e[5:]))
		} else {
			setWelcomeMessage("Fail")
		}
	case events.EventPong:
		go ponged()
		setCurrentConnected(int(e[5]))
	case events.EventInit:
		id, color, name, isFromMe := events.ParseInitEvent(e)
		initMsg(id, color, name, true, isFromMe)
	case events.EventPub:
		pubMsg(events.ParsePubEvent(e))
	case events.EventInsert:
		insertIntoMsg(events.ParseInsertEvent(e))
	case events.EventDelete:
		deleteFromMessage(events.ParseDeleteEvent(e))
	}
}

func pinger(send chan events.LRCEvent, quit chan struct{}) {
	ping(send)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-quit:
		case <-ticker.C:
			ping(send)
		}
	}
}

func ping(send chan events.LRCEvent) {
	p := make([]byte, 2)
	copy(p, events.ClientPing)
	t0 := time.Now()
	send <- p
	<-pingChannel
	t1 := time.Now()
	setPingTo(int(t1.Sub(t0).Milliseconds()))
}

func ponged() {
	pingChannel <- struct{}{}
}

// dial dials the url
func dial(url string) (net.Conn, error) {
	return net.Dial("tcp", url+":927")
}

// hangUp closes the connection if it exists
func hangUp(conn net.Conn) {
	if conn != nil {
		conn.Close()
	}
}

// deNagle disables Nagle's algorithm, causing the connection to send tcp packets as soon as possible at the cost of increasing overhead by not pooling events
func deNagle(conn net.Conn) {
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetNoDelay(true)
}
