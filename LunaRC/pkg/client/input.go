package client

import (
	"fmt"
	"lrc"
	"math"
	"net"
	"os"
	"strconv"
)

type inputState = int

const (
	menuNormal inputState = iota
	menuInsert
	chanNormal
	chanInsert
)

type cmdState = int

const (
	none cmdState = iota
	name
	color
)

var (
	cs               = none
	is               = menuNormal
	cmdBuffer        = ""
	cursor    uint16 = math.MaxUint16
	wordL     uint16 = 0
)

func AcceptInput() {
	buf := make([]byte, 10)
	quit := make(chan struct{})
	send := make(chan events.LRCEvent)
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

func inputMenuNormal(buf []byte, quit chan struct{}, send chan events.LRCEvent) net.Conn {
	switch buf[0] {
	case 10, 13:
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

func inputMenuInsert(buf []byte, quit chan struct{}, send chan events.LRCEvent) {
	switch buf[0] {
	case 10, 13:
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

func evaluateCommandBuffer(quit chan struct{}, send chan events.LRCEvent) {
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

func inputChanNormal(buf []byte, quit chan struct{}, send chan events.LRCEvent) {
	switch cs {
	case none:
		switch buf[0] {
		case 99:
			setMyColor()
		case 100:
			dumpCmdLog()
		case 105:
			switchToChanInsert()
			cursorHome()
		case 106:
			scrollViewportDown(false)
		case 107:
			scrollViewportUp(false)
		case 110:
			setName()
		case 113:
			close(quit)
		case 114:
			rerender()
		}
	case name:
		if buf[0] > 31 && buf[0] < 127 {
			if len(cmdBuffer) < 12 {
				cmdBuffer = cmdBuffer + string(buf[0])
				renderPartialName()
			}
		} else if buf[0] == 127 {
			if cmdBuffer != "" {
				cmdBuffer = string(cmdBuffer[:len(cmdBuffer)-1])
				renderPartialName()
			}
		} else if buf[0] == 10 || buf[0] == 13 {
			setNameTo()
		}
	case color:
		if buf[0] > 31 && buf[0] < 127 {
			if len(cmdBuffer) < 3 {
				cmdBuffer = cmdBuffer + string(buf[0])
				renderPartialColor()
			}
		} else if buf[0] == 127 {
			if cmdBuffer != "" {
				cmdBuffer = string(cmdBuffer[:len(cmdBuffer)-1])
				renderPartialColor()
			}
		} else if buf[0] == 10 || buf[0] == 13 {
			setMyColorTo()
		}
	}
}

func setName() {
	cs = name
	renderPartialName()
}

func setNameTo() {
	if len(cmdBuffer) > 12 {
		cmdBuffer = cmdBuffer[:12]
	}
	as.name = cmdBuffer
	cmdBuffer = ""
	cs = none
	rerender()
}

func renderPartialName() {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	cursorGoto(ts.h, 1)
	clearLine()
	homeStyle()
	fmt.Printf("%s", cmdBuffer)
	renderPing(true)
}

func setMyColor() {
	cs = color
	renderPartialColor()
}

func renderPartialColor() {
	c, _ := strconv.Atoi(cmdBuffer)
	if cmdBuffer == "" {
		c = 15
	}
	as.color = uint8(c)
	fmtMu.Lock()
	defer fmtMu.Unlock()
	cursorGoto(ts.h, 1)
	clearLine()
	homeStyle()
	fmt.Printf("%s", cmdBuffer)
	renderPing(true)
}

func setMyColorTo() {
	c, _ := strconv.Atoi(cmdBuffer)
	if cmdBuffer == "" {
		c = 15
	}
	as.color = uint8(c)
	cmdBuffer = ""
	cs = none
	rerender()
}

func switchToChanInsert() {
	is = chanInsert
	renderHome(false)
}

func inputChanInsert(buf []byte, quit chan struct{}, send chan events.LRCEvent) {
	if (buf[0] < 127) && (buf[0] > 31) {
		if cursor == math.MaxUint16 {
			cursor = 0
			send <- events.GenInitEvent(as.color,as.name)
			wordL = 0
			initMyMsg(as.color, as.name)
		}
		send <- events.GenInsertEvent(cursor, string(buf[0]))
		insertIntoMyMsg(cursor, string(buf[0]))
		cursor = cursor + 1
		wordL = wordL - 1

	} else if buf[0] == 127 {
		if cursor > 0 && cursor != math.MaxUint16 {
			send <- events.GenDeleteEvent(cursor)
			deleteFromMyMessage(cursor)
			cursor = cursor - 1
			wordL = wordL - 1
		}
	} else if buf[0] == 10 || buf[0] == 13 {
		if cursor != math.MaxUint16 {
			cursor = math.MaxUint16
			send <- events.GenPubEvent()
			pubMyMsg()
			wordL = 0
		}
	}
	if buf[0] == 27 {
		switchToChanNormal()
		return
		// if len(buf) == 1 || buf[1] == 0 {

		// }
		// switch buf[2] {
		// case byte('A'): //up
		// 	break
		// 	if cursor != math.MaxUint16 {
		// 		cursor = 0
		// 	}
		// case 'B': //down
		// 	break
		// 	if cursor != math.MaxUint16 {
		// 		cursor = wordL
		// 	}
		// case 'C': //right
		// 	break
		// 	if cursor != wordL {
		// 		cursor = cursor + 1
		// 	}
		// case 'D': //left
		// 	break
		// 	if cursor != 0 {
		// 		cursor = cursor - 1
		// 	}
		// default:
		// 	switchToChanNormal()
		// }
	}
}

func switchToChanNormal() {
	is = chanNormal
	renderHome(false)
}
