package client

import (
	"fmt"
	"slices"
)

func appendTo(m *message, s string, mi int) {
	if !overflowing(m) {
		if lastLineInViewport(m) {
			appendInLineInViewport(m, s)
		} else {
			appendInLineNotInViewport(m, s)
		}
	} else if isLast(m) {
		if lastLineJustInViewport(m) {
			appendEndOfAllLinesJustInViewport(m, s)
		} else if lastLineInViewport(m) {
			appendEndOfAllLinesInViewport(m, s)
		} else {
			appendEndOfAllLinesBelowViewport(m, s)
		}
	} else {
		if lastLineJustInViewport(m) {
			appendEndOfLineJustInViewport(m, s, mi)
		} else if lastLineInViewport(m) {
			appendEndOfLineInViewport(m, s, mi)
		} else if lastLineAboveViewport(m) {
			appendEndOfLineAboveViewport(m, s, mi)
		} else {
			appendEndOfLineBelowViewport(m, s, mi)
		}
	}
}

// appendInLineNotInViewport appends s to an m whose final line is both incomplete and not currently in the viewport
func appendInLineNotInViewport(m *message, s string) {
	m.text = m.text + s
}

// appendInLineInViewport appends s to an m whose final line is both incomplete and currently in the viewport. Renders the change
func appendInLineInViewport(m *message, s string) {
	l := len(m.text)
	cursorGoto(findAbsoluteLineNumberOf(m, l/ts.cpl)-ts.viewportTop, 14+l%ts.cpl)
	appendToLine(line{m, l / ts.cpl}, s)
	m.text = m.text + s
}

// appendEndOfAllLinesInViewport appends s to an m which is the last message and whose final line is complete, and which is in viewport. Renders the change
func appendEndOfAllLinesInViewport(m *message, s string) {
	l := len(m.text)
	m.text = m.text + s
	nln := l / ts.cpl
	nl := line{m, nln}
	lines = append(lines, nl)
	cursorGoto(findAbsoluteLineNumberOf(m, nln)-ts.viewportTop, 1)
	renderLine(nl)
}

// appendEndOfAllLinesJustInViewport appends s to an m which is the last message and whose final line is complete, and which is at the very bottom of the viewport. Renders the change
func appendEndOfAllLinesJustInViewport(m *message, s string) {
	scrollViewportUp(true)
	appendEndOfAllLinesInViewport(m, s)
}

// appendEndOfAllLinesBelowViewport apppends s to an m which is the last message and whose final line is complete, and which is below the viewport
func appendEndOfAllLinesBelowViewport(m *message, s string) {
	l := len(m.text)
	m.text = m.text + s
	nln := l / ts.cpl
	nl := line{m, nln}
	lines = append(lines, nl)
}

// appendEndOfLineAboveViewport appends s to an m which is not the last message and whose final line is complete, and which is above the viewport. Updates all absolute line numbers of messages after mi
func appendEndOfLineAboveViewport(m *message, s string, mi int) {
	appendEndOfLine(m, s, mi)
	ts.viewportTop += 1
	ts.viewportBottom += 1
}

// appendEndOfLineInViewport appends s to an m which is not the last message and whose final line is complete, and which is in the viewport. Renders the change and updates all absolute line numbers of messages after mi
func appendEndOfLineInViewport(m *message, s string, mi int) {
	l := len(m.text)
	m.text = m.text + s
	nln := l / ts.cpl
	nl := line{m, nln}
	nlan := nln + m.absPos
	lines = slices.Insert(lines, nlan, nl)
	updateAbsoluteLineNumbersAfter(mi, 1)
	scrollAllBelow(nlan - ts.viewportTop + 1)
	cursorGoto(nlan-ts.viewportTop+1, 1)
	renderLine(nl)
}

// appendEndOfLineJustInViewport appends s to an m which is not the last message and whose final line is complete, and which is in the viewport while the viewport is just full. Renders the change and updates all line numbers of messages after mi
// TODO investigate bugs
func appendEndOfLineJustInViewport(m *message, s string, mi int) {
	scrollViewportUp(true)
	appendEndOfLineInViewport(m, s, mi)
}

