package uecontext

import (
	"context"
	"sync"
)

// DUConnection is the in-memory channel pair between a UE and one DU.
// A UeContext holds exactly ONE active DUConnection at a time.
// Handover closes the current one and replaces it with a new one.
type DUConnection struct {
	duID string

	// DU writes inbound RRC here; UE goroutine reads from it.
	ReceiveFromDu chan []byte
	// UE writes outbound RRC here; DU goroutine reads from it.
	SendToDu chan []byte
	// DU signals F1/RRC readiness here.
	IsReady chan bool

	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool
}

func newDUConnection(duID string, parentCtx context.Context) *DUConnection {
	ctx, cancel := context.WithCancel(parentCtx)
	return &DUConnection{
		duID:          duID,
		ReceiveFromDu: make(chan []byte, 100),
		SendToDu:      make(chan []byte, 100),
		IsReady:       make(chan bool, 1),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Send queues an RRC PDU toward the DU.
// Returns false if the connection is already closed or context is done.
func (c *DUConnection) Send(pdu []byte) bool {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()

	select {
	case c.SendToDu <- pdu:
		return true
	case <-c.ctx.Done():
		return false
	}
}

// Close tears down the connection. Safe to call multiple times.
func (c *DUConnection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}
	c.closed = true
	c.cancel()

	close(c.ReceiveFromDu)
	close(c.SendToDu)
	close(c.IsReady)
}