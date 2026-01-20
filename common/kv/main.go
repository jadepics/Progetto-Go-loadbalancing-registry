package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/rpc"
	"sort"
	"sync"
	"time"

	"example.com/service-registry-lb/common"
	"example.com/service-registry-lb/internal/util"
)

type KVService struct {
	id       string
	registry *rpc.Client

	mu        sync.RWMutex
	store     map[string]string
	seq       int64
	lastApply int64

	roleMu  sync.RWMutex
	role    string // "primary" | "backup"
	primary common.Instance
}

func (s *KVService) isPrimary() bool {
	s.roleMu.RLock()
	defer s.roleMu.RUnlock()
	return s.role == "primary"
}
func (s *KVService) primaryAddr() string {
	s.roleMu.RLock()
	defer s.roleMu.RUnlock()
	return s.primary.Addr
}
func (s *KVService) setRole(role string, primary common.Instance) {
	s.roleMu.Lock()
	s.role = role
	s.primary = primary
	s.roleMu.Unlock()
}

// -------- RPC: Get (su primary e backup) --------
func (s *KVService) Get(args *common.GetArgs, reply *common.GetReply) error {
	if args == nil {
		args = &common.GetArgs{}
	}
	s.mu.RLock()
	v, ok := s.store[args.Key]
	s.mu.RUnlock()

	reply.Found = ok
	reply.Value = v
	reply.From = s.id
	return nil
}

// -------- RPC: Put (solo primary) --------
func (s *KVService) Put(args *common.PutArgs, reply *common.PutReply) error {
	if args == nil {
		args = &common.PutArgs{}
	}
	if args.Key == "" {
		return errors.New("missing key")
	}

	// Se sono backup: rifiuto e comunico il primary
	if !s.isPrimary() {
		reply.OK = false
		reply.From = s.id
		reply.RedirectTo = s.primaryAddr()
		return nil
	}

	// Applico localmente con sequenza monotona
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.store[args.Key] = args.Value
	s.lastApply = seq
	s.mu.Unlock()

	// Replica sincrona “strict” verso tutti i backup
	backups, err := s.lookupBackups()
	if err != nil {
		return err
	}

	for _, b := range backups {
		c, err := rpc.DialHTTP("tcp", b.Addr)
		if err != nil {
			return fmt.Errorf("replicate dial %s: %w", b.Addr, err)
		}
		var arep common.ApplyReply
		callErr := c.Call("KV.Apply", &common.ApplyArgs{Seq: seq, Key: args.Key, Value: args.Value}, &arep)
		_ = c.Close()
		if callErr != nil || !arep.OK {
			return fmt.Errorf("replicate apply to %s: %v", b.ID, callErr)
		}
	}

	reply.OK = true
	reply.From = s.id
	return nil
}

