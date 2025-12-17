// Package api provides HTTP handlers for the RPC API.
package api

import (
	"sync"
)

// WriteEvent represents a message write that occurred
type WriteEvent struct {
	Namespace      string
	Stream         string
	Category       string
	Position       int64
	GlobalPosition int64
}

// Subscriber is a channel that receives write events
type Subscriber chan WriteEvent

// PubSub manages subscriptions for real-time notifications
type PubSub struct {
	mu sync.RWMutex

	// Stream subscribers: namespace -> stream -> subscribers
	streamSubs map[string]map[string]map[Subscriber]struct{}

	// Category subscribers: namespace -> category -> subscribers
	categorySubs map[string]map[string]map[Subscriber]struct{}
}

// NewPubSub creates a new PubSub instance
func NewPubSub() *PubSub {
	return &PubSub{
		streamSubs:   make(map[string]map[string]map[Subscriber]struct{}),
		categorySubs: make(map[string]map[string]map[Subscriber]struct{}),
	}
}

// SubscribeStream subscribes to a specific stream
func (ps *PubSub) SubscribeStream(namespace, stream string) Subscriber {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	sub := make(Subscriber, 100) // Buffer to avoid blocking

	if ps.streamSubs[namespace] == nil {
		ps.streamSubs[namespace] = make(map[string]map[Subscriber]struct{})
	}
	if ps.streamSubs[namespace][stream] == nil {
		ps.streamSubs[namespace][stream] = make(map[Subscriber]struct{})
	}
	ps.streamSubs[namespace][stream][sub] = struct{}{}

	return sub
}

// UnsubscribeStream removes a stream subscription
func (ps *PubSub) UnsubscribeStream(namespace, stream string, sub Subscriber) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.streamSubs[namespace] != nil && ps.streamSubs[namespace][stream] != nil {
		delete(ps.streamSubs[namespace][stream], sub)
		if len(ps.streamSubs[namespace][stream]) == 0 {
			delete(ps.streamSubs[namespace], stream)
		}
		if len(ps.streamSubs[namespace]) == 0 {
			delete(ps.streamSubs, namespace)
		}
	}
	close(sub)
}

// SubscribeCategory subscribes to a category
func (ps *PubSub) SubscribeCategory(namespace, category string) Subscriber {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	sub := make(Subscriber, 100) // Buffer to avoid blocking

	if ps.categorySubs[namespace] == nil {
		ps.categorySubs[namespace] = make(map[string]map[Subscriber]struct{})
	}
	if ps.categorySubs[namespace][category] == nil {
		ps.categorySubs[namespace][category] = make(map[Subscriber]struct{})
	}
	ps.categorySubs[namespace][category][sub] = struct{}{}

	return sub
}

// UnsubscribeCategory removes a category subscription
func (ps *PubSub) UnsubscribeCategory(namespace, category string, sub Subscriber) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.categorySubs[namespace] != nil && ps.categorySubs[namespace][category] != nil {
		delete(ps.categorySubs[namespace][category], sub)
		if len(ps.categorySubs[namespace][category]) == 0 {
			delete(ps.categorySubs[namespace], category)
		}
		if len(ps.categorySubs[namespace]) == 0 {
			delete(ps.categorySubs, namespace)
		}
	}
	close(sub)
}

// Publish notifies all relevant subscribers about a write
func (ps *PubSub) Publish(event WriteEvent) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	// Notify stream subscribers
	if ps.streamSubs[event.Namespace] != nil {
		if subs := ps.streamSubs[event.Namespace][event.Stream]; subs != nil {
			for sub := range subs {
				select {
				case sub <- event:
				default:
					// Channel full, skip (subscriber is slow)
				}
			}
		}
	}

	// Notify category subscribers
	if ps.categorySubs[event.Namespace] != nil {
		if subs := ps.categorySubs[event.Namespace][event.Category]; subs != nil {
			for sub := range subs {
				select {
				case sub <- event:
				default:
					// Channel full, skip (subscriber is slow)
				}
			}
		}
	}
}
