package sse

import (
	"fmt"
	"testing"
)

func TestSetLastId(t *testing.T) {
	c, err := newClient("", "channel")
	if err != nil {
		t.Fatal("Cannot create client.")
	}

	go func() {
		for msg := range c.send {
			fmt.Printf("Message ID: %s\n", msg.id)
		}
	}()

	c.SendMessage(NewMessage("id", "data", "event"))

	if c.LastEventID() != "id" {
		t.Fatal("Wrong Last ID.")
	}
}
