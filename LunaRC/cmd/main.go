package main

import (
	"LunaRC/pkg/client"
)

var (
	url string
)

func main() {
	client.SetTerminal()
	defer client.ResetTerminal()

	url = ""
	conn, err := client.Dial(url)
	if (err != nil) {
		client.ConnectionFailure(url, err)
		return
	}
	defer client.HangUp(conn)
	client.ConnectionSuccess(url)
	
}