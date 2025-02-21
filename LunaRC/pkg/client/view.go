package client

import (
	"fmt"
	"golang.org/x/term"
	"os"
)

var (
	oldState *term.State
	width    int
	height   int
)

func SetTerminal() {
	old, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	oldState = old
	altBufferOn()
	clearAll()
	purple()
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
                                    %`)
	resetStyles()
	faint()
	fmt.Println(`  ...and now you're using LunaRC,
     an LRC client made by moth11...`)
	getTerminalSize()
	fmt.Printf("%dx%d", width, height)
	resetStyles()
	waitForInput()

	resizeChan := make(chan struct{})
	listenForResize(resizeChan)
	for {
		select {
		case <- resizeChan:
			getTerminalSize()
			fmt.Printf("%dx%d", width, height)
		}
	}
}

func ResetTerminal() {
	altBufferOff()
	term.Restore(int(os.Stdin.Fd()), oldState)
}

func ConnectionSuccess(to string) {
	fmt.Print("\nConnected")
	if to != "" {
		fmt.Printf(" to %s", to)
	}
	waitForInput()
}

func ConnectionFailure(to string, err error) {
	fmt.Print("\nFailed to connect")
	if to != "" {
		fmt.Printf(" to %s", to)
	}
	fmt.Print("\n" + err.Error())
	waitForInput()
}

func getTerminalSize() {
	var err error
	width, height, err = term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		panic(err)
	}
}

func altBufferOn() {
	fmt.Print("\033[?1049h")
}

func altBufferOff() {
	fmt.Print("\033[?1049l")
}

func purple() {
	fmt.Print("\033[38;5;55m")
}

func clearAll() {
	fmt.Print("\033[2J")
}

func resetStyles() {
	fmt.Print("\033[0m")
}

func faint() {
	fmt.Print("\033[2m")
}

func waitForInput() {
	buf := make([]byte, 10)
	_, err := os.Stdin.Read(buf)
	if err != nil {
		panic(err)
	}
}
