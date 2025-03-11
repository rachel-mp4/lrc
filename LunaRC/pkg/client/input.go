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
	is               = menuNormal
	cmdBuffer        = ""
	cursor    uint16 = math.MaxUint16
	wordL     uint16 = 0
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
			n, err := os.Stdin.Read(buf)
			if err != nil {
				panic(err)
			}
			input := buf[:n]

			switch is {
			case menuNormal:
				conn = inputMenuNormal(input, quit, send)
			case menuInsert:
				inputMenuInsert(input, quit, send)
			case chanNormal:
				inputChanNormal(input, quit, send)
			case chanInsert:
				inputChanInsert(input, quit, send)
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
	case 100:
		dumpCmdLog()
	case 105:
		switchToChanInsert()
		cursorHome()
	case 106:
		scrollViewportDown(false)
	case 107:
		scrollViewportUp(false)
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
			wordL = 0
		}
		send <- genInsertEvent(cursor, string(buf[0]))
		cursor = cursor + 1
		wordL = wordL - 1

	} else if buf[0] == 127 {
		if cursor > 0 && cursor != math.MaxUint16 {
			send <- genDeleteEvent(cursor)
			cursor = cursor - 1
			wordL = wordL - 1
		}
	} else if buf[0] == newline() {
		if cursor != math.MaxUint16 {
			cursor = math.MaxUint16
			send <- genPubEvent()
			wordL = 0
		}
	}
	if buf[0] == 27 {
		if len(buf) == 1 || buf[1] == 0 {
			switchToChanNormal()
			return
		}
		switch buf[2] {
		case byte('A'): //up
			break
			if cursor != math.MaxUint16 {
				cursor = 0
			}
		case 'B': //down
			break
			if cursor != math.MaxUint16 {
				cursor = wordL
			}
		case 'C': //right
			break
			if cursor != wordL {
				cursor = cursor + 1
			}
		case 'D': //left
			break
			if cursor != 0 {
				cursor = cursor - 1
			}
		default:
			switchToChanNormal()
		}
	}
}

func switchToChanNormal() {
	is = chanNormal
	renderHome()
}

func genInitEvent() LRCEvent {
	e := []byte{byte(EventInit), 0, as.color}
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
