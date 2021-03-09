package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aliakseiz/go-sse"
)

func main() {
	disconnectChan := make(chan string, 1)

	s := sse.NewServer(&sse.Options{
		// Increase default retry interval to 10s.
		RetryInterval: 10 * 1000,
		// CORS headers
		Headers: map[string]string{
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Methods": "GET, OPTIONS",
			"Access-Control-Allow-Headers": "Keep-Alive,X-Requested-With,Cache-Control,Content-Type,Last-Event-ID",
		},
		// Custom channel name generator
		ChannelNameFunc: func(request *http.Request) string {
			return request.URL.Path
		},
		// Print debug info
		Logger:            log.New(os.Stdout, "go-sse: ", log.Ldate|log.Ltime|log.Lshortfile),
		AuthorizationFunc: AuthHandler,
		DisconnectChan:    disconnectChan,
	})

	defer s.Shutdown()

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.Handle("/events/", s)

	go DisconnectHandler(disconnectChan)

	go func() {
		for {
			msg := sse.SimpleMessage(time.Now().Format("2006/02/01/ 15:04:05"))

			if err := s.SendMessage("/events/channel-1", msg); err != nil {
				log.Printf("Failed to send broadcast message: %s", err.Error())
			}

			time.Sleep(5 * time.Second)
		}
	}()

	go func() {
		i := 0
		for {
			i++

			if err := s.SendMessage("/events/channel-2", sse.SimpleMessage(strconv.Itoa(i))); err != nil {
				log.Printf("Failed to send broadcast message: %s", err.Error())
			}

			time.Sleep(5 * time.Second)
		}
	}()

	log.Println("Listening at :3000")
	http.ListenAndServe(":3000", nil)
}

// AuthHandler mock authorization
func AuthHandler(uuid string, req *http.Request) error {
	log.Printf("Client %s authorized\n", uuid)

	return nil
}

// DisconnectHandler mock disconnection handler
func DisconnectHandler(uuids <-chan string) {
	for {
		log.Printf("Client %s disconnected\n", <-uuids)
	}
}
