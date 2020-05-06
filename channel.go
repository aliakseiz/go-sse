package sse

import (
	"sync"
)

// Channel represents a server sent events channel.
type Channel struct {
	name        string
	lastEventID string
	clients     struct {
		sync.RWMutex                    // mutex to control `m` map usage
		m            map[string]*Client // key:client UUID, value:Client
	}
}

func newChannel(name string) *Channel {
	return &Channel{
		name:        name,
		lastEventID: "",
		clients: struct {
			sync.RWMutex
			m map[string]*Client
		}{sync.RWMutex{},
			make(map[string]*Client),
		},
	}
}

// SendBroadcastMessage broadcast a message to all clients in a channel.
func (c *Channel) SendBroadcastMessage(message *Message) {
	c.lastEventID = message.id

	c.clients.RLock()

	for _, client := range c.clients.m {
		client.send <- message
	}

	c.clients.RUnlock()
}

// SendMessageToClients broadcast a message to specific clients in a channel.
func (c *Channel) SendMessageToClients(message *Message, uuids []string) {
	c.lastEventID = message.id

	c.clients.RLock()

	for _, uuid := range uuids {
		if client, ok := c.clients.m[uuid]; ok {
			client.send <- message
		}
	}

	c.clients.RUnlock()
}

// Close the channel and disconnect all clients.
func (c *Channel) Close() {
	for uuid := range c.clients.m {
		c.removeClient(uuid)
	}
}

// ClientCount returns the number of clients connected to this channel.
func (c *Channel) ClientCount() int {
	c.clients.RLock()

	count := len(c.clients.m)

	c.clients.RUnlock()

	return count
}

// LastEventID returns the ID of the last message sent.
func (c *Channel) LastEventID() string {
	return c.lastEventID
}

func (c *Channel) addClient(client *Client) {
	c.clients.Lock()

	c.clients.m[client.uuid] = client

	c.clients.Unlock()
}

func (c *Channel) removeClient(uuid string) {
	if client, ok := c.clients.m[uuid]; ok {
		close(client.send)

		c.clients.Lock()

		delete(c.clients.m, uuid)

		c.clients.Unlock()
	}
}
