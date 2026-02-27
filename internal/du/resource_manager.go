package du

import (
	"fmt"
	"sync"
)

// ResourceManager manages radio resources like C-RNTIs and PCI lists
type ResourceManager struct {
	// C-RNTI Management
	crntiPool map[int64]bool // Set of allocated C-RNTIs
	mu        sync.Mutex
	minCrnti  int64
	maxCrnti  int64

	// Neighbor Management
	neighborList []int64

	// TEID Management
	teidCounter uint32
	teidMu      sync.Mutex
}

// NewResourceManager creates a new resource manager
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		crntiPool:    make(map[int64]bool),
		minCrnti:     100, // Reserve 0-99 for special use
		maxCrnti:     65535,
		neighborList: []int64{2, 3, 4}, // Initial simulated neighbors
		teidCounter:  1000,             // Start TEIDs from 1000
	}
}

// AllocateCRNTI allocates a unique C-RNTI
func (rm *ResourceManager) AllocateCRNTI() (int64, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Simple linear search for now - optimize if needed
	for id := rm.minCrnti; id <= rm.maxCrnti; id++ {
		if !rm.crntiPool[id] {
			rm.crntiPool[id] = true
			return id, nil
		}
	}

	return 0, fmt.Errorf("no C-RNTI available")
}

// ReleaseCRNTI releases a C-RNTI back to the pool
func (rm *ResourceManager) ReleaseCRNTI(crnti int64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	delete(rm.crntiPool, crnti)
}

// IsValidNeighbor checks if a PCI is in the neighbor list
func (rm *ResourceManager) IsValidNeighbor(pci int64) bool {
	for _, neighbor := range rm.neighborList {
		if neighbor == pci {
			return true
		}
	}
	return false
}

// AddNeighbor adds a PCI to the neighbor list
func (rm *ResourceManager) AddNeighbor(pci int64) {
	if !rm.IsValidNeighbor(pci) {
		rm.neighborList = append(rm.neighborList, pci)
	}
}

// AllocateTEID allocates a unique TEID for PDU Sessions
func (rm *ResourceManager) AllocateTEID() (uint32, error) {
	rm.teidMu.Lock()
	defer rm.teidMu.Unlock()

	rm.teidCounter++
	return rm.teidCounter, nil
}
