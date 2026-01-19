package main

import (
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"time"

	"example.com/service-registry-lb/common"
	"example.com/service-registry-lb/internal/lb"
)

func main() {
	registryAddr := flag.String("registry", "localhost:9000", "registry address host:port")
	service := flag.String("service", "echo", "service name: echo|math")
	algo := flag.String("algo", "rr", "load balancing algorithm: random|rr|wrr")
	n := flag.Int("n", 20, "number of requests in the session")
	sleep := flag.Duration("sleep", 200*time.Millisecond, "sleep between requests")
	flag.Parse()

	// Lookup ONCE per session (cache)
	reg, err := rpc.DialHTTP("tcp", *registryAddr)
	if err != nil {
		log.Fatalf("dial registry: %v", err)
	}

	var lrep common.LookupReply
	if err := reg.Call("Registry.Lookup", &common.LookupArgs{Service: *service}, &lrep); err != nil {
		log.Fatalf("lookup: %v", err)
	}
	instances := lrep.Instances
	if len(instances) == 0 {
		log.Fatalf("no instances for service %q", *service)
	}

	fmt.Printf("Session started. Cached instances for %q:\n", *service)
	for _, inst := range instances {
		fmt.Printf(" - id=%s addr=%s weight=%d\n", inst.ID, inst.Addr, inst.Weight)
	}

	// Choose picker
	var picker lb.Picker
	switch *algo {
	case "random":
		picker = lb.NewRandom(instances)
	case "rr":
		picker = lb.NewRoundRobin(instances)
	case "wrr":
		picker = lb.NewSmoothWeightedRR(instances)
	default:
		log.Fatalf("unknown algo %q", *algo)
	}

	fmt.Printf("\nUsing LB algorithm: %s\n\n", picker.Name())

	for i := 1; i <= *n; i++ {
		inst, err := picker.Pick()
		if err != nil {
			log.Fatalf("pick: %v", err)
		}

		c, err := rpc.DialHTTP("tcp", inst.Addr)
		if err != nil {
			log.Fatalf("dial %s: %v", inst.Addr, err)
		}

		switch *service {
		case "echo":
			var rep common.EchoReply
			err = c.Call("Echo.Echo", &common.EchoArgs{Msg: fmt.Sprintf("hello #%d", i)}, &rep)
			if err != nil {
				log.Fatalf("rpc call: %v", err)
			}
			fmt.Printf("[%02d] picked=%s => reply=%q from=%s\n", i, inst.ID, rep.Msg, rep.From)

		case "math":
			var rep common.AddReply
			err = c.Call("Math.Add", &common.AddArgs{A: i, B: i}, &rep)
			if err != nil {
				log.Fatalf("rpc call: %v", err)
			}
			fmt.Printf("[%02d] picked=%s => %d+%d=%d from=%s\n", i, inst.ID, i, i, rep.Sum, rep.From)

		default:
			log.Fatalf("unknown service %q", *service)
		}

		_ = c.Close()
		time.Sleep(*sleep)
	}

	fmt.Println("\nSession ended (client did NOT refresh registry during the session).")
}
