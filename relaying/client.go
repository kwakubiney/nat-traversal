package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

func main() {
	remoteAddress := "8.8.8.8:80"
	addr, err := net.ResolveUDPAddr("", remoteAddress)
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("The UDP server is %s\n", conn.RemoteAddr().String())
	defer conn.Close()

	for {
		data := []byte("!2345" + "\n")
		time.Sleep(5 * time.Second)
		_, err := conn.Write(data)
		fmt.Println("Writing")
		if err != nil {
			log.Println(err)
		}
	}
}
