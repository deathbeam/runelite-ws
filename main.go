package main

import (
	"flag"
	"log"
	"net/http"
)
import "github.com/gorilla/websocket"
import "github.com/go-redis/redis"

type Handshake struct {
	msgType string `json:"type"`
	session string `json:"session"`
}

func main() {
	// Parse CLI arguments
	listenAddr := flag.String("listenaddr", ":8081", "listen address eg :8081")
	redisAddr := flag.String("redisaddr", "127.0.0.1:6379", "redis address eg 127.0.0.1:6379")
	flag.Parse()

	// Create session clients
	clients := make(map[*websocket.Conn]string)

	// Create redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: *redisAddr,
	})

	if _, err := redisClient.Ping().Result(); err != nil {
		log.Fatalf("unable to connect to redis: %v", err)
	}

	sessionSub := redisClient.PSubscribe("session.*")
	defer sessionSub.Close()

	// Wait for confirmation that subscription is created before publishing anything.
	_, err := sessionSub.Receive()
	if err != nil {
		log.Fatalf("failed to create subscription for session.*: %v", err)
	}

	// Go channel which receives messages.
	sessionSubChannel := sessionSub.Channel()

	// Start listening
	go func() {
		for {
			for msg := range sessionSubChannel {
				uuid := msg.Channel[8:len(msg.Channel)]
				log.Printf("received message from %v: %v", uuid, msg.Payload)

				for client, clientUuid := range clients {
					if uuid != clientUuid {
						continue
					}

					if err := client.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
						log.Printf("failed to send message to %v: %v", uuid, err)
					}
				}
			}
		}
	}()

	upgrader := websocket.Upgrader{}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Get ws connection from request
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Fatalf("unable to create websocket: %v", err)
		}

		// Make sure we close the connection when the function returns
		for {
			var msg Handshake

			// Read in a new message as JSON and try to map it to a Handshake object
			err := ws.ReadJSON(&msg)
			if err != nil {
				delete(clients, ws)
				break
			}

			log.Printf("client connected: %v\n", msg.session)
			clients[ws] = msg.session
		}
	})

	if err := http.ListenAndServe(*listenAddr, nil); err != nil {
		log.Fatalf("unable to start http server: %v", err)
	}

	log.Printf("http server started on %v\n", listenAddr)
}
