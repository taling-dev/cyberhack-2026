package events

import (
	"context"
	"log/slog"
	"runtime/debug"

	"github.com/nats-io/nats.go"
)

// SubscribedSubjects is the canonical list of NATS subject patterns the API
// fans out to SSE clients. Kept here (not in filter.go) because it relates to
// subscription wiring rather than per-user authorization.
var SubscribedSubjects = []string{
	"qc.>",
	"lot.>",
	"warehouse.>",
	"dispatch.>",
	"audit.>",
}

// StartSubscriber opens core (non-JetStream) NATS subscriptions for every
// SubscribedSubjects pattern and routes incoming messages into the hub.
//
// Why core subs instead of JetStream consumers:
//   - We only want best-effort delivery to SSE clients. The TanStack Query
//     full-resync on reconnect heals any missed event.
//   - No consumer state on the NATS server. Adding/removing API replicas is
//     stateless: each pod independently subscribes; messages fan out to all
//     pods automatically (no queue group).
//   - Lower latency: no ack tracking, no consumer interest bookkeeping.
//   - JetStream still stores the durable copy for the AI worker.
//
// Returns the open subscriptions so the caller can drain them on shutdown.
func StartSubscriber(ctx context.Context, nc *nats.Conn, hub *Hub) ([]*nats.Subscription, error) {
	subs := make([]*nats.Subscription, 0, len(SubscribedSubjects))
	for _, subject := range SubscribedSubjects {
		sub, err := nc.Subscribe(subject, makeHandler(hub))
		if err != nil {
			// Roll back any successful subs so we don't leak goroutines.
			for _, s := range subs {
				_ = s.Unsubscribe()
			}
			return nil, err
		}
		subs = append(subs, sub)
		slog.Info("sse: subscribed", "subject", subject)
	}
	go func() {
		<-ctx.Done()
		// Caller should also call sub.Drain() during shutdown sequencing.
	}()
	return subs, nil
}

// makeHandler returns a NATS message handler that dispatches into the hub
// with panic recovery. A bad message can never kill the dispatcher
// goroutine — we recover, increment a counter, and continue.
func makeHandler(hub *Hub) nats.MsgHandler {
	return func(msg *nats.Msg) {
		defer func() {
			if r := recover(); r != nil {
				SSEDispatchPanicsTotal.Inc()
				slog.Error("sse: nats handler panic recovered",
					"panic", r,
					"subject", msg.Subject,
					"stack", string(debug.Stack()),
				)
			}
		}()
		hub.Dispatch(msg.Subject, msg.Data)
	}
}
