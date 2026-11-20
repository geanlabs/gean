package host

import (
	"context"
	"errors"
	"fmt"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/geanlabs/gean/internal/core/logger"
)

// Deliver receives a gossip message from another peer that passed its topic's
// validator.
type Deliver func(topic string, msg *pubsub.Message)

// JoinTopic joins and subscribes to topic, with validate judging each message
// before pubsub relays it.
func (h *Host) JoinTopic(topic string, validate pubsub.ValidatorEx) error {
	if err := h.pubsub.RegisterTopicValidator(topic, validate); err != nil {
		return fmt.Errorf("register validator for %s: %w", topic, err)
	}
	t, err := h.pubsub.Join(topic)
	if err != nil {
		_ = h.pubsub.UnregisterTopicValidator(topic)
		return fmt.Errorf("join topic %s: %w", topic, err)
	}
	sub, err := t.Subscribe()
	if err != nil {
		_ = h.pubsub.UnregisterTopicValidator(topic)
		return fmt.Errorf("subscribe to %s: %w", topic, err)
	}
	h.topics[topic] = t
	h.subs[topic] = sub
	return nil
}

// Topics returns the topics the host has joined.
func (h *Host) Topics() []string {
	topics := make([]string, 0, len(h.topics))
	for topic := range h.topics {
		topics = append(topics, topic)
	}
	return topics
}

// Publish gossips data on a joined topic.
func (h *Host) Publish(ctx context.Context, topic string, data []byte) error {
	t, ok := h.topics[topic]
	if !ok {
		return fmt.Errorf("not subscribed to topic: %s", topic)
	}
	return t.Publish(ctx, data)
}

// StartListeners hands every message received on the joined topics to deliver.
func (h *Host) StartListeners(deliver Deliver) {
	h.deliver.Store(&deliver)
	for topic, sub := range h.subs {
		h.tasks.Go(func() { h.listenTopic(h.ctx, topic, sub, deliver) })
	}
}

func (h *Host) listenTopic(ctx context.Context, topic string, sub *pubsub.Subscription, deliver Deliver) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, pubsub.ErrSubscriptionCancelled) {
				return
			}
			logger.Error(logger.Gossip, "recv error on %s: %v", topic, err)
			return
		}

		if msg.ReceivedFrom == h.host.ID() {
			continue
		}
		deliver(topic, msg)
	}
}

// ReannounceSubscriptions renews every subscription, so peers hear them again.
func (h *Host) ReannounceSubscriptions() error {
	deliver := h.deliver.Load()
	if deliver == nil {
		return fmt.Errorf("reannounce: gossip listeners not started yet")
	}
	for topic, oldSub := range h.subs {
		oldSub.Cancel()
		t, ok := h.topics[topic]
		if !ok {
			return fmt.Errorf("reannounce: topic %s missing", topic)
		}
		newSub, err := t.Subscribe()
		if err != nil {
			return fmt.Errorf("reannounce: subscribe %s: %w", topic, err)
		}
		h.subs[topic] = newSub
		h.tasks.Go(func() { h.listenTopic(h.ctx, topic, newSub, *deliver) })
		logger.Info(logger.Network, "re-announced subscription topic=%s", topic)
	}
	return nil
}
