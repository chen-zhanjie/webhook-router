package broker

import (
	"sync"
	"time"
)

type Client struct {
	AppID string
	done  chan struct{}
	once  sync.Once
}

func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) Close() {
	c.once.Do(func() {
		close(c.done)
	})
}

type Broker struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]struct{}
}

func New(buffer int) *Broker {
	return &Broker{clients: map[string]map[*Client]struct{}{}}
}

func (b *Broker) Register(appID string) *Client {
	client := &Client{AppID: appID, done: make(chan struct{})}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clients[appID] == nil {
		b.clients[appID] = map[*Client]struct{}{}
	}
	b.clients[appID][client] = struct{}{}
	return client
}

func (b *Broker) Unregister(client *Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if clients := b.clients[client.AppID]; clients != nil {
		delete(clients, client)
		if len(clients) == 0 {
			delete(b.clients, client.AppID)
		}
	}
	client.Close()
}

func (b *Broker) Online(appID string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients[appID])
}

func (b *Broker) TotalOnline() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	total := 0
	for _, clients := range b.clients {
		total += len(clients)
	}
	return total
}

func (b *Broker) Snapshot() map[string]int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := map[string]int{}
	for appID, clients := range b.clients {
		out[appID] = len(clients)
	}
	return out
}

func Heartbeat(interval time.Duration) *time.Ticker { return time.NewTicker(interval) }
