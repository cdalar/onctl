package tools

import (
	"log"
	"net"
	"time"
)

func resolveDomain(domain string) (*net.TCPAddr, error) {
	return net.ResolveTCPAddr("tcp4", domain)
}

func Ping(addr string) (time.Duration, error) {
	tcpaddr, err := resolveDomain(addr)
	if err != nil {
		log.Fatalln("Error:", err)
	}

	start := time.Now()
	// fsn1-speed.hetzner.com:80
	conn, err := net.Dial("tcp", tcpaddr.String())
	if err != nil {
		log.Fatalln("Error:", err)

	}
	since := time.Since(start)
	defer conn.Close()
	return since, nil
}
