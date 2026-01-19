package registry

import (
	"errors"
	"sort"
	"sync"

	"example.com/service-registry-lb/common"
)

type Registry struct {
	mu       sync.RWMutex
	services map[string]map[string]common.Instance // service -> id -> instance
}

func New() *Registry {
	return &Registry{
		services: make(map[string]map[string]common.Instance),
	}
}

// Register adds/updates an instance in the registry.
func (r *Registry) Register(args *common.RegisterArgs, reply *common.RegisterReply) error {
	if args == nil || args.Service == "" || args.Instance.ID == "" || args.Instance.Addr == "" {
		return errors.New("invalid register args")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	m, ok := r.services[args.Service]
	if !ok {
		m = make(map[string]common.Instance)
		r.services[args.Service] = m
	}
	m[args.Instance.ID] = args.Instance
	reply.OK = true
	return nil
}

// Deregister removes an instance from the registry.
func (r *Registry) Deregister(args *common.DeregisterArgs, reply *common.DeregisterReply) error {
	if args == nil || args.Service == "" || args.ID == "" {
		return errors.New("invalid deregister args")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if m, ok := r.services[args.Service]; ok {
		delete(m, args.ID)
		if len(m) == 0 {
			delete(r.services, args.Service)
		}
	}
	reply.OK = true
	return nil
}

// Lookup returns the list of active instances for a given service.
func (r *Registry) Lookup(args *common.LookupArgs, reply *common.LookupReply) error {
	if args == nil || args.Service == "" {
		return errors.New("invalid lookup args")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.services[args.Service]
	if !ok || len(m) == 0 {
		reply.Instances = nil
		return nil
	}

	out := make([]common.Instance, 0, len(m))
	for _, inst := range m {
		out = append(out, inst)
	}

	// Stable order helps debugging and round-robin consistency.
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == out[j].ID {
			return out[i].Addr < out[j].Addr
		}
		return out[i].ID < out[j].ID
	})

	reply.Instances = out
	return nil
}
