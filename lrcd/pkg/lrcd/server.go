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
	wsserver     *wsserver
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

type wsserver struct {
	port     int
	path     string
	upgrader *websocket.Upgrader
	server   *http.Server
}

type tcpserver struct {
	port int
	nl   *net.Listener
}

type options struct {
	portTCP    *int
	portWS     *int
	pathWS     *string
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
	if options.portTCP == nil && options.portWS == nil {
		return nil, errors.New("server must be open on at least one port")
	}

	server := Server{}

	if options.portTCP != nil {
		server.tcpserver = &tcpserver{port: *options.portTCP}
	}
	if options.portWS != nil {
		server.wsserver = &wsserver{port: *options.portWS, upgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}}
	}
	server.wsserver.path = "/"
	if options.pathWS != nil {
		server.wsserver.path += *options.pathWS + "/"
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

	if s.wsserver != nil {
		mux := http.NewServeMux()
		mux.HandleFunc(s.wsserver.path+"ws", s.wsHandler)
		s.wsserver.server = &http.Server{
			Addr:    ":" + strconv.Itoa(s.wsserver.port),
			Handler: mux,
		}
		go func() {
			if err := s.wsserver.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				s.log("websocket server " + s.wsserver.path + " error: " + err.Error())
			}
		}()
		s.log("listening on ws/" + strconv.Itoa(s.wsserver.port))
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

		if s.wsserver != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.wsserver.server.Shutdown(ctx)
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

func (s *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := s.wsserver.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade failed:", err)
		return
	}
	defer conn.Close()
	client := &client{wsconn: conn, evtChan: make(chan events.LRCEvent, 100)}
	s.clientsMu.Lock()
	s.clients[client] = true
	s.clientsMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); client.wsWriter() }()
	go func() { defer wg.Done(); s.listenToWS(client) }()
	client.evtChan <- s.welcomeEvt
	s.logDebug("greeted new ws connection!")
	wg.Wait()

	s.clientsMu.Lock()
	delete(s.clients, client)
	close(client.evtChan)
	s.clientsMu.Unlock()
	conn.Close()
	s.logDebug("closed ws connection")
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

func (s *Server) listenToWS(client *client) {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			_, e, err := client.wsconn.ReadMessage()
			if err != nil {
				return
			}
			s.logDebug(fmt.Sprintf("read %x", e))
			s.eventChannel <- evt{client, e[1:]}
		}
	}
}

func (client *client) wsWriter() {
	for {
		evt, ok := <-client.evtChan
		if !ok {
			return
		}
		client.wsconn.WriteMessage(websocket.BinaryMessage, evt)
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
			id := s.clientToID[evt.client]
			if id == 0 {
				if !(events.IsInit(evt.evt) || events.IsPing(evt.evt)) {
					s.logDebug(fmt.Sprintf("skipped %x", evt.evt))
					continue
				}
				s.clientToID[evt.client] = s.lastID + 1
				s.lastID += 1
				id = s.lastID
			}

			if events.IsPing(evt.evt) {
				evt.client.evtChan <- events.ServerPongWithClientCount(uint8(len(s.clients)))
				continue
			}
			if events.IsPub(evt.evt) {
				s.clientToID[evt.client] = 0
			}
			bevt, eevt := events.GenServerEvent(evt.evt, id)

			s.clientsMu.Lock()
			for client := range s.clients {
				evtToSend := bevt
				if client == evt.client {
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
			s.clientsMu.Unlock()
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
