//go:build !windows
package client

import (
	"os"
	"os/signal"
	"golang.org/x/sys/unix"
)

func listenForResize(resizeChan chan struct{}) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, unix.SIGWINCH)

	go func() {
		for range sigChan {
			resizeChan <- struct{}{}
		}
	}()
}