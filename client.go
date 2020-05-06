package sse

import (
	"github.com/google/uuid"
)

// Client represents a web browser connection.
type Client struct {
	uuid string
	lastEventID,
	channel string
	send chan *Message
}

func newClient(lastEventID, channel string) (*Client, error) {
	uuid, err := uuid.NewUUID()
	if err != nil {
		return nil, err
	}

	return &Client{
		uuid.String(),
		lastEventID,
		channel,
		make(chan *Message),
	}, nil
}

// SendMessage sends a message to client.
func (c *Client) SendMessage(message *Message) {
	c.lastEventID = message.id
	c.send <- message
}

// Channel returns the channel where this client is subscribe to.
func (c *Client) Channel() string {
	return c.channel
}

// LastEventID returns the ID of the last message sent.
func (c *Client) LastEventID() string {
	return c.lastEventID
}
