package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/rpc"

	"example.com/service-registry-lb/internal/registry"
)

func main() {
	listen := flag.String("listen", ":9000", "registry listen address")
	flag.Parse()

	reg := registry.New()

	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Registry", reg); err != nil {
		log.Fatalf("register rpc: %v", err)
	}
	// NOTE: HandleHTTP registers on DefaultServeMux, so we use ListenAndServe(..., nil).
	rpcServer.HandleHTTP(rpc.DefaultRPCPath, rpc.DefaultDebugPath)

	fmt.Printf("Service Registry listening on %s (RPC path %s)\n", *listen, rpc.DefaultRPCPath)
	log.Fatal(http.ListenAndServe(*listen, nil))
}
