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
	fmt.Println("if you're reading this then your terminal does not natively support escape codes. LunaRC will not be an enjoyable experience for you until you install git bash or some other cool terminal emulator. or there is a bug somewhere in my codes. contact me on discord UGH or email me rachel@moth11.net or you can even open an issue on github if you're feeling fancy")
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
	fmt.Print("\033[2J")
}