// -------- RPC: Apply (primary -> backup) --------
func (s *KVService) Apply(args *common.ApplyArgs, reply *common.ApplyReply) error {
	if args == nil {
		args = &common.ApplyArgs{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// idempotenza: se arriva due volte lo stesso seq
	if args.Seq <= s.lastApply {
		reply.OK = true
		return nil
	}

	// ordine stretto: se manca una replica, forza resync (snapshot)
	if s.lastApply != 0 && args.Seq != s.lastApply+1 {
		reply.OK = false
		return fmt.Errorf("out of order apply: have=%d got=%d", s.lastApply, args.Seq)
	}

	s.store[args.Key] = args.Value
	s.lastApply = args.Seq
	if args.Seq > s.seq {
		s.seq = args.Seq
	}

	reply.OK = true
	return nil
}

// -------- RPC: Snapshot (backup -> primary) --------
func (s *KVService) Snapshot(_ *common.SnapshotArgs, reply *common.SnapshotReply) error {
	if !s.isPrimary() {
		return errors.New("not primary")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	state := make(map[string]string, len(s.store))
	for k, v := range s.store {
		state[k] = v
	}
	reply.Seq = s.seq
	reply.State = state
	return nil
}

// ----- registry helpers -----
func (s *KVService) lookupAll() ([]common.Instance, error) {
	var rep common.LookupReply
	if err := s.registry.Call("Registry.Lookup", &common.LookupArgs{Service: "kv"}, &rep); err != nil {
		return nil, err
	}
	return rep.Instances, nil
}
func (s *KVService) lookupBackups() ([]common.Instance, error) {
	inst, err := s.lookupAll()
	if err != nil {
		return nil, err
	}
	out := make([]common.Instance, 0, len(inst))
	for _, in := range inst {
		if in.ID != s.id {
			out = append(out, in)
		}
	}
	return out, nil
}

func pickPrimary(instances []common.Instance, forcedPrimaryID string) (common.Instance, bool) {
	if len(instances) == 0 {
		return common.Instance{}, false
	}
	if forcedPrimaryID != "" {
		for _, inst := range instances {
			if inst.ID == forcedPrimaryID {
				return inst, true
			}
		}
	}
	// fallback: lowest ID
	sort.Slice(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
	return instances[0], true
}

func bootstrapFromPrimary(svc *KVService, primaryAddr string) error {
	c, err := rpc.DialHTTP("tcp", primaryAddr)
	if err != nil {
		return err
	}
	defer c.Close()

	var rep common.SnapshotReply
	if err := c.Call("KV.Snapshot", &common.SnapshotArgs{}, &rep); err != nil {
		return err
	}

	svc.mu.Lock()
	if rep.Seq > svc.seq {
		svc.store = make(map[string]string, len(rep.State))
		for k, v := range rep.State {
			svc.store[k] = v
		}
		svc.seq = rep.Seq
		svc.lastApply = rep.Seq
	}
	svc.mu.Unlock()
	return nil
}

func main() {
	listen := flag.String("listen", ":9301", "service listen address")
	registryAddr := flag.String("registry", "localhost:9000", "registry address host:port")
	instanceID := flag.String("id", "", "instance id (default: env INSTANCE_ID or 'kv-<unix>')")
	publicAddr := flag.String("public", "", "public address to register (default: env PUBLIC_ADDR or listen)")
	forcedPrimary := flag.String("primary-id", "", "force a specific instance ID to be primary")
	weight := flag.Int("weight", 1, "instance weight")
	flag.Parse()

	id := *instanceID
	if id == "" {
		id = util.Env("INSTANCE_ID", "")
	}
	if id == "" {
		id = fmt.Sprintf("kv-%d", time.Now().Unix())
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

	// RPC server
	rpcServer := rpc.NewServer()
	svc := &KVService{id: id, store: map[string]string{}}
	if err := rpcServer.RegisterName("KV", svc); err != nil {
		log.Fatalf("register KV RPC: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle(rpc.DefaultRPCPath, rpcServer)
	httpSrv := &http.Server{Addr: *listen, Handler: mux}

	go func() {
		log.Printf("[kv %s] listening on %s", id, *listen)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	// Connect registry
	regClient, err := rpc.DialHTTP("tcp", *registryAddr)
	if err != nil {
		log.Fatalf("dial registry: %v", err)
	}
	svc.registry = regClient

	// Register to registry
	var regReply common.RegisterReply
	err = regClient.Call("Registry.Register", &common.RegisterArgs{
		Service: "kv",
		Instance: common.Instance{
			ID:     id,
			Addr:   pub,
			Weight: w,
			Meta:   map[string]string{"kind": "kv"},
		},
	}, &regReply)
	if err != nil || !regReply.OK {
		log.Fatalf("register in registry: %v", err)
	}
	log.Printf("[kv %s] registered at %s with addr=%s weight=%d", id, *registryAddr, pub, w)

	// Primary selection: flag > env > lowest ID
	primaryID := *forcedPrimary
	if primaryID == "" {
		primaryID = util.Env("PRIMARY_ID", "")
	}

	// Loop: ricalcola ruolo e (se backup) bootstrap snapshot
	go func() {
		for {
			inst, err := svc.lookupAll()
			if err == nil {
				p, ok := pickPrimary(append([]common.Instance(nil), inst...), primaryID)
				if ok {
					role := "backup"
					if p.ID == id {
						role = "primary"
					}
					wasPrimary := svc.isPrimary()
					svc.setRole(role, p)
					if wasPrimary != (role == "primary") {
						log.Printf("[kv %s] role => %s (primary=%s@%s)", id, role, p.ID, p.Addr)
					}
					if role == "backup" && p.Addr != "" {
						_ = bootstrapFromPrimary(svc, p.Addr)
					}
				}
			}
			time.Sleep(2 * time.Second)
		}
	}()

	// Deregister on shutdown
	util.WaitForShutdown(func(ctx context.Context) {
		log.Printf("[kv %s] shutting down...", id)
		var drep common.DeregisterReply
		_ = regClient.Call("Registry.Deregister", &common.DeregisterArgs{Service: "kv", ID: id}, &drep)
		_ = httpSrv.Shutdown(ctx)
	})
}
