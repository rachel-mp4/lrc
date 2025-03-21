package client

import (
	"fmt"
	"strings"
)

func moth() {
	fmt.Println("  %%%%%%%\r\n %%%%%%%%%%  %%%            %   %           %%%%%%%%\r\n   %%%%%%%%%%%%%%%%%        %%%%     %%%%%%%%%%%%%%%%%%\r\n     %%%%%%%%%%%%%%%%%%%%%%% %% %%%%%%%%%%%%%%%%%%%%%%%%%%\r\n       %%%%%%%%%%%%%%%%%%%%% %% %%%%%%%%%%%%%%%%%%%%%%%\r\n            %%%%%%%%%%%%%%%% %% %%%%%%%%%%%%%%%%%%%%%\r\n         %%%%%%%%%%%%%%%%%%% %% %%%%%%%%%%%%%%\r\n       %%%%%%%%%%%%%%%%%%%%%    %%%%%%%%%%%%%%%%%%\r\n          %%%%%%%%%%%%         %%%%%%%%%%%%%%%%\r\n             %%%%%               %%%%%%%%%\r\n                                    %")
}

func appendToLine(l line, s string) {
	if l.from.active {
		setColor(l.from.user.c)
		inverted()
	}
	fmt.Print(s)
	if l.from.active {
		resetStyles()
	}
}

func insertIntoLine(l line, s string) {
	insertCharacter()
	appendToLine(l, s)
}

func renderLine(l line) {
	resetStyles()
	nspaces := 12 - len(l.from.user.name)
	fmt.Print(strings.Repeat(" ", nspaces))
	if l.num == 0 {
		setColor(l.from.user.c)
		if l.from.active {
			inverted()
		}
		fmt.Print(l.from.user.name)
	}
	resetStyles()
	fmt.Print(" ")
	if l.from.active {
		setColor(l.from.user.c)
		inverted()
	}
	cursorBeginLine()
	fmt.Print(lineContents(l))
}

func scrollUpAbove(idx int) {
	if idx == 1 {
		cursorHome()
		clearLine()
		return
	} else if idx >= ts.h {
		idx = ts.h - 1
	}
	fmt.Printf("\033[1;%dr", idx)
	scrollUp()
	setupScrollRegion()
}

func scrollDownAbove(idx int) {
	if idx == 1 {
		cursorHome()
		clearLine()
		return
	} else if idx >= ts.h {
		idx = ts.h - 1
	}
	fmt.Printf("\033[1;%dr", idx)
	scrollDown()
	setupScrollRegion()
} 

func scrollDownBelow(idx int) {
	if idx == ts.h-1 {
		cursorGoto(idx, 1)
		clearLine()
		return
	}
	fmt.Printf("\033[%d;%dr", idx, ts.h-1)
	cursorGoto(idx, 1)
	scrollDown()
	setupScrollRegion()
}

func scrollUpBelow(idx int) {
	if idx == ts.h-1 {
		cursorGoto(idx, 1)
		clearLine()
		return
	}
	fmt.Printf("\033[%d;%dr", idx, ts.h-1)
	cursorGoto(idx, 1)
	scrollUp()
	setupScrollRegion()
}

func insertCharacter() {
	fmt.Print("\033[1@")
}

func setupScrollRegion() {
	fmt.Printf("\033[1;%dr", ts.h-1)
}

func lineContents(l line) string {
	ncpl := (ts.w - 13)
	nlpm := len(l.from.text) / ncpl

	if nlpm > l.num {
		return l.from.text[l.num*ncpl : (l.num+1)*ncpl]
	}
	return l.from.text[l.num*ncpl:]
}

func lineFirst(l line) string {
	ncpl := (ts.w - 13)
	return string(l.from.text[l.num*ncpl])
}

func clearLine() {
	fmt.Printf("\033[2K")
}

func inverted() {
	fmt.Printf("\033[7m")
}

func clearAll() {
	fmt.Print("\033[2J")
	cursorHome()
}

func cursorGoto(row, col int) {
	fmt.Printf("\033[%d;%dH", row, col)
}

func cursorHome() {
	fmt.Print("\033[H")
}

// scrollUp scrolls the region down, adding a new line at the top
func scrollDown() {
	fmt.Print("\033[1T")
}

// scrollUp scrolls the region up, adding a new line at the bottom
func scrollUp() {
	fmt.Print("\033[1S")
}

func cursorFullLeft() {
	fmt.Print("\033[1G")
}

func cursorBeginLine() {
	fmt.Print("\033[14G")
}

func resetStyles() {
	fmt.Print("\033[0m")
}

func setColor(colorCode uint8) {
	fmt.Printf("\033[38;5;%dm", colorCode)
}

func faint() {
	fmt.Print("\033[2m")
}

func cursorBar() {
	fmt.Print("\033[5 q")
}

func cursorBlock() {
	fmt.Print("\033[1 q")
}
