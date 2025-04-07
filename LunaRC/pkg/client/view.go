package client

import (
	"fmt"
	events "lrc"
	"os"
	"sync"

	"golang.org/x/term"
)

var (
	as         appState
	ts         terminalState
	idToMsgIdx = make(map[uint32]int)
	msgs       = make([]*message, 0)
	lines      []line
	fmtMu      sync.Mutex
	cmdLog     []events.LRCEvent
	myMsgIdx   int
)

type appState struct {
	url              string
	welcome          string
	ping             int
	currentConnected int
	color            uint8
	name             string
}

type terminalState struct {
	w              int
	h              int
	viewportTop    int
	viewportBottom int
	cpl            int //characters per line
}

type user struct {
	c    uint8
	name string
}

type message struct {
	user   *user
	text   string
	active bool
	absPos int
}

type line struct {
	from *message
	num  int
}

func InitView() {
	recallApplicationState()
	getTerminalSize()
	fmtMu.Lock()
	renderSplash()
	fmtMu.Unlock()
	resizeChan := make(chan struct{})
	listenForResize(resizeChan)
	go resize(resizeChan)
}

func renderSplash() {
	cursorBlock()
	clearAll()
	setColor(as.color)
	moth()
	resetStyles()
	faint()
	fmt.Print("\r\n  ...and now you're using LunaRC,\r\n     an LRC client made by moth11...")
	resetStyles()
}

func initChan() {
	fmtMu.Lock()
	defer fmtMu.Unlock()
	clearAll()
	renderHome(true)
	lines = make([]line, 0)
}

// TODO store and read from file
func recallApplicationState() {
	as = appState{"moth11.net", "beep", 0, 0, 13, "wanderer"}
}

func getTerminalSize() {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		panic(err)
	}
	ts = terminalState{width, height, 0, height - 1, width - 13}
}

func resize(resizeChan chan struct{}) {
	for {
		select {
		case <-resizeChan:
			getTerminalSize()
			fixAfterResize()
		}
	}
}

// TODO recalculate line breaks on change width
func fixAfterResize() {
	if is == menuInsert || is == menuNormal {
		cursorHome()
		renderSplash()
	} else {
		rerender()
	}
}

func setCurrentConnected(n int) {
	as.currentConnected = n
	renderCurrentConnected(false)
}

func setPingTo(ms int) {
	if ms > 999 {
		ms = 999
	}
	as.ping = ms
	renderPing(false)
}

func setWelcomeMessage(s string) {
	as.welcome = s
	if spaceForWelcome() {
		renderWelcomeMessage(false)
	}
}

func spaceForWelcome() bool {
	return (16 + len(as.welcome) + len(as.url)) <= ts.w
}

func homeStyle() {
	setColor(as.color)
	if is == chanNormal && cs != color {
		faint()
	} else {
		inverted()
	}
}

// renderPing renders the ping in the bottom right
func renderPing(alreadyLocked bool) {
	if !alreadyLocked {
		fmtMu.Lock()
		defer fmtMu.Unlock()
	}

	cursorGoto(ts.h, ts.w-4)
	homeStyle()
	fmt.Printf("%3dms", as.ping)
	resetStyles()
}

func renderCurrentConnected(alreadyLocked bool) {
	if !alreadyLocked {
		fmtMu.Lock()
		defer fmtMu.Unlock()
	}

	cursorGoto(ts.h, ts.w-6)
	homeStyle()
	if as.currentConnected < 0 {
		fmt.Print("--")
	} else {
		fmt.Printf("%d", as.currentConnected)
	}
	resetStyles()
}

func renderWelcomeMessage(alreadyLocked bool) {
	if !alreadyLocked {
		fmtMu.Lock()
		defer fmtMu.Unlock()
	}

	cursorGoto(ts.h, ts.w-7-len(as.welcome))
	homeStyle()
	fmt.Print(as.welcome)
	resetStyles()
}

