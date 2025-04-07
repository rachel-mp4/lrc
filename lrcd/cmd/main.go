package main

import (
	"log"
	"github.com/rachel-mp4/lrc/lrcd/pkg/lrcd"
	"os"
	"sync"
)

func main() {
	server, err := lrcd.NewServer(lrcd.WithTCPPort(927), lrcd.WithWSPort(8080), lrcd.WithLogging(os.Stdout, true))
	if err != nil {
		log.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	server.Start()
	wg.Wait()
}
