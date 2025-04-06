package lrcd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	events "lrc"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

type Server struct {
	started      bool
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
}

type wsserver struct {
	port     int
	upgrader *websocket.Upgrader
	server   *http.Server
}

type tcpserver struct {
	port int
	nl   *net.Listener
}

type options struct {
	portTCP *int
	portWS  *int
	welcome *string
	writer  *io.Writer
	verbose bool
}

type Option func(option *options) error

func WithTCPPort(port int) Option {
	return func(options *options) error {
		if port < 0 {
			return errors.New("port should be postive")
		}
		options.portTCP = &port
		return nil
	}
}

func WithWSPort(port int) Option {
	return func(options *options) error {
		if port < 0 {
			return errors.New("port should be postive")
		}
		options.portWS = &port
		return nil
	}
}

func WithWelcome(welcome string) Option {
	return func(options *options) error {
		if utf8.RuneCountInString(welcome) > 50 {
			return errors.New("welcome must be at most 50 runes")
		}
		options.welcome = &welcome
		return nil
	}
}

func WithLogging(w io.Writer, verbose bool) Option {
	return func(options *options) error {
		if w == nil {
			return errors.New("must provide a writer to log to")
		}
		options.writer = &w
		options.verbose = verbose
		return nil
	}
}

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
	e := append([]byte{byte(events.EventPing)}, []byte(welcomeString)...)
	wm, _ := events.GenServerEvent(e, 0)
	server.welcomeEvt = wm
	return &server, nil
}

func (server *Server) Start() error {
	if server.started {
		return errors.New("cannot start already started server")
	}
	server.clients = make(map[*client]bool)
	server.clientsMu = sync.Mutex{}
	server.clientToID = make(map[*client]uint32)
	server.lastID = 0
	server.eventChannel = make(chan evt, 100)
	go server.broadcaster()
	server.logDebug("Hello, world!")
	if server.tcpserver != nil {
		nl, err := net.Listen("tcp", ":"+strconv.Itoa(server.tcpserver.port))
		if err != nil {
			server.log(err.Error())
			return err
		}
		server.tcpserver.nl = &nl
		go server.greet()
		server.log("listening on tcp/" + strconv.Itoa(server.tcpserver.port))
	}

	if server.wsserver != nil {
		mux := http.NewServeMux()
		mux.HandleFunc("/ws", server.wsHandler)
		server.wsserver.server = &http.Server{
			Addr:    ":" + strconv.Itoa(server.wsserver.port),
			Handler: mux,
		}
		go func() {
			if err := server.wsserver.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				server.log("websocket server error: " + err.Error())
			}
		}()
		server.log("listening on ws/" + strconv.Itoa(server.wsserver.port))
	}
	server.started = true
	return nil
}

func (server *Server) Stop() error {
	if !server.started {
		return errors.New("cannot stop already stopped server")
	}
	if server.tcpserver != nil {
		(*server.tcpserver.nl).Close()
	}
	if server.wsserver != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.wsserver.server.Shutdown(ctx)
	}
	close(server.eventChannel)
	server.logDebug("Goodbye world :c")
	return nil
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

func (server *Server) wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := server.wsserver.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade failed:", err)
		return
	}
	defer conn.Close()
	client := &client{wsconn: conn, evtChan: make(chan events.LRCEvent, 100)}
	server.clientsMu.Lock()
	server.clients[client] = true
	server.clientsMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); wsWriter(*client) }()
	go func() { defer wg.Done(); server.listenToWS(client) }()
	client.evtChan <- server.welcomeEvt
	server.logDebug("greeted new ws connection!")
	wg.Wait()

	server.clientsMu.Lock()
	delete(server.clients, client)
	close(client.evtChan)
	server.clientsMu.Unlock()
	conn.Close()
	server.logDebug("closed ws connection")
}

// Entrypoint
// greet accepts new connections, and then passes them to the handler
func (server *Server) greet() {
	for {
		conn, err := (*server.tcpserver.nl).Accept()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				server.log(err.Error())
				return
			}
			log.Println("Error accepting connection:", err)
			continue
		}
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetNoDelay(true)
		go server.tcpHandler(conn)
	}
}