// renderUrl renders the url, with the protocol that the connection is occuring over
func renderUrl(alreadyLocked bool) {
	if !alreadyLocked {
		fmtMu.Lock()
		defer fmtMu.Unlock()
	}

	homeStyle()
	fmt.Printf("lrc://%s/", as.url)
	resetStyles()
}

// renderHome renders the bottom row
func renderHome(alreadyLocked bool) {
	if !alreadyLocked {
		fmtMu.Lock()
		defer fmtMu.Unlock()
	}

	fmt.Printf("\033[%d;1H", ts.h)
	homeStyle()
	fmt.Printf("%-"+fmt.Sprintf("%d", ts.w)+"s", " ")
	cursorFullLeft()
	renderUrl(true)
	if spaceForWelcome() {
		renderWelcomeMessage(true)
	}
	renderCurrentConnected(true)
	renderPing(true)
	if is == chanInsert {
		cursorBar()
	} else {
		cursorBlock()
	}
}

// rerender should rerender every line in viewport correctly
// TODO investigate bugs with scrolling
func rerender() {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	clearAll()
	cursorHome()
	for idx := 1; idx < ts.h; idx++ {
		if idx+ts.viewportTop > len(lines) {
			break
		}
		cursorGoto(idx, 1)
		renderLine(lines[idx-1+ts.viewportTop])
	}
	renderHome(true)
}

// lcount counts how many lines are in a message, 1-indexed
func (m *message) lCount() int {
	return len(m.text)/ts.cpl + 1
}

// initMSg initializes a message from a user, and renders the initial line.
func initMsg(id uint32, color uint8, name string, alreadyLocked bool, isFromMe bool) {
	if !alreadyLocked {
		fmtMu.Lock()
		defer fmtMu.Unlock()
	}

	if isFromMe {
		idToMsgIdx[id] = -1
		return
	}

	idToMsgIdx[id] = len(msgs)
	initAMsg(color, name)
}

func initMyMsg(color uint8, name string) {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	myMsgIdx = len(msgs)
	initAMsg(color, name)
}

func initAMsg(color uint8, name string) {
	u := user{color, name}
	abs := 0
	if len(msgs) != 0 {
		pm := msgs[len(msgs)-1]
		abs = pm.absPos + pm.lCount()
	}
	m := message{&u, "", true, abs}
	l := line{&m, 0}
	msgs = append(msgs, &m)
	appendAndRender(l)
}

// appendAndRender is called whenever a new line is appended to the end of lines
func appendAndRender(l line) {
	if viewportFull() {
		lines = append(lines, l)
		setupScrollRegion()
		cursorHome()
		scrollUp()
		cursorGoto(ts.h-1, 1)
		renderLine(l)
		ts.viewportTop = ts.viewportTop + 1
		ts.viewportBottom = ts.viewportBottom + 1
	} else if len(lines) > ts.viewportBottom { //the viewport is overfull if the length of lines is greater than the viewport bottom
		lines = append(lines, l)
	} else { //the viewport is not full
		lines = append(lines, l)
		cursorGoto(len(lines)-ts.viewportTop, 1)
		renderLine(l)
	}
}

// the viewport is full if the top of the viewport + the height of the viewport = the length of lines
func viewportFull() bool {
	return ts.viewportTop+ts.h-1 == len(lines)
}

func addToCmdLog(e events.LRCEvent) {
	cmdLog = append(cmdLog, e)
}

func dumpCmdLog() {
	clearAll()
	cursorHome()
	for _, v := range cmdLog {
		fmt.Printf("%x\r\n", v)
	}
}

func pubMsg(id uint32) {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	mi, ok := idToMsgIdx[id]
	if !ok {
		return
	}
	if mi < 0 { //this is a message that originated from me
		return
	}

	pubAMsg(mi)
}

func pubMyMsg() {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	pubAMsg(myMsgIdx)
}

