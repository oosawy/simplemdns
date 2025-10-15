package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/miekg/dns"
	"github.com/oosawy/simplemdns"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ", os.Args[0], " <hostname>")
		return
	}
	host := os.Args[1]

	client, err := simplemdns.NewClient()
	if err != nil {
		panic(err)
	}
	defer client.Close()

	question := dns.Question{
		Name:   dns.Fqdn(host),
		Qtype:  dns.TypeA,
		Qclass: dns.ClassINET,
	}
	fmt.Printf("Querying for %s\n", question.String())
	rr, err := client.QueryFirst(context.Background(), question)
	if err != nil {
		fmt.Println("QueryFirst error:", err.Error())
		return
	}
	fmt.Println(rr.String())

}
