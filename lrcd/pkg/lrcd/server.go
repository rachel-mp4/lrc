package lrcd

import (
	"context"
	"errors"
	"fmt"
	events "github.com/rachel-mp4/lrc/lrc"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Server struct {
	ctx          context.Context
	cancel       context.CancelFunc
	tcpserver    *tcpserver
	clients      map[*client]bool
	clientsMu    sync.Mutex
	clientToID   map[*client]uint32
	lastID       uint32
	eventChannel chan evt
	welcomeEvt   events.LRCEvent
	logger       *log.Logger
	debugLogger  *log.Logger
	emptyChan    chan struct{}
	timeToEmit   *time.Duration
}

type tcpserver struct {
	port int
	nl   *net.Listener
}

type options struct {
	portTCP    *int
	welcome    *string
	writer     *io.Writer
	verbose    bool
	emptyChan  chan struct{}
	timeToEmit *time.Duration
}

type Option func(option *options) error

func NewServer(opts ...Option) (*Server, error) {
	var options options
	for _, opt := range opts {
		err := opt(&options)
		if err != nil {
			return nil, err
		}
	}

	server := Server{}

	if options.portTCP != nil {
		server.tcpserver = &tcpserver{port: *options.portTCP}
	}

	welcomeString := "Welcome to my lrc server!"
	if options.welcome != nil {
		welcomeString = *options.welcome
	}
	if options.writer != nil {
		server.logger = log.New(*options.writer, "[log]", log.Ldate|log.Ltime)
		if options.verbose {
			server.debugLogger = log.New(*options.writer, "[debug]", log.Ldate|log.Ltime)
		}
	}
	if options.emptyChan != nil {
		server.emptyChan = options.emptyChan
		server.timeToEmit = options.timeToEmit
	}

	e := append([]byte{byte(events.EventPing)}, []byte(welcomeString)...)
	wm, _ := events.GenServerEvent(e, 0)
	server.welcomeEvt = wm
	return &server, nil
}

func (s *Server) Start() error {
	if s.ctx != nil {
		return errors.New("cannot start already started server")
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.clients = make(map[*client]bool)
	s.clientsMu = sync.Mutex{}
	s.clientToID = make(map[*client]uint32)
	s.lastID = 0
	s.eventChannel = make(chan evt, 100)
	go s.broadcaster()
	go func() {
		t := time.NewTicker(15 * time.Second)
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-t.C:
				s.broadcastAll(s.welcomeEvt)
			}
		}
	}()
	s.logDebug("Hello, world!")
	if s.tcpserver != nil {
		nl, err := net.Listen("tcp", ":"+strconv.Itoa(s.tcpserver.port))
		if err != nil {
			s.log(err.Error())
			return err
		}
		s.tcpserver.nl = &nl
		go s.greet()
		s.log("listening on tcp/" + strconv.Itoa(s.tcpserver.port))
	}

	if s.timeToEmit != nil {
		time.AfterFunc(*s.timeToEmit, s.checkIfEmpty)
	}
	return nil
}

func (s *Server) Stop() error {
	select {
	case <-s.ctx.Done():
		return errors.New("cannot stop already stopped server")
	default:
		s.cancel()

		if s.tcpserver != nil {
			(*s.tcpserver.nl).Close()
		}
		s.logDebug("Goodbye world :c")
		return nil
	}
}

func (s *Server) checkIfEmpty() {
	if s.emptyChan == nil {
		return
	}
	if len(s.clients) != 0 {
		return
	}
	close(s.emptyChan)
	s.emptyChan = nil
}

// Client is a model for a client's connection, and their evtChannel, the queue of LRCEvents that have yet to be written to the connection
type client struct {
	tcpconn *net.Conn
	wsconn  *websocket.Conn
	evtChan chan events.LRCEvent
}

// Evt is a model for an lrc event from a specific client
type evt struct {
	client *client
	evt    events.LRCEvent
}

func (s *Server) WSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		upgrader := &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Upgrade failed:", err)
			return
		}
		defer conn.Close()

		client := &client{wsconn: conn, evtChan: make(chan events.LRCEvent, 100)}
		s.clientsMu.Lock()
		s.clients[client] = true
		s.clientsMu.Unlock()

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); s.wsWriter(client, ctx, cancel) }()
		go func() { defer wg.Done(); s.listenToWS(client, ctx, cancel) }()
		client.evtChan <- s.welcomeEvt
		s.logDebug("greeted new ws connection!")
		wg.Wait()

		s.clientsMu.Lock()
		delete(s.clients, client)
		close(client.evtChan)
		s.checkIfEmpty()
		s.clientsMu.Unlock()
		conn.Close()
		s.logDebug("closed ws connection")
	}

}

// Entrypoint
// greet accepts new connections, and then passes them to the handler
func (s *Server) greet() {
	for {
		conn, err := (*s.tcpserver.nl).Accept()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				s.log(err.Error())
				return
			}
			log.Println("Error accepting connection:", err)
			continue
		}
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetNoDelay(true)
		go s.tcpHandler(conn)
	}
}

