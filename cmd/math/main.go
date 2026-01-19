package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"time"

	"example.com/service-registry-lb/common"
	"example.com/service-registry-lb/internal/util"
)

type MathService struct {
	ID string
}

func (s *MathService) Add(args *common.AddArgs, reply *common.AddReply) error {
	if args == nil {
		args = &common.AddArgs{}
	}
	reply.Sum = args.A + args.B
	reply.From = s.ID
	return nil
}

func main() {
	listen := flag.String("listen", ":9201", "service listen address")
	registryAddr := flag.String("registry", "localhost:9000", "registry address host:port")
	instanceID := flag.String("id", "", "instance id (default: env INSTANCE_ID or 'math-<unix>')")
	publicAddr := flag.String("public", "", "public address to register (default: env PUBLIC_ADDR or listen)")
	weight := flag.Int("weight", 1, "instance weight")
	flag.Parse()

	id := *instanceID
	if id == "" {
		id = util.Env("INSTANCE_ID", "")
	}
	if id == "" {
		id = fmt.Sprintf("math-%d", time.Now().Unix())
	}

	pub := *publicAddr
	if pub == "" {
		pub = util.Env("PUBLIC_ADDR", "")
	}
	if pub == "" {
		pub = *listen
	}

	w := *weight
	if w == 1 {
		w = util.EnvInt("WEIGHT", 1)
	}

	rpcServer := rpc.NewServer()
	if err := rpcServer.RegisterName("Math", &MathService{ID: id}); err != nil {
		log.Fatalf("register Math RPC: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle(rpc.DefaultRPCPath, rpcServer)
	httpSrv := &http.Server{Addr: *listen, Handler: mux}

	// Register to registry
	regClient, err := rpc.DialHTTP("tcp", *registryAddr)
	if err != nil {
		log.Fatalf("dial registry: %v", err)
	}
	regArgs := &common.RegisterArgs{
		Service: "math",
		Instance: common.Instance{
			ID:     id,
			Addr:   pub,
			Weight: w,
			Meta:   map[string]string{"kind": "math"},
		},
	}
	var regReply common.RegisterReply
	if err := regClient.Call("Registry.Register", regArgs, &regReply); err != nil || !regReply.OK {
		log.Fatalf("register in registry: %v", err)
	}
	log.Printf("[math %s] registered at %s with addr=%s weight=%d", id, *registryAddr, pub, w)

	go func() {
		log.Printf("[math %s] listening on %s", id, *listen)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	util.WaitForShutdown(func(ctx context.Context) {
		log.Printf("[math %s] shutting down...", id)
		var drep common.DeregisterReply
		_ = regClient.Call("Registry.Deregister", &common.DeregisterArgs{Service: "math", ID: id}, &drep)
		_ = httpSrv.Shutdown(ctx)
	})
}