// appendEndOfLineBelowViewport appends s to an m which is not the last message and whose final line is complete, and which is below the viewport. Updates all absolute line numbers of messages after mi
func appendEndOfLineBelowViewport(m *message, s string, mi int) {
	appendEndOfLine(m, s, mi)
}

func appendEndOfLine(m *message, s string, mi int) {
	l := len(m.text)
	m.text = m.text + s
	nln := l / ts.cpl
	nl := line{m, nln}
	lines = slices.Insert(lines, m.absPos+nln, nl)
	updateAbsoluteLineNumbersAfter(mi, 1)
}

func insertInto(m *message, i uint16, s string, mi int) {
	if !overflowing(m) {
		if idxInLastLine(m, i) {
			if lastLineInViewport(m) {
				insertInLastLineInViewport(m, i, s)
			} else {
				insertInLastLineNotInViewport(m, i, s)
			}
		} else {
			if idxInViewport(m, i) {
				insertInNotLastLineInViewport(m, i, s)
			} else if idxAffectingViewport(m, i) {
				insertInNotLastLineAffectingViewport(m, i, s)
			} else {
				insertInNotLastLineNotInViewport(m, i, s)
			}
		}
	} else {
		if idxJustInViewport(m, i) {
			insertOverflowingJustInViewport(m, i, s)
		} else if idxInViewport(m, i) {
			insertOverflowingInViewport(m, i, s)
		} else if idxAffectingViewport(m, i) {
			insertOverflowingAffectingViewport(m, i, s, mi)
		} else if lastLineAboveViewport(m) {
			insertOverflowingAboveViewport(m, i, s, mi)
		} else if messageNotInViewport(m) {
			insertOverflowingBelowViewport(m, i, s)
		} else {
			insertOverflowingEffectOutOfViewport(m, i, s)
		}
	}
}

// insertInLastLineNotInViewport inserts s at i in an m which is the last line of its m and which is not in viewport
func insertInLastLineNotInViewport(m *message, i uint16, s string) {
	m.text = m.text[:int(i)] + s + m.text[int(i):]
}

// insertInLastLineInViewport inserts s at i in an m which is the last line of its m and which is in viewport. Renders the change
func insertInLastLineInViewport(m *message, i uint16, s string) {
	l := lines[m.absPos+len(m.text)/ts.cpl]
	m.text = m.text[:int(i)] + s + m.text[int(i):]
	cursorGoto(findAbsoluteLineNumberOf(m, int(i)/ts.cpl)-ts.viewportTop, 14+int(i)%ts.cpl)
	insertIntoLine(l, s)
}

// insertInNotLastLineNotInViewport inserts s at i in an m where i is not in the last line of its m and which is not in viewport
func insertInNotLastLineNotInViewport(m *message, i uint16, s string) {
	m.text = m.text[:int(i)] + s + m.text[int(i):]
}

// insertInNotLastLineAffectingViewport inserts s at i in an m where i is not in the last line of its m which is not in viewport, but m has at least one line in viewport. Renders the change
func insertInNotLastLineAffectingViewport(m *message, i uint16, s string) {
	m.text = m.text[:int(i)] + s + m.text[int(i):]
	for idx := ts.viewportTop; isALineOf(idx, m); idx++ {
		c := lineFirst(lines[idx])
		cursorGoto(idx-ts.viewportTop+1, 14)
		insertIntoLine(lines[idx], c)
	}
}

func isALineOf(idx int, m *message) bool {
	if idx >= len(lines) {
		return false
	}
	return lines[idx].from == m
}

// insertInNotLastLineInViewport inserts s at i in an m where i is not in the last line of its m which is in viewport (the m is not necessarily entirely contained in the viewport). Renders the chagne
func insertInNotLastLineInViewport(m *message, i uint16, s string) {
	fi := m.absPos + len(m.text)/ts.cpl
	l := lines[fi]
	m.text = m.text[:int(i)] + s + m.text[int(i):]
	cursorGoto(findAbsoluteLineNumberOf(m, int(i)/ts.cpl)-ts.viewportTop, 14+int(i)%ts.cpl)
	insertIntoLine(l, s)
	for idx := fi; isALineOf(idx, m); idx++ {
		c := lineFirst(lines[idx])
		cursorGoto(idx-ts.viewportTop+1, 14)
		insertIntoLine(lines[idx], c)
	}
}

