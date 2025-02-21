package main

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/term"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

func main() {
	log := log.Default()
	fmt.Println(`  %%%%%%%
 %%%%%%%%%%  %%%            %   %           %%%%%%%%
   %%%%%%%%%%%%%%%%%        %%%%     %%%%%%%%%%%%%%%%%%
     %%%%%%%%%%%%%%%%%%%%%%% %% %%%%%%%%%%%%%%%%%%%%%%%%%%
       %%%%%%%%%%%%%%%%%%%%% %% %%%%%%%%%%%%%%%%%%%%%%%
            %%%%%%%%%%%%%%%% %% %%%%%%%%%%%%%%%%%%%%%
         %%%%%%%%%%%%%%%%%%% %% %%%%%%%%%%%%%%
       %%%%%%%%%%%%%%%%%%%%%    %%%%%%%%%%%%%%%%%%
          %%%%%%%%%%%%         %%%%%%%%%%%%%%%%
             %%%%%               %%%%%%%%%
                     moth11.        %`)

	oldState, err := term.MakeRaw(int(syscall.Stdin))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(syscall.Stdin), oldState)
	conn, err := net.Dial("tcp", ":6667")
	if err != nil {
		log.Fatal("Error conecting: ", err)
	}
	defer conn.Close()
	log.Println("Connected!")
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetNoDelay(true)
	var wg sync.WaitGroup
	quit := make(chan struct{})
	wg.Add(2)
	go listen(conn, &wg, quit)
	go chat(conn, &wg, quit)
	wg.Wait()
}

func listen(conn net.Conn, wg *sync.WaitGroup, quit chan struct{}) {
	defer wg.Done()
	buf := make([]byte, 1024)
	quitLoop:
	for {
		select {
		case <- quit:
			return
		default:
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Fatal("Read error:", err)
				} else {
					fmt.Println("Server closed")
				}
				break quitLoop
			}
			parseAction(buf, n)
		}
	}
	close(quit)
	fmt.Println("Disconnected from server")
}

func chat(conn net.Conn, wg *sync.WaitGroup, quit chan struct{}) {
	defer wg.Done()
	buf := make([]byte, 10)
	home = Cpos{15,0}
	active := false
	quitLoop:
	for {
		select {
		case <- quit:
			return
		default:
		_, err := os.Stdin.Read(buf)
		if err != nil {
			panic(err)
		}
		switch buf[0] {
		case 3:
			break quitLoop
		case 10:
			if active {
				publishMyMsg(conn)
				active = false
			}
		case 27: 
			switch buf[1] {
			case 'A':
				cMoveMyMsg(conn, Cpos{home.row, 0})
			}
		case 127:
			backspaceMyMsg(conn)
		default:
			if !active {
				createMyMsg(conn)
				active = true
			}
			s := string(buf[0])
			appendMyMsg(conn,s)
		}
		}
	}
	close(quit)
}

func cMoveMyMsg(conn net.Conn, cpos Cpos) {
	fmtMu.Lock()
	home = cpos
	goHome()
	flag := []byte{3}
	pos := make([]byte,4)
	binary.BigEndian.PutUint32(pos, uint32(home.col))
	msg := append(flag, pos...)
	conn.Write(msg)
	fmtMu.Unlock()
}

func publishMyMsg(conn net.Conn) {
	fmtMu.Lock()
	fmt.Print("\r\n")
	home.row += 2
	goHome()
	conn.Write([]byte{4})
	fmtMu.Unlock()
}

func backspaceMyMsg(conn net.Conn) {
	fmtMu.Lock()
	fmt.Print("\b \b")
	conn.Write([]byte{2})
	fmtMu.Unlock()
}

func createMyMsg(conn net.Conn) {
	fmtMu.Lock()
	fmt.Print("anon:")
	
	flag := []byte{0}
	msg := append(flag, []byte("anon")...)
	conn.Write(msg)
	fmtMu.Unlock()
}

func appendMyMsg(conn net.Conn, s string) {
	fmtMu.Lock()
	fmt.Print(s)
	home.col += 1
	flag := []byte{1}
	// pos := make([]byte,4)
	// binary.BigEndian.PutUint32(pos, uint32(home.col - 1))
	// header := append(flag, pos...)
	msg := append(flag, []byte(s)...)
	conn.Write(msg)
	fmtMu.Unlock()
}

type Msg struct {
	id   int
	cpos Cpos
	time time.Time
	name string
}

type Cpos struct {
	row uint32
	col uint32
}

var (
	idToMsg = make(map[int]Msg)
	home    Cpos
	fmtMu   sync.Mutex
)

func parseAction(action []byte, n int) {
	id := int(binary.BigEndian.Uint32(action[0:4]))
	switch action[4] {
	case 0:
		createMsg(id, time.Now(), string(action[5:n]))
	case 1:
		appendMsg(id, string(action[5:n]))
	case 2:
		backspaceMsg(id)
	case 3:
		cMoveMsg(id, binary.BigEndian.Uint32(action[5:9]))
	case 4:
		publishMsg(id)
	}
}

func publishMsg(id int) {
	delete(idToMsg, id)
}

func cMoveMsg(id int, col uint32) {
	msg := idToMsg[id]
	msg.cpos.col = col
	idToMsg[id] = msg
}

func createMsg(id int, time time.Time, name string) {
	msg := Msg{id, Cpos{row: home.row, col: 0}, time, name}
	fmtMu.Lock()
	moveToCpos(msg.cpos)
	fmt.Printf("%s: \n", name)
	msg.cpos.col = uint32(len(name) + 2)
	idToMsg[id] = msg
	home.row += 1
	goHome()
	fmtMu.Unlock()
}

func appendMsg(id int, data string) {
	msg := idToMsg[id]
	fmtMu.Lock()
	moveToCpos(idToMsg[id].cpos)
	fmt.Print(data)
	mcpos := msg.cpos
	mcpos.col += 1
	msg.cpos = mcpos
	idToMsg[id] = msg
	goHome()
	fmtMu.Unlock()
}

func moveToCpos(cpos Cpos) {
	fmt.Printf("\033[%dd\033[%dG", cpos.row, cpos.col)
}

func backspaceMsg(id int) {
	msg := idToMsg[id]
	fmtMu.Lock()
	moveToCpos(msg.cpos)
	fmt.Print("\b \b")
	goHome()
	fmtMu.Unlock()
}

func goHome() {
	moveToCpos(home)
}