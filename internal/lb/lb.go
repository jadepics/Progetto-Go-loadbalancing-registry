package lb

import (
	"errors"
	"math/rand"
	"sync/atomic"
	"time"

	"example.com/service-registry-lb/common"
)

type Picker interface {
	Pick() (common.Instance, error)
	Name() string
}

// -------- Random (stateless) --------

type RandomPicker struct {
	instances []common.Instance
	rnd       *rand.Rand
}

func NewRandom(instances []common.Instance) *RandomPicker {
	return &RandomPicker{
		instances: append([]common.Instance(nil), instances...),
		rnd:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (p *RandomPicker) Name() string { return "random" }

func (p *RandomPicker) Pick() (common.Instance, error) {
	if len(p.instances) == 0 {
		return common.Instance{}, errors.New("no instances")
	}
	return p.instances[p.rnd.Intn(len(p.instances))], nil
}

// -------- Round-robin (stateless-ish) --------

type RoundRobinPicker struct {
	instances []common.Instance
	idx       uint64
}

func NewRoundRobin(instances []common.Instance) *RoundRobinPicker {
	return &RoundRobinPicker{
		instances: append([]common.Instance(nil), instances...),
	}
}

func (p *RoundRobinPicker) Name() string { return "round_robin" }

func (p *RoundRobinPicker) Pick() (common.Instance, error) {
	if len(p.instances) == 0 {
		return common.Instance{}, errors.New("no instances")
	}
	i := atomic.AddUint64(&p.idx, 1)
	return p.instances[int(i-1)%len(p.instances)], nil
}

// -------- Smooth Weighted Round-robin (stateful) --------
//
// Stateful because it keeps per-instance "current weight" that changes at every pick.
// It produces a smooth distribution proportional to weights.

type SmoothWeightedRR struct {
	instances []common.Instance
	current   []int
	totalW    int
}

func NewSmoothWeightedRR(instances []common.Instance) *SmoothWeightedRR {
	p := &SmoothWeightedRR{
		instances: append([]common.Instance(nil), instances...),
		current:   make([]int, len(instances)),
	}
	for _, inst := range instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}
		p.totalW += w
	}
	if p.totalW == 0 {
		p.totalW = 1
	}
	return p
}

func (p *SmoothWeightedRR) Name() string { return "smooth_weighted_rr" }

func (p *SmoothWeightedRR) Pick() (common.Instance, error) {
	if len(p.instances) == 0 {
		return common.Instance{}, errors.New("no instances")
	}

	best := 0
	bestVal := -1 << 30

	for i, inst := range p.instances {
		w := inst.Weight
		if w <= 0 {
			w = 1
		}
		p.current[i] += w
		if p.current[i] > bestVal {
			bestVal = p.current[i]
			best = i
		}
	}

	p.current[best] -= p.totalW
	return p.instances[best], nil
}