func pubAMsg(mi int) {
	m := msgs[mi]
	m.active = false
	fliv := findFLInViewport(m)
	if fliv == -1 {
		return
	} else {
		for idx := fliv; checkLinesIdxIsM(idx, m); idx++ {
			cursorGoto(idx-ts.viewportTop+1, 1)
			renderLine(lines[idx])
		}
	}
}

// checkLinesIdxIsM takes an absolute line number, and determines if it belongs to m. 
// If there is no line, with abspos=idx, this returns false.
func checkLinesIdxIsM(idx int, m *message) bool {
	if idx >= len(lines) {
		return false
	}
	return lines[idx].from == m
}

// findFLInViewport returns the least absolute position of a line in viewport that belongs to m
func findFLInViewport(m *message) int {
	for i := ts.viewportTop; i <= ts.viewportBottom; i++ {
		if i >= len(lines) {
			return -1
		}
		if lines[i].from == m {
			return i
		}
	}
	return -1
}

func findAbsoluteLineNumberOf(msg *message, lnum int) int {
	return msg.absPos + lnum + 1
}

// updateAbsoluteLineNumbersAfter takes a message index idx, and updates all messages after (and not including) the idx by an amount
func updateAbsoluteLineNumbersAfter(idx int, by int) {
	for i := idx + 1; i < len(msgs); i++ {
		msgs[i].absPos += by
	}
}

// move cursor down from vim
// TODO investigate bug on rerender, possibly caused by scrolling viewport
func scrollViewportUp(alreadyLocked bool) {
	if !alreadyLocked {
		fmtMu.Lock()
		defer fmtMu.Unlock()
	}

	if ts.viewportTop == len(lines) {
		return
	}
	ts.viewportTop += 1
	ts.viewportBottom += 1
	setupScrollRegion()
	cursorHome()
	scrollUp()
	if len(lines) > ts.viewportBottom {
		cursorGoto(ts.h-1, 1)
		renderLine(lines[ts.viewportBottom])
	}
}

// move cursor up from vim
func scrollViewportDown(alreadyLocked bool) {
	if !alreadyLocked {
		fmtMu.Lock()
		defer fmtMu.Unlock()
	}

	if ts.viewportTop == 0 {
		return
	}
	ts.viewportTop -= 1
	ts.viewportBottom -= 1
	setupScrollRegion()
	cursorHome()
	scrollDown()
	renderLine(lines[ts.viewportTop])
}

func insertIntoMsg(id uint32, idx uint16, s string) {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	mi, exists := idToMsgIdx[id]
	if !exists {
		initMsg(id, 66, "???", true, false)
		mi = idToMsgIdx[id]
	}
	if mi < 0 {
		return
	}

	insertIntoAMsg(mi, idx, s)
}

func insertIntoMyMsg(idx uint16, s string) {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	mi := myMsgIdx
	insertIntoAMsg(mi, idx, s)
}

func insertIntoAMsg(mi int, idx uint16, s string) {
	m := msgs[mi]
	l := len(m.text)
	if l == int(idx) {
		appendTo(m, s, mi)
	} else if l > int(idx) {
		insertInto(m, idx, s, mi)
	} else {
		lateInsertInto(m, idx, s, mi)
	}
}

func deleteFromMessage(id uint32, idx uint16) {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	mi, exists := idToMsgIdx[id]
	if !exists {
		initMsg(id, 66, "???", true, false)
		mi = idToMsgIdx[id]
	}
	if mi < 0 {
		return
	}

	deleteFromAMessage(mi, idx)
}

func deleteFromMyMessage(idx uint16) {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	mi := myMsgIdx
	deleteFromAMessage(mi, idx)
}

func deleteFromAMessage(mi int, idx uint16) {
	m := msgs[mi]
	l := len(m.text)
	if l == int(idx) {
		truncFrom(m, mi)
	} else if l > int(idx) {

	}
}

func connectionFailure(to string, err error) {
	fmt.Print("\r\nFailed to connect")
	if to != "" {
		fmt.Printf(" to %s", to)
	}
	fmt.Print("\r\n" + err.Error())
	panic(err)
}
