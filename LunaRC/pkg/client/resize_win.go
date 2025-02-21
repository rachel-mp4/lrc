//go:build windows
package client

import (
	"os"
	"time"
	"golang.org/x/term"
)

func listenForResize(resizeChan chan struct{}) {
	go func() {
		lw, lh, _ := term.GetSize(int(os.Stdout.Fd()))
		for {
			time.Sleep(500*time.Millisecond)
			w, h, _ := term.GetSize(int(os.Stdout.Fd()))
			if w != lw || h != lh {
				resizeChan <- struct{}{}
				lw, lh = w, h
			}
			
		}
	}()
}