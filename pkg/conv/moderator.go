package conv

import (
	"context"
	"fmt"
)

// ModeratorOption configures a Moderator.
type ModeratorOption func(*Moderator)

// WithMaxRounds sets the maximum number of conversation rounds.
func WithMaxRounds(n int) ModeratorOption {
	return func(m *Moderator) {
		if n > 0 {
			m.maxRounds = n
		}
	}
}

// WithTermination sets a custom termination condition.
// The function receives the full conversation history and returns true to stop.
func WithTermination(fn func(history []Envelope) bool) ModeratorOption {
	return func(m *Moderator) {
		m.terminationFn = fn
	}
}

// WithTurnOrder sets a fixed turn order for participants.
// Names must match registered participant names.
func WithTurnOrder(order []string) ModeratorOption {
	return func(m *Moderator) {
		m.turnOrder = order
	}
}

// Moderator orchestrates multi-round conversations between Participants.
//
// Each round:
//  1. Each participant receives messages from the channel
//  2. Each participant generates a response
//  3. Responses are broadcast to the channel
//  4. Termination condition is checked
type Moderator struct {
	name          string
	participants  []Participant
	maxRounds     int
	terminationFn func([]Envelope) bool
	turnOrder     []string
}

// NewModerator creates a new conversation moderator.
func NewModerator(name string, opts ...ModeratorOption) *Moderator {
	m := &Moderator{
		name:      name,
		maxRounds: 10,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// AddParticipant registers a participant in the conversation.
func (m *Moderator) AddParticipant(p Participant) {
	m.participants = append(m.participants, p)
}

// Run starts the conversation with the given topic and returns the full history.
func (m *Moderator) Run(ctx context.Context, topic string) ([]Envelope, error) {
	if len(m.participants) == 0 {
		return nil, fmt.Errorf("moderator %q: no participants", m.name)
	}

	channel := NewChannel()

	// Seed the conversation with the topic.
	channel.Send(Envelope{
		From:    "moderator",
		Content: topic,
		Round:   0,
	})

	// Determine turn order.
	order := m.turnOrder
	if len(order) == 0 {
		order = make([]string, len(m.participants))
		for i, p := range m.participants {
			order[i] = p.Name()
		}
	}

	// Build name→participant lookup.
	lookup := make(map[string]Participant, len(m.participants))
	for _, p := range m.participants {
		lookup[p.Name()] = p
	}

	for round := 1; round <= m.maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return channel.History(), err
		}

		for _, name := range order {
			p, ok := lookup[name]
			if !ok {
				continue
			}

			incoming := channel.Receive(name)
			response, err := p.Respond(ctx, incoming)
			if err != nil {
				return channel.History(), fmt.Errorf("moderator %q: participant %q round %d: %w",
					m.name, name, round, err)
			}

			response.From = name
			response.Round = round
			channel.Send(response)
		}

		// Check termination.
		if m.terminationFn != nil && m.terminationFn(channel.History()) {
			break
		}
	}

	return channel.History(), nil
}
