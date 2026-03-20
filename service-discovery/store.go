package servicediscovery

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type instanceRef struct {
	Type string
	ID   int64
}

// store provides local-cache lookups for discovered service instances.
// All methods are safe for concurrent use.
type store struct {
	mu sync.RWMutex

	byTypeID    map[string]map[int64]*Instance
	byPrivateEP map[string]instanceRef

	selfPrivateEP string
	selfID        int64
	ordering      OrderingStrategy

	rrCursor map[string]*atomic.Uint64 // For round-robin ordering
}

func newStore(selfPrivateEP string, selfID int64, ordering OrderingStrategy) *store {
	return &store{
		byTypeID:      make(map[string]map[int64]*Instance),
		byPrivateEP:   make(map[string]instanceRef),
		selfPrivateEP: selfPrivateEP,
		selfID:        selfID,
		ordering:      ordering,
		rrCursor:      make(map[string]*atomic.Uint64),
	}
}

func (s *store) setOrdering(ordering OrderingStrategy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ordering = ordering
}

func (s *store) upsert(inst Instance) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If the same private endpoint is already registered for a different instance id,
	// remove the old mapping to avoid duplicates.
	if ref, ok := s.byPrivateEP[inst.PrivateEndPoint]; ok {
		if ref.Type != inst.Type || ref.ID != inst.ID {
			if m := s.byTypeID[ref.Type]; m != nil {
				delete(m, ref.ID)
				if len(m) == 0 {
					delete(s.byTypeID, ref.Type)
				}
			}
		}
	}

	m := s.byTypeID[inst.Type]
	if m == nil {
		m = make(map[int64]*Instance)
		s.byTypeID[inst.Type] = m
	}
	// Create copy
	cpy := inst
	m[inst.ID] = &cpy
	s.byPrivateEP[inst.PrivateEndPoint] = instanceRef{Type: inst.Type, ID: inst.ID}

	if s.rrCursor[inst.Type] == nil {
		s.rrCursor[inst.Type] = &atomic.Uint64{}
	}
}

func (s *store) removeByTypeID(serviceType string, id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	m := s.byTypeID[serviceType]
	if m == nil {
		return
	}
	inst := m[id]
	delete(m, id)
	if inst != nil {
		delete(s.byPrivateEP, inst.PrivateEndPoint)
	}
	if len(m) == 0 {
		delete(s.byTypeID, serviceType)
		delete(s.rrCursor, serviceType)
	}
}

func (s *store) removeByPrivateEP(privateEP string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ref, ok := s.byPrivateEP[privateEP]
	if !ok {
		return
	}
	delete(s.byPrivateEP, privateEP)
	m := s.byTypeID[ref.Type]
	if m != nil {
		delete(m, ref.ID)
		if len(m) == 0 {
			delete(s.byTypeID, ref.Type)
			delete(s.rrCursor, ref.Type)
		}
	}
}

func (s *store) sweepExpired(now time.Time, expiryTimeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for svcType, m := range s.byTypeID {
		for id, inst := range m {
			if inst == nil {
				delete(m, id)
				continue
			}
			if now.Sub(inst.LastSeenUTC) > expiryTimeout {
				delete(s.byPrivateEP, inst.PrivateEndPoint)
				delete(m, id)
			}
		}
		if len(m) == 0 {
			delete(s.byTypeID, svcType)
			delete(s.rrCursor, svcType)
		}
	}
}

// GetAllServiceInstances returns discovered instances for a given service type.
// It never returns the calling (self) instance.
func (s *store) GetAllServiceInstances(serviceType string) []Instance {
	s.mu.Lock()
	defer s.mu.Unlock()

	m := s.byTypeID[serviceType]
	if len(m) == 0 {
		return nil
	}

	instances := make([]Instance, 0, len(m))
	for _, inst := range m {
		if inst == nil {
			continue
		}
		if inst.PrivateEndPoint == s.selfPrivateEP || inst.ID == s.selfID {
			continue
		}
		instances = append(instances, *inst)
	}
	if len(instances) == 0 {
		return nil
	}

	switch s.ordering {
	case OrderingLastSeen:
		sort.Slice(instances, func(i, j int) bool {
			return instances[i].LastSeenUTC.After(instances[j].LastSeenUTC)
		})
		return instances
	case OrderingLatestVersion:
		best := instances[0].Version
		for _, inst := range instances[1:] {
			if compareVersionLoose(inst.Version, best) > 0 {
				best = inst.Version
			}
		}
		filtered := instances[:0]
		for _, inst := range instances {
			if compareVersionLoose(inst.Version, best) == 0 {
				filtered = append(filtered, inst)
			}
		}
		instances = filtered
		// within the best version, order by last-seen desc
		sort.Slice(instances, func(i, j int) bool {
			return instances[i].LastSeenUTC.After(instances[j].LastSeenUTC)
		})
		return instances
	case OrderingRoundRobin:
		// Stable order base: by ID asc, then rotate.
		sort.Slice(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
		cur := s.rrCursor[serviceType]
		if cur == nil {
			cur = &atomic.Uint64{}
			s.rrCursor[serviceType] = cur
		}
		n := uint64(len(instances))
		if n <= 1 {
			return instances
		}
		off := cur.Add(1) % n
		rotated := make([]Instance, 0, len(instances))
		rotated = append(rotated, instances[off:]...)
		rotated = append(rotated, instances[:off]...)
		return rotated
	case OrderingNone:
		fallthrough
	default:
		return instances
	}
}

// GetServiceInstances returns a single discovered instance for the given service type,
// selected according to the configured ordering strategy.
func (s *store) GetServiceInstances(serviceType string) *Instance {
	instances := s.GetAllServiceInstances(serviceType)
	if len(instances) == 0 {
		return nil
	}
	selected := instances[0]
	return &selected
}
