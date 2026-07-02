package agent

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/AitorConS/jerboa/internal/api"
)

type VMLister interface {
	List(context.Context) ([]api.VMInfo, error)
}

type VMEventData struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	State     string `json:"state"`
	PrevState string `json:"prevState,omitempty"`
}

func DiffVMEvents(prev, cur []api.VMInfo) []SSEEvent {
	events := []SSEEvent{}
	prevByID := make(map[string]api.VMInfo, len(prev))
	curByID := make(map[string]api.VMInfo, len(cur))
	for _, vm := range prev {
		prevByID[vm.ID] = vm
	}
	for _, vm := range cur {
		curByID[vm.ID] = vm
		if old, ok := prevByID[vm.ID]; !ok {
			events = appendJSONEvent(events, "vm-added", VMEventData{ID: vm.ID, Name: vm.Name, State: vm.State})
		} else if old.State != vm.State {
			events = appendJSONEvent(events, "vm-state-changed", VMEventData{
				ID:        vm.ID,
				Name:      vm.Name,
				State:     vm.State,
				PrevState: old.State,
			})
		}
	}
	for _, vm := range prev {
		if _, ok := curByID[vm.ID]; !ok {
			events = appendJSONEvent(events, "vm-removed", VMEventData{ID: vm.ID, Name: vm.Name, State: vm.State})
		}
	}
	return events
}

func appendJSONEvent(events []SSEEvent, name string, v any) []SSEEvent {
	data, err := json.Marshal(v)
	if err != nil {
		return events
	}
	return append(events, SSEEvent{Name: name, Data: data})
}

type EventPoller struct {
	listerFactory func() (VMLister, func(), error)
	interval      time.Duration
	heartbeat     time.Duration

	mu           sync.Mutex
	subs         map[chan SSEEvent]struct{}
	running      bool
	stop         context.CancelFunc
	wasReachable *bool
	prev         []api.VMInfo
}

func NewEventPoller(factory func() (VMLister, func(), error)) *EventPoller {
	return &EventPoller{
		listerFactory: factory,
		interval:      1500 * time.Millisecond,
		heartbeat:     15 * time.Second,
		subs:          map[chan SSEEvent]struct{}{},
		prev:          []api.VMInfo{},
	}
}

func (p *EventPoller) Subscribe(ctx context.Context) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)
	p.mu.Lock()
	p.subs[ch] = struct{}{}
	if !p.running {
		// The poller must outlive any single subscriber, so it runs under its
		// own context — not the subscriber's request context — and shuts down
		// only when the last subscriber leaves.
		runCtx, cancel := context.WithCancel(context.Background())
		p.running = true
		p.stop = cancel
		go p.run(runCtx)
	}
	p.mu.Unlock()

	go func() {
		<-ctx.Done()
		p.mu.Lock()
		delete(p.subs, ch)
		close(ch)
		if len(p.subs) == 0 && p.running {
			p.running = false
			p.stop()
			// Reset snapshots so a future subscriber gets a fresh
			// daemon-status event instead of diffs against stale state.
			p.wasReachable = nil
			p.prev = []api.VMInfo{}
		}
		p.mu.Unlock()
	}()
	return ch
}

func (p *EventPoller) run(ctx context.Context) {
	tick := time.NewTicker(p.interval)
	beat := time.NewTicker(p.heartbeat)
	defer tick.Stop()
	defer beat.Stop()
	p.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			p.poll(ctx)
		case <-beat.C:
			p.broadcast(SSEEvent{Data: []byte(": heartbeat")})
		}
	}
}

func (p *EventPoller) poll(ctx context.Context) {
	lister, closeFn, err := p.listerFactory()
	reachable := err == nil
	var cur []api.VMInfo
	if err == nil {
		cur, err = lister.List(ctx)
		reachable = err == nil
	}
	if closeFn != nil {
		closeFn()
	}

	p.mu.Lock()
	if p.wasReachable == nil || *p.wasReachable != reachable {
		p.wasReachable = &reachable
		p.mu.Unlock()
		p.broadcastJSON("daemon-status", map[string]bool{"reachable": reachable})
		p.mu.Lock()
	}
	if reachable {
		events := DiffVMEvents(p.prev, cur)
		p.prev = append([]api.VMInfo(nil), cur...)
		p.mu.Unlock()
		for _, ev := range events {
			p.broadcast(ev)
		}
		return
	}
	p.prev = []api.VMInfo{}
	p.mu.Unlock()
}

func (p *EventPoller) broadcastJSON(name string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	p.broadcast(SSEEvent{Name: name, Data: data})
}

func (p *EventPoller) broadcast(ev SSEEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for ch := range p.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}
