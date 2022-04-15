//go:build windows

package main

import "context"

type fakePublisher struct {
	events []interface{}
}

var _ publisher = &fakePublisher{}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{}
}

func (p *fakePublisher) publishEvent(ctx context.Context, topic string, event interface{}) (err error) {
	if p == nil {
		return nil
	}
	p.events = append(p.events, event)
	return nil
}
