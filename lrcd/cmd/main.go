package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/rachel-mp4/lrc/lrcd/pkg/lrcd"
)

func main() {
	ec := make(chan struct{})
	server, err := lrcd.NewServer(
		lrcd.WithTCPPort(927), 
		lrcd.WithLogging(os.Stdout, true), 
		lrcd.WithEmptyChannel(ec),
		lrcd.WithEmptySignalAfter(2*time.Second))
	if err != nil {
		log.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		<-ec
		fmt.Println("still empty")
	}()
	go func() {
		time.Sleep(7*time.Second)
		wg.Done()
	}()
	server.Start()
	wg.Wait()
	server.Stop()
	time.Sleep(7*time.Second)
	fmt.Println("no errors!")
}
