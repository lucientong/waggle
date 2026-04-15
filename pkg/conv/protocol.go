// Package conv provides a multi-agent conversation protocol.
//
// It enables structured conversations between multiple AI agents, with a
// Moderator coordinating turn-taking, message routing, and termination.
//
// Key concepts:
//   - Envelope: a message with sender, recipient, and content
//   - Channel: thread-safe message queue for agent communication
//   - Participant: an agent that can respond to conversation messages
//   - Moderator: orchestrates multi-round conversations between participants
package conv

import (
	"context"
	"sync"

	"github.com/lucientong/waggle/pkg/agent"
)

// Envelope represents a message in a multi-agent conversation.
type Envelope struct {
	// From is the sender's name.
	From string `json:"from"`
	// To is the recipient's name. Empty string means broadcast to all.
	To string `json:"to,omitempty"`
	// Content is the message text.
	Content string `json:"content"`
	// Round is the conversation round number (0-based).
	Round int `json:"round"`
}

// Channel is a thread-safe message queue for agent communication.
type Channel struct {
	mu       sync.RWMutex
	messages []Envelope
}

// NewChannel creates an empty communication channel.
func NewChannel() *Channel {
	return &Channel{}
}

// Send appends a message to the channel.
func (c *Channel) Send(env Envelope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, env)
}

// Receive returns all messages addressed to the given participant (or broadcast).
func (c *Channel) Receive(participantName string) []Envelope {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var result []Envelope
	for _, m := range c.messages {
		if m.To == "" || m.To == participantName {
			result = append(result, m)
		}
	}
	return result
}

// History returns a copy of all messages in the channel.
func (c *Channel) History() []Envelope {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Envelope, len(c.messages))
	copy(out, c.messages)
	return out
}

// Clear removes all messages from the channel.
func (c *Channel) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = c.messages[:0]
}

// Participant is an agent that can participate in multi-agent conversations.
type Participant interface {
	// Name returns the participant's identifier.
	Name() string

	// Respond generates a response given the incoming messages.
	Respond(ctx context.Context, incoming []Envelope) (Envelope, error)
}

// FuncParticipant creates a Participant from a function.
func FuncParticipant(name string, fn func(ctx context.Context, incoming []Envelope) (string, error)) Participant {
	return &funcParticipant{name: name, fn: fn}
}

type funcParticipant struct {
	name string
	fn   func(ctx context.Context, incoming []Envelope) (string, error)
}

func (p *funcParticipant) Name() string { return p.name }

func (p *funcParticipant) Respond(ctx context.Context, incoming []Envelope) (Envelope, error) {
	content, err := p.fn(ctx, incoming)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{From: p.name, Content: content}, nil
}

// AsAgent wraps a Moderator as an Agent[string, []Envelope] for integration
// with waggle's Chain, Parallel, and DAG patterns.
func AsAgent(m *Moderator) agent.Agent[string, []Envelope] {
	return &moderatorAgent{mod: m}
}

type moderatorAgent struct {
	mod *Moderator
}

func (a *moderatorAgent) Name() string { return a.mod.name }

func (a *moderatorAgent) Run(ctx context.Context, topic string) ([]Envelope, error) {
	return a.mod.Run(ctx, topic)
}
