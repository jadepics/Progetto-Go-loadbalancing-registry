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
	service := flag.String("service", "echo", "service name: echo|math|kv")
	algo := flag.String("algo", "rr", "load balancing algorithm: random|rr|wrr")
	n := flag.Int("n", 20, "number of requests in the session")
	sleep := flag.Duration("sleep", 200*time.Millisecond, "sleep between requests")

	op := flag.String("op", "get", "kv operation: get|put (only for service=kv)")
	key := flag.String("key", "x", "kv key (only for service=kv)")
	value := flag.String("value", "v", "kv value (only for service=kv and op=put)")

	flag.Parse()

	if *service == "kv" {
		if *op != "get" && *op != "put" {
			log.Fatalf("invalid -op %q (use get|put)", *op)
		}
		if *key == "" {
			log.Fatalf("missing -key for kv")
		}
	}
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

		case "kv":

			switch *op {
			case "get":
				var rep common.GetReply
				err = c.Call("KV.Get", &common.GetArgs{Key: *key}, &rep)
				if err != nil {
					_ = c.Close()
					log.Fatalf("KV.Get rpc call: %v", err)
				}
				if rep.Found {
					fmt.Printf("[%02d] GET key=%q picked=%s => value=%q from=%s\n", i, *key, inst.ID, rep.Value, rep.From)
				} else {
					fmt.Printf("[%02d] GET key=%q picked=%s => NOT FOUND from=%s\n", i, *key, inst.ID, rep.From)
				}

			case "put":
				// Provo sul server scelto dal LB
				putVal := fmt.Sprintf("%s#%d", *value, i)

				var rep common.PutReply
				err = c.Call("KV.Put", &common.PutArgs{Key: *key, Value: putVal}, &rep)
				if err != nil {
					_ = c.Close()
					log.Fatalf("KV.Put rpc call: %v", err)
				}

				// Se ho colpito un backup: mi dice dove sta il primary -> ritento lì
				if !rep.OK && rep.RedirectTo != "" {
					pc, derr := rpc.DialHTTP("tcp", rep.RedirectTo)
					if derr != nil {
						_ = c.Close()
						log.Fatalf("dial primary %s: %v", rep.RedirectTo, derr)
					}

					var rep2 common.PutReply
					err2 := pc.Call("KV.Put", &common.PutArgs{Key: *key, Value: putVal}, &rep2)
					_ = pc.Close()
					if err2 != nil {
						_ = c.Close()
						log.Fatalf("KV.Put on primary rpc call: %v", err2)
					}
					if !rep2.OK {
						_ = c.Close()
						log.Fatalf("KV.Put on primary failed (ok=false), from=%s", rep2.From)
					}

					fmt.Printf("[%02d] PUT key=%q value=%q picked=%s (backup) -> primary=%s ok from=%s\n",
						i, *key, putVal, inst.ID, rep.RedirectTo, rep2.From)
				} else {
					// Ho colpito direttamente il primary
					if !rep.OK {
						_ = c.Close()
						log.Fatalf("KV.Put failed (ok=false) without redirect, from=%s", rep.From)
					}
					fmt.Printf("[%02d] PUT key=%q value=%q picked=%s ok from=%s\n",
						i, *key, putVal, inst.ID, rep.From)
				}
			}

		default:
			_ = c.Close()
			log.Fatalf("unknown service %q", *service)
		}

		_ = c.Close()
		time.Sleep(*sleep)
	}

	fmt.Println("\nSession ended (client did NOT refresh registry during the session).")
}
