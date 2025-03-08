package client

import (
	"fmt"
	"os"
	"slices"
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
)

type appState struct {
	url     string
	welcome string
	ping    int
	color   uint8
	name    string
}

type terminalState struct {
	w              int
	h              int
	viewportTop    int
	viewportBottom int
	cpl            int
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
	cursorDisplayInsert()
	clearAll()
	setColor(as.color)
	moth()
	resetStyles()
	faint()
	fmt.Println(`  ...and now you're using LunaRC,
	an LRC client made by moth11...`)
	resetStyles()
	fmtMu.Unlock()
	resizeChan := make(chan struct{})
	listenForResize(resizeChan)
	go resize(resizeChan)
}

func initChan() {
	clearAll()
	renderHome()
	lines = make([]line, 0)
}

// TODO store and read from file
func recallApplicationState() {
	as = appState{"moth11.net", "", 0, 13, "wanderer"}
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

// TODO fix state after resize
func fixAfterResize() {

}

func setPingTo(ms int) {
	if ms > 999 {
		ms = 999
	}
	as.ping = ms
	renderPing()
}

func setWelcomeMessage(s string) {
	as.welcome = s
	if spaceForWelcome() {
		renderWelcomeMessage()
	}
}

func spaceForWelcome() bool {
	return (14 + len(as.welcome) + len(as.url)) <= ts.w
}

func homeStyle() {
	setColor(as.color)
	if is == chanNormal {
		faint()
	} else {
		inverted()
	}
}

func renderPing() {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	cursorGoto(ts.h, ts.w-4)
	homeStyle()
	fmt.Printf("%3dms", as.ping)
	resetStyles()
}

func renderWelcomeMessage() {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	cursorGoto(ts.h, ts.w+8)
	homeStyle()
	fmt.Print(as.welcome)
	resetStyles()
}

func renderUrl() {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	homeStyle()
	fmt.Printf("lrc://%s/", as.url)
	resetStyles()
}

func renderHome() {
	fmtMu.Lock()
	fmt.Printf("\033[%d;1H", ts.h)
	homeStyle()
	fmt.Printf("%-"+fmt.Sprintf("%d", ts.w)+"s", " ")
	cursorFullLeft()
	fmtMu.Unlock()
	renderUrl()
	if spaceForWelcome() {
		renderWelcomeMessage()
	}
	renderPing()
}

func (m *message) lCount() int {
	return len(m.text)/ts.cpl + 1
}

// initMSg initializes a message from a user, and renders the initial line.
func initMsg(id uint32, u user, alreadyLocked bool) {
	if !alreadyLocked {
		fmtMu.Lock()
		defer fmtMu.Unlock()
	}

	abs := 0
	if len(msgs) != 0 {
		pm := msgs[len(msgs)-1]
		abs = pm.absPos + pm.lCount()
	}
	m := message{&u, "", true, abs}
	l := line{&m, 0}
	idToMsgIdx[id] = len(msgs)
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

// insertAndRender is called whenever a new line is inserted into the middle of lines 
func insertAndRender(l line, midx int) {
	if viewportFull() { //want to scroll up all the lines above the inserted line, if it's currently visible
		lines = slices.Insert(lines, l.from.absPos + l.num)
		cursorHome()
		scrollAllAbove(l.from.absPos + l.num - ts.viewportTop)
		cursorGoto(l.from.absPos + l.num - ts.viewportTop, 1)
		renderLine(l)
		ts.viewportTop = ts.viewportTop + 1
		ts.viewportBottom = ts.viewportTop + 1
	} else if len(lines) > ts.viewportBottom {

	}
}

// the viewport is full if the top of the viewport + the height of the viewport = the length of lines
func viewportFull() bool {
	return ts.viewportTop+ts.h-1 == len(lines)
}

func pubMsg(id uint32) {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	m := msgs[idToMsgIdx[id]]
	m.active = false
	fliv := findFLInViewport(id)
	if fliv == -1 {
		return
	} else {
		for idx := fliv; checkLinesIdxIsM(idx, m); idx++ {
			cursorGoto(idx-ts.viewportTop+1, 1)
			renderLine(lines[idx])
		}
	}
}

func checkLinesIdxIsM(idx int, m *message) bool {
	if idx >= len(lines) {
		return false
	}
	return lines[idx].from == m
}

func findFLInViewport(id uint32) int {
	m := msgs[idToMsgIdx[id]]
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

func updateAbsoluteLineNumbersAfter(idx, by int) {
	for i := idx; i < len(msgs); i++ {
		msgs[i].absPos += by
	}
}

// move cursor down from vim
func scrollViewportUp() {
	fmtMu.Lock()
	defer fmtMu.Unlock()

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
func scrollViewportDown() {
	fmtMu.Lock()
	defer fmtMu.Unlock()

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
		initMsg(id, user{66, "???"}, true)
		mi = idToMsgIdx[id]
	}
	m := msgs[mi]
	l := len(m.text)
	clnum := (l - 1) / ts.cpl
	nlnum := (l) / ts.cpl
	fliv := findFLInViewport(id)
	if fliv == -1 { //post not in viewport
		a := m.text[:idx]
		b := m.text[idx:]
		nt := a + s + b
		m.text = nt
		if clnum != nlnum { //adds a newline, so we have to increase all subsequent messages lines cumsum
			updateAbsoluteLineNumbersAfter(mi + 1, 1)
			if m.absPos < ts.viewportTop { //occurs above our viewport, so we need to shift our viewport down to account
				ts.viewportTop += 1
				ts.viewportBottom += 1
			}
			nl := line{m, nlnum}
			bp := msgs[mi+1].absPos - 1
			lines = slices.Insert(lines, bp, nl)
		}
		return
	}
	if l == int(idx) { //append
		if clnum == nlnum { //no newline issues
			appendInViewWithNoNewline(m, clnum, s)
		} else {
			if mi == len(msgs) - 1 {
				appendInViewWithNewlineLastMessage(m, nlnum, s)
			} else {
				appendInViewWithNewlineNonLast(m, mi, nlnum, s)
			}
			
		}
	} else if l < int(idx) { //insert
		if clnum == nlnum {
			insertInViewWithNoNewline(m, idx, s)
		}

	} else { //late insert
	}
}

func insertInViewWithNoNewline(m *message, idx uint16, s string) {

}

func deleteFromMessage(id uint32, idx uint16) {
	fmtMu.Lock()
	defer fmtMu.Unlock()

	mi, exists := idToMsgIdx[id]
	if !exists {
		initMsg(id, user{66, "???"}, true)
		mi = idToMsgIdx[id]
	}
	m := msgs[mi]
	l := len(m.text)
	clnum := (l - 1) / ts.cpl
	nlnum := (l - 2) / ts.cpl
	fliv := findFLInViewport(id)
	if fliv == -1 { //post not in viewport
		a := m.text[:idx - 1]
		b := m.text[idx:]
		nt := a + b
		m.text = nt
		if clnum != nlnum { //subtracts a newline, so we have to decrease all subsequent messages lines cumsum
			updateAbsoluteLineNumbersAfter(mi + 1, -1)
			if m.absPos < ts.viewportTop { //occurs above our viewport, so we need to shift our viewport down to account
				ts.viewportTop -= 1
				ts.viewportBottom -= 1
			}
			bp := msgs[mi+1].absPos - 1
			lines = slices.Delete(lines, bp, nlnum) //i think this is wrong
		}
		return
	}
	if l == int(idx) {
		if clnum == nlnum {
			truncFromViewWithNoNewline(m, clnum)
		}
	}
}

func truncFromViewWithNoNewline(m *message, on int) {
	cursorGoto(findAbsoluteLineNumberOf(m, on), 14 + (len(m.text))%ts.cpl)
	resetStyles()
	fmt.Printf("\b \b")
	m.text = m.text[:len(m.text)-1]
}

func truncFromViewWithNewline(m *message, on int) {
	m.text = m.text[:len(m.text)-1]

}

func appendInViewWithNoNewline(m *message, on int, s string) {
	cursorGoto(findAbsoluteLineNumberOf(m, on) - ts.viewportTop, 14+(len(m.text))%ts.cpl)
	appendToLine(line{m, on}, s)
	m.text = m.text + s
}

func appendInViewWithNewlineLastMessage(m *message, on int, s string) {
	m.text = m.text + s
	appendAndRender(line{m, on})
}

func appendInViewWithNewlineNonLast(m *message, midx int, on int, s string) {
	m.text = m.text + s
	insertAndRender(line{m, on}, midx)
}

func connectionFailure(to string, err error) {
	fmt.Print("\nFailed to connect")
	if to != "" {
		fmt.Printf(" to %s", to)
	}
	fmt.Print("\n" + err.Error())
	panic(err)
}
