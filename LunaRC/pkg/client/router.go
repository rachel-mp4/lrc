package client

import (
	"fmt"
	"net"
)

func Dial(url string) (net.Conn, error){
	return net.Dial("tcp", fmt.Sprintf("%s:927", url))
}

func HangUp(conn net.Conn) {
	conn.Close()
}