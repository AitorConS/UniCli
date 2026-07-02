package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/AitorConS/jerboa/internal/api"
	"github.com/stretchr/testify/require"
)

func TestDiffVMEvents(t *testing.T) {
	prev := []api.VMInfo{
		{ID: "keep", Name: "api", State: "running"},
		{ID: "gone", Name: "old", State: "stopped"},
	}
	cur := []api.VMInfo{
		{ID: "keep", Name: "api", State: "stopped"},
		{ID: "new", Name: "web", State: "running"},
	}

	events := DiffVMEvents(prev, cur)

	require.Len(t, events, 3)
	require.Equal(t, "vm-state-changed", events[0].Name)
	require.Equal(t, "vm-added", events[1].Name)
	require.Equal(t, "vm-removed", events[2].Name)

	var changed VMEventData
	require.NoError(t, json.Unmarshal(events[0].Data, &changed))
	require.Equal(t, "keep", changed.ID)
	require.Equal(t, "running", changed.PrevState)
	require.Equal(t, "stopped", changed.State)
}

type stubLister struct {
	mu  sync.Mutex
	vms []api.VMInfo
}

func (s *stubLister) List(context.Context) ([]api.VMInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]api.VMInfo(nil), s.vms...), nil
}

func (s *stubLister) set(vms []api.VMInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vms = vms
}

// The poller must survive its first subscriber disconnecting: a later
// subscriber has to keep receiving events (regression test for the poller
// being bound to the first subscriber's request context).
func TestEventPollerSurvivesFirstSubscriber(t *testing.T) {
	lister := &stubLister{}
	p := NewEventPoller(func() (VMLister, func(), error) { return lister, nil, nil })
	p.interval = 10 * time.Millisecond
	p.heartbeat = time.Hour

	waitEvent := func(ch <-chan SSEEvent, name string) SSEEvent {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case ev, ok := <-ch:
				require.True(t, ok, "channel closed while waiting for %s", name)
				if ev.Name == name {
					return ev
				}
			case <-deadline:
				t.Fatalf("timed out waiting for event %s", name)
			}
		}
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	ch1 := p.Subscribe(ctx1)
	waitEvent(ch1, "daemon-status")
	cancel1()

	require.Eventually(t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return !p.running && len(p.subs) == 0
	}, 2*time.Second, 10*time.Millisecond, "poller should stop after last unsubscribe")

	ch2 := p.Subscribe(t.Context())
	waitEvent(ch2, "daemon-status")
	lister.set([]api.VMInfo{{ID: "vm1", Name: "web", State: "running"}})
	ev := waitEvent(ch2, "vm-added")

	var data VMEventData
	require.NoError(t, json.Unmarshal(ev.Data, &data))
	require.Equal(t, "vm1", data.ID)
}
