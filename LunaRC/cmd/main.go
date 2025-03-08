package main

import (
	"LunaRC/pkg/client"
	"os"
	"fmt"
	"golang.org/x/term"
)

var (
	oldState *term.State
)

func main() {
	setTerminal()
	defer resetTerminal()
	client.InitView()
	client.AcceptInput()
}

func setTerminal() {
	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	oldState = old
	fmt.Print("\033[?1049h") //alt buffer on
}

func resetTerminal() {
	fmt.Print("\033[?1049l") //alt buffer off
	term.Restore(int(os.Stdin.Fd()), oldState)
}