// handle handles the lifetime of a connection (pings it, makes a model for the client, reads from it, and then ultimately removes it when they disconnect)
func (server *Server) tcpHandler(conn net.Conn) {
	client := &client{tcpconn: &conn, evtChan: make(chan events.LRCEvent, 100)}
	server.clientsMu.Lock()
	server.clients[client] = true
	server.clientsMu.Unlock()

	var wg sync.WaitGroup
	readChan := make(chan []byte, 10)
	outChan := make(chan []byte, 10)

	quit := make(chan struct{}) //can be closed by degunker if something goes wrong

	wg.Add(4)
	go func() { defer wg.Done(); clientWriter(client, quit) }()
	go func() { defer wg.Done(); server.relayToBroadcaster(client, outChan, quit) }()
	go func() { defer wg.Done(); events.Degunker(10, readChan, outChan, quit) }()
	go func() { defer wg.Done(); server.listenToClient(client, readChan, quit) }()
	client.evtChan <- server.welcomeEvt
	server.logDebug("greeted new tcp connection!")
	wg.Wait()
	close(outChan)

	server.clientsMu.Lock()
	delete(server.clients, client)
	close(client.evtChan)
	server.clientsMu.Unlock()
	conn.Close()
	server.logDebug("closed tcp connection")
}

// relayToBroadcaster wrangles the output of degunker into an event that we can send to broadcaster through the global eventChannel.
// Returns if quit or outChan closes.
func (server *Server) relayToBroadcaster(client *client, outChan chan events.LRCEvent, quit chan struct{}) {
	for {
		select {
		case <-quit:
			return
		case e, ok := <-outChan:
			if !ok {
				return
			}
			server.eventChannel <- evt{client, e}
		}
	}
}

// listenToClient polls the clients connection and then sends any daya it recieves to the degunker.
// If the connection closes, it closes readChan, which causes degunker to close quit
func (server *Server) listenToClient(client *client, readChan chan []byte, quit chan struct{}) {
	buf := make([]byte, 1024)
	for {
		select {
		case <-quit:
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
func clientWriter(client *client, quit chan struct{}) {
	for {
		select {
		case <-quit:
			return
		case evt, ok := <-client.evtChan:
			if !ok {
				return
			}
			(*client.tcpconn).Write(evt)
		}
	}
}

func (server *Server) listenToWS(client *client) {
	for {
		_, e, err := client.wsconn.ReadMessage()
		if err != nil {
			return
		}
		server.logDebug(fmt.Sprintf("read %x", e))
		server.eventChannel <- evt{client, e[1:]}
	}
}

func wsWriter(client client) {
	for {
		evt, ok := <-client.evtChan
		if !ok {
			return
		}
		client.wsconn.WriteMessage(websocket.BinaryMessage, evt)
	}
}

// broadcaster takes an event from the events channel, and broadcasts it to all the connected clients individual event channels
func (server *Server) broadcaster() {
	for evt := range server.eventChannel {
		server.logDebug(fmt.Sprintf("recieved %x from %x", evt.evt, evt.client))
		id := server.clientToID[evt.client]
		if id == 0 {
			if !(events.IsInit(evt.evt) || events.IsPing(evt.evt)) {
				server.logDebug(fmt.Sprintf("skipped %x", evt.evt))
				continue
			}
			server.clientToID[evt.client] = server.lastID + 1
			server.lastID += 1
			id = server.lastID
		}
		if events.IsPing(evt.evt) {
			evt.client.evtChan <- events.ServerPongWithClientCount(uint8(len(server.clients)))
			continue
		}
		if events.IsPub(evt.evt) {
			server.clientToID[evt.client] = 0
		}
		bevt, eevt := events.GenServerEvent(evt.evt, id)

		server.clientsMu.Lock()
		for client := range server.clients {
			evtToSend := bevt
			if client == evt.client {
				evtToSend = eevt
			}
			select {
			case client.evtChan <- evtToSend:
				server.logDebug(fmt.Sprintf("b %x", bevt))
			default:
				server.log("kicked client")
				if client.tcpconn != nil {
					err := (*client.tcpconn).Close()
					if err != nil {
						//TODO what's going on here chat
					}
				}
				if client.wsconn != nil {
					err := (*client.wsconn).Close()
					if err != nil {
						//TODO help chat
					}
				}
				delete(server.clients, client)
			}
		}
		server.clientsMu.Unlock()
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
