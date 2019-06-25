package main

import "context"

var _ = (publisher)(fakePublisher)

func fakePublisher(ctx context.Context, topic string, event interface{}) {
	// Do nothing
}