// insertOverflowingAboveViewport inserts s at i in an m that is currently full and which has no lines in viewport
func insertOverflowingAboveViewport(m *message, i uint16, s string, mi int) {
	m.text = m.text[:int(i)] + s + m.text[int(i):]
	os := len(m.text)/ts.cpl
	lli := m.absPos + os
	nll := line{m, os}
	lines = slices.Insert(lines, lli, nll)
	ts.viewportBottom += 1
	ts.viewportTop += 1
	updateAbsoluteLineNumbersAfter(mi, 1)
}

// insertOverflowingAffectingViewport inserts s at i in an m that is currently full and which has at least one line in viewport, but i is not in viewport. Renders the change
func insertOverflowingAffectingViewport(m *message, i uint16, s string, mi int) {
	m.text = m.text[:int(i)] + s + m.text[int(i):]
	os := len(m.text)/ts.cpl
	lli := m.absPos + os
	nll := line{m, os}
	lines = slices.Insert(lines,lli,nll)
	
}

// insertOverflowingInViewport inserts s at i in an m that is currently full and where i is in viewport (the m is not necessarily entirely contained within viewport). Renders the change
func insertOverflowingInViewport(m *message, i uint16, s string) {
	fmt.Printf("logic err")
}

// insertOverflowingJustInViewport inserts s at i in an m that is currently full and where i is in a just full viewport (the m is not necessarily entirely contained within viewport). Renders the change
func insertOverflowingJustInViewport(m *message, i uint16, s string) {
	fmt.Printf("logic err")
}

// insertOverflowingEffectOutOfViewport inserts s at i in an m that is currently full and where there are more messages after m, and this occurs on the last line of the viewport
func insertOverflowingEffectOutOfViewport(m *message, i uint16, s string) {
	fmt.Printf("logic err")
}

// insertOverflowingBelowViewport inserts s at i in an m that is currently full and where i is below viewport
func insertOverflowingBelowViewport(m *message, i uint16, s string) {
	fmt.Printf("logic err")
}

// TODO: late insert logic
func lateInsertInto(m *message, i uint16, s string, mi int) {
	fmt.Printf("logic error")
}

func messageNotInViewport(m *message) bool {
	return findFLInViewport(m) == -1
}

func idxJustInViewport(m *message, idx uint16) bool {
	idxLine := int(idx) / ts.cpl
	lc := len(m.text) / ts.cpl
	return idxLine == lc && lastLineJustInViewport(m)
}

func idxAffectingViewport(m *message, idx uint16) bool {
	idxLine := int(idx) / ts.cpl
	fliv := findFLInViewport(m)
	return fliv > idxLine
}

func idxInViewport(m *message, idx uint16) bool {
	idxLine := int(idx) / ts.cpl
	return lnumInViewport(idxLine)
}

func lnumInViewport(lnum int) bool {
	return lnum >= ts.viewportTop && lnum <= ts.viewportBottom
}

// idxInLastLine returns true if idx is in the last line
func idxInLastLine(m *message, idx uint16) bool {
	l := len(m.text)
	lc := l / ts.cpl
	return lc*ts.cpl <= int(idx)
}

// isLast returns true if m is the last message
func isLast(m *message) bool {
	return m == msgs[len(msgs)-1]
}

// lastLineInViewport returns true if the last line of m is in the viewport
func lastLineInViewport(m *message) bool {
	llpos := m.absPos + len(m.text)/ts.cpl
	return lnumInViewport(llpos)
}

// lastLineJustInViewport returns true if the last line of m is the last line of the viewport
func lastLineJustInViewport(m *message) bool {
	llpos := m.absPos + len(m.text)/ts.cpl
	return llpos == ts.viewportBottom
}

func lastLineAboveViewport(m *message) bool {
	llpos := m.absPos + len(m.text)/ts.cpl
	return llpos < ts.viewportTop
}

// overflowing returns true if m is currently full
func overflowing(m *message) bool {
	l := len(m.text)
	lc := l / ts.cpl
	cill := l % ts.cpl
	return cill == 0 && lc != 0
}
