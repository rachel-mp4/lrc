package client

import (
	"encoding/binary"
	"math"
	"net"
	"os"
)

type inputState = int

const (
	menuNormal inputState = iota
	menuInsert
	chanNormal
	chanInsert
)

var (
	is        = menuNormal
	cmdBuffer = ""
	cursor    uint16 = math.MaxUint16
)

func AcceptInput() {
	buf := make([]byte, 10)
	quit := make(chan struct{})
	send := make(chan LRCEvent)
	var conn net.Conn
quitloop:
	for {
		select {
		case <-quit:
			break quitloop
		default:
			_, err := os.Stdin.Read(buf)
			if err != nil {
				panic(err)
			}

			switch is {
			case menuNormal:
				conn = inputMenuNormal(buf, quit, send)
			case menuInsert:
				inputMenuInsert(buf, quit, send)
			case chanNormal:
				inputChanNormal(buf, quit, send)
			case chanInsert:
				inputChanInsert(buf, quit, send)
			}
		}
	}
	hangUp(conn)
}

func inputMenuNormal(buf []byte, quit chan struct{}, send chan LRCEvent) net.Conn {
	switch buf[0] {
	case newline():
		conn := ConnectToChannel(as.url, quit, send)
		if conn != nil {
			is = chanNormal
			initChan()
			cursor = math.MaxUint16
			return conn
		}
	case 58:
		switchToMenuInsert()
	case 113:
		close(quit)
	}
	return nil
}

func switchToMenuInsert() {
	is = menuInsert
}

func inputMenuInsert(buf []byte, quit chan struct{}, send chan LRCEvent) {
	switch buf[0] {
	case newline():
		evaluateCommandBuffer(quit, send)
		switchToMenuNormal()
	case 27:
		switchToMenuNormal()
	default:
		if buf[0] > 31 && buf[0] < 127 {
			cmdBuffer = cmdBuffer + string(buf[0])
		} else if buf[0] == 127 {
			if cmdBuffer != "" {
				cmdBuffer = string(cmdBuffer[:len(cmdBuffer)-1])
			}
		}
	}
}

func evaluateCommandBuffer(quit chan struct{}, send chan LRCEvent) {
	if cmdBuffer == "q" {
		close(quit)
	}
	if cmdBuffer[0] == '/' {
		as.url = string(cmdBuffer[1:])
		ConnectToChannel(as.url, quit, send)
	}
}

func switchToMenuNormal() {
	is = menuNormal
}

func inputChanNormal(buf []byte, quit chan struct{}, send chan LRCEvent) {
	switch buf[0] {
	case 105:
		switchToChanInsert()
	case 106:
		scrollViewportDown()
	case 107:
	 	scrollViewportUp()
	case 113:
		close(quit)
	}
}

func switchToChanInsert() {
	is = chanInsert
	renderHome()
}

func inputChanInsert(buf []byte, quit chan struct{}, send chan LRCEvent) {
	if (buf[0] < 127) && (buf[0] > 31) {
		if cursor == math.MaxUint16 {
			cursor = 0
			send <- genInitEvent()
		}
		send <- genInsertEvent(cursor, string(buf[0]))
		cursor = cursor + 1
		
	} else if buf[0] == 127 {
		if cursor > 0 && cursor!=math.MaxUint16 {
			send <- genDeleteEvent(cursor)
			cursor = cursor - 1
		}
	} else if buf[0] == newline() {
		if cursor != math.MaxUint16 {
			cursor = math.MaxUint16
			send <- genPubEvent()
		}
	}
	if buf[0] == 27 {
		switchToChanNormal()
	}
}

func switchToChanNormal() {
	is = chanNormal
	renderHome()
}

func genInitEvent() LRCEvent {
	e := []byte{byte(EventInit), as.color}
	e = append(e, []byte(as.name)...)
	return e
}

func genPubEvent() LRCEvent {
	e := []byte{byte(EventPub)}
	return e
}

func genInsertEvent(at uint16, s string) LRCEvent {
	e := []byte{byte(EventInsert)}
	a := make([]byte, 2)
	binary.BigEndian.PutUint16(a, at)
	e = append(e, a...)
	e = append(e, []byte(s)...)
	return e
}

func genDeleteEvent(at uint16) LRCEvent {
	e := []byte{byte(EventDelete)}
	a := make([]byte, 2)
	binary.BigEndian.PutUint16(a, at)
	e = append(e, a...)
	return e
}