// tcpHandler handles the lifetime of a connection (pings it, makes a model for the client, reads from it, and then ultimately removes it when they disconnect)
func (s *Server) tcpHandler(conn net.Conn) {
	client := &client{tcpconn: &conn, evtChan: make(chan events.LRCEvent, 100)}
	s.clientsMu.Lock()
	s.clients[client] = true
	s.clientsMu.Unlock()

	var wg sync.WaitGroup
	readChan := make(chan []byte, 10)
	outChan := make(chan []byte, 10)

	quit := make(chan struct{}) //can be closed by degunker if something goes wrong

	wg.Add(4)
	go func() { defer wg.Done(); client.tcpWriter(quit, s.ctx) }()
	go func() { defer wg.Done(); client.relayToBroadcaster(outChan, s.eventChannel, quit, s.ctx) }()
	go func() { defer wg.Done(); events.Degunker(10, readChan, outChan, quit, s.ctx) }()
	go func() { defer wg.Done(); s.listenToTCP(client, readChan, quit, s.ctx) }()
	client.evtChan <- s.welcomeEvt
	s.logDebug("greeted new tcp connection!")
	wg.Wait()
	close(outChan)

	s.clientsMu.Lock()
	delete(s.clients, client)
	close(client.evtChan)
	s.checkIfEmpty()
	s.clientsMu.Unlock()
	conn.Close()
	s.logDebug("closed tcp connection")
}

// relayToBroadcaster wrangles the output of degunker into an event that we can send to broadcaster through the global eventChannel.
// Returns if quit or outChan closes.
func (client *client) relayToBroadcaster(fromDegunker chan events.LRCEvent, toBroadcaster chan evt, degunkerError chan struct{}, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-degunkerError:
			return
		case e, ok := <-fromDegunker:
			if !ok {
				return
			}
			toBroadcaster <- evt{client, e}
		}
	}
}

// listenToClient polls the clients connection and then sends any data it recieves to the degunker.
// If the connection closes, it closes readChan, which causes degunker to close quit
func (server *Server) listenToTCP(client *client, readChan chan []byte, degunkerError chan struct{}, ctx context.Context) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return
		case <-degunkerError:
			return
		default:
			n, err := (*client.tcpconn).Read(buf)
			if err != nil {
				close(readChan)
				return
			}
			m := make([]byte, n)
			copy(m, buf)
			server.logDebug(fmt.Sprintf("read %x", m))
			readChan <- m
		}
	}
}

// clientWriter takes an event from the clients event channel, and writes it to the tcp connection.
// If the degunker runs into an error, or if the client's eventChannel closes, then this returns
func (client *client) tcpWriter(degunkerError chan struct{}, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-degunkerError:
			return
		case evt, ok := <-client.evtChan:
			if !ok {
				return
			}
			(*client.tcpconn).Write(evt)
		}
	}
}

func (s *Server) listenToWS(client *client, ctx context.Context, cancel context.CancelFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			cancel()
			return
		default:
			_, e, err := client.wsconn.ReadMessage()
			if err != nil {
				cancel()
				return
			}
			s.logDebug(fmt.Sprintf("read %x", e))
			s.eventChannel <- evt{client, e[1:]}
		}
	}
}

func (s *Server) wsWriter(client *client, ctx context.Context, cancel context.CancelFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			cancel()
			return
		case evt, ok := <-client.evtChan:
			if !ok {
				cancel()
				return
			}
			err := client.wsconn.WriteMessage(websocket.BinaryMessage, evt)
			if err != nil {
				cancel()
				return
			}
		}

	}
}

// broadcaster takes an event from the events channel, and broadcasts it to all the connected clients individual event channels
func (s *Server) broadcaster() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case evt := <-s.eventChannel:
			s.logDebug(fmt.Sprintf("recieved %x from %x", evt.evt, evt.client))
			if events.IsPing(evt.evt) {
				s.clientsMu.Lock()
				evt.client.evtChan <- events.ServerPongWithClientCount(uint8(len(s.clients)))
				s.clientsMu.Unlock()
				continue
			}

			id, ok := s.clientToID[evt.client]
			if !ok {
				if !events.IsInit(evt.evt) {
					s.logDebug(fmt.Sprintf("skipped %x", evt.evt))
					continue
				}
				s.clientToID[evt.client] = s.lastID + 1
				s.lastID += 1
				id = s.lastID
				s.broadcastInit(evt.evt, evt.client, id)
				continue
			}

			if events.IsPub(evt.evt) {
				delete(s.clientToID, evt.client)
			}
			lrcEvent, _ := events.GenServerEvent(evt.evt, id)
			s.broadcastAll(lrcEvent)
		}
	}
}

func (s *Server) broadcastAll(evt []byte) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for client := range s.clients {
		select {
		case client.evtChan <- evt:
			s.logDebug(fmt.Sprintf("b %x", evt))
		default:
			s.log("kicked client")
			if client.tcpconn != nil {
				(*client.tcpconn).Close()
			}
			if client.wsconn != nil {
				(*client.wsconn).Close()
			}
			delete(s.clients, client)
		}
	}
}

func (s *Server) broadcastInit(evt []byte, c *client, id uint32) {
	bevt, eevt := events.GenServerEvent(evt, id)
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	for client := range s.clients {
		evtToSend := bevt
		if client == c {
			evtToSend = eevt
		}
		select {
		case client.evtChan <- evtToSend:
			s.logDebug(fmt.Sprintf("b %x", bevt))
		default:
			s.log("kicked client")
			if client.tcpconn != nil {
				(*client.tcpconn).Close()
			}
			if client.wsconn != nil {
				(*client.wsconn).Close()
			}
			delete(s.clients, client)
		}
	}
}

// logDebug debugs unless in production
func (server *Server) logDebug(s string) {
	if server.debugLogger != nil {
		server.debugLogger.Println(s)
	}
}

func (server *Server) log(s string) {
	if server.logger != nil {
		server.logger.Println(s)
	}
}

func (t evt) parseEvent() string {
	if len(t.evt) < 5 {
		return ""
	}
	return fmt.Sprintf("[%x]", t.evt)
}
