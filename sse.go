package sse

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
)

var (
	errChannelWithoutClients = errors.New("channel has no clients")
)

// Server represents SSE server.
type Server struct {
	mu           sync.RWMutex
	options      *Options
	channels     map[string]*Channel
	addClient    chan *Client
	removeClient chan *Client
	shutdown     chan bool
	closeChannel chan string
}

// NewServer creates a new SSE server.
func NewServer(options *Options) *Server {
	if options == nil {
		options = &Options{
			Logger: log.New(os.Stdout, "go-sse: ", log.LstdFlags),
		}
	}

	if options.Logger == nil {
		options.Logger = log.New(ioutil.Discard, "", log.LstdFlags)
	}

	s := &Server{
		sync.RWMutex{},
		options,
		make(map[string]*Channel),
		make(chan *Client),
		make(chan *Client),
		make(chan bool),
		make(chan string),
	}

	go s.dispatch()

	return s
}

func (s *Server) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	flusher, ok := response.(http.Flusher)

	if !ok {
		http.Error(response, "Streaming unsupported.", http.StatusInternalServerError)
		return
	}

	h := response.Header()

	if s.options.hasHeaders() {
		for k, v := range s.options.Headers {
			h.Set(k, v)
		}
	}

	switch request.Method {
	case "GET":
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		h.Set("X-Accel-Buffering", "no")

		var channelName string

		if s.options.ChannelNameFunc == nil {
			channelName = request.URL.Path
		} else {
			channelName = s.options.ChannelNameFunc(request)
		}

		lastEventID := request.Header.Get("Last-Event-ID")

		c, err := newClient(lastEventID, channelName)
		if err != nil {
			http.Error(response, "Cannot create client.", http.StatusInternalServerError)

			return
		}

		if s.options.AuthorizationFunc != nil {
			if err = s.options.AuthorizationFunc(c.uuid, request); err != nil {
				http.Error(response, "Not authorized.", http.StatusUnauthorized)

				return
			}
		}

		s.addClient <- c
		closeNotify := request.Context().Done()

		go func() {
			<-closeNotify
			s.removeClient <- c
			s.options.DisconnectChan <- c.uuid
		}()

		response.WriteHeader(http.StatusOK)
		flusher.Flush()

		for msg := range c.send {
			msg.retry = s.options.RetryInterval
			fmt.Fprintf(response, msg.String())
			flusher.Flush()
		}

	case "OPTIONS":
		// Do nothing for OPTIONS request
	default:
		response.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// SendBroadcastMessage broadcast a message to all clients in a channel.
func (s *Server) SendBroadcastMessage(channel string, message *Message) error {
	if ch, ok := s.getChannel(channel); ok {
		ch.SendBroadcastMessage(message)
		s.options.Logger.Printf("message sent to channel '%s'.", channel)

		return nil
	}
	s.options.Logger.Printf("message not sent, channel '%s' has no clients.", channel)

	return errChannelWithoutClients
}

// SendMessageToClients broadcast a message to specific clients in a channel.
func (s *Server) SendMessageToClients(channel string, uuids []string, message *Message) error {
	if ch, ok := s.getChannel(channel); ok {
		ch.SendMessageToClients(message, uuids)
		s.options.Logger.Printf("message sent to channel '%s'.", channel)

		return nil
	}
	s.options.Logger.Printf("message not sent, channel '%s' has no clients.", channel)

	return errChannelWithoutClients
}

// Restart closes all channels and clients.
func (s *Server) Restart() {
	s.options.Logger.Print("restarting server.")
	s.close()
}

// Shutdown performs a graceful server shutdown.
func (s *Server) Shutdown() {
	s.shutdown <- true
}

// ClientCount returns the number of clients connected to this server.
func (s *Server) ClientCount() int {
	i := 0

	s.mu.RLock()

	for _, channel := range s.channels {
		i += channel.ClientCount()
	}

	s.mu.RUnlock()

	return i
}

// HasChannel returns true if the channel associated with name exists.
func (s *Server) HasChannel(name string) bool {
	_, ok := s.getChannel(name)

	return ok
}

// GetChannel returns the channel associated with name or nil if not found.
func (s *Server) GetChannel(name string) (*Channel, bool) {
	return s.getChannel(name)
}

// Channels returns a list of all channels to the server.
func (s *Server) Channels() []string {
	var channels []string

	s.mu.RLock()

	for name := range s.channels {
		channels = append(channels, name)
	}

	s.mu.RUnlock()

	return channels
}

// CloseChannel closes a channel.
func (s *Server) CloseChannel(name string) {
	s.closeChannel <- name
}

func (s *Server) addChannel(name string) *Channel {
	ch := newChannel(name)

	s.mu.Lock()
	s.channels[ch.name] = ch
	s.mu.Unlock()

	s.options.Logger.Printf("channel '%s' created.", ch.name)

	return ch
}

func (s *Server) removeChannel(ch *Channel) {
	s.mu.Lock()
	delete(s.channels, ch.name)
	s.mu.Unlock()

	ch.Close()

	s.options.Logger.Printf("channel '%s' closed.", ch.name)
}

func (s *Server) getChannel(name string) (*Channel, bool) {
	s.mu.RLock()
	ch, ok := s.channels[name]
	s.mu.RUnlock()

	return ch, ok
}

func (s *Server) close() {
	for _, ch := range s.channels {
		s.removeChannel(ch)
	}
}

func (s *Server) dispatch() {
	s.options.Logger.Print("server started.")

	for {
		select {

		// New client connected.
		case c := <-s.addClient:
			ch, exists := s.getChannel(c.channel)

			if !exists {
				ch = s.addChannel(c.channel)
			}

			ch.addClient(c)
			s.options.Logger.Printf("new client connected to channel '%s'.", ch.name)

		// Client disconnected.
		case c := <-s.removeClient:
			if ch, exists := s.getChannel(c.channel); exists {
				ch.removeClient(c.uuid)
				s.options.Logger.Printf("client disconnected from channel '%s'.", ch.name)

				if ch.ClientCount() == 0 {
					s.options.Logger.Printf("channel '%s' has no clients.", ch.name)
					s.removeChannel(ch)
				}
			}

		// Close channel and all clients in it.
		case channel := <-s.closeChannel:
			if ch, exists := s.getChannel(channel); exists {
				s.removeChannel(ch)
			} else {
				s.options.Logger.Printf("requested to close nonexistent channel '%s'.", channel)
			}

		// Event Source shutdown.
		case <-s.shutdown:
			s.close()
			close(s.addClient)
			close(s.removeClient)
			close(s.closeChannel)
			close(s.shutdown)

			s.options.Logger.Print("server stopped.")

			return
		}
	}
}
