package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
)

type Handshake struct {
	MsgType string `json:"type"`
	Session string `json:"session"`
	Party   bool   `json:"_party"`
}

func redisWatcher(clients map[*websocket.Conn]string, sessionSubChannel <-chan *redis.Message) {
	for {
		for msg := range sessionSubChannel {
			uuid := msg.Channel[8:len(msg.Channel)]
			log.Printf("received message from %v: %v\n", uuid, msg.Payload)

			for client, clientUuid := range clients {
				if uuid != clientUuid {
					continue
				}

				if err := client.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
					log.Printf("failed to send message to %v: %v\n", uuid, err)
				}

				log.Printf("sent message to %v: %v", uuid, msg.Payload)
			}
		}
	}
}

func wsWatcher(clients map[*websocket.Conn]string, ws *websocket.Conn) {
	// Close websocket after watcher ends
	defer ws.Close()

	// Make sure we close the connection when the function returns
	for {
		var msg Handshake

		// Read in a new message as JSON and try to map it to a Handshake object
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("got error while reading ws message %v\n", err)
			break
		}

		log.Printf("client connected: %v\n", msg)
		clients[ws] = msg.Session
		ws.SetCloseHandler(func(code int, text string) error {
			delete(clients, ws)
			return nil
		})
	}
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

	log.Printf("connected to redis %v\n", *redisAddr)

	// Go channel which receives messages.
	sessionSubChannel := sessionSub.Channel()

	// Start listening for redis messages
	go redisWatcher(clients, sessionSubChannel)

	// Create ws service
	upgrader := websocket.Upgrader{}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Get ws connection from request
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("unable to create websocket: %v\n", err)
		}

		log.Printf("got ws connection from %v\n", ws.RemoteAddr())

		// Start listening for websocket messages
		go wsWatcher(clients, ws)
	})

	log.Printf("starting http server on %v\n", *listenAddr)
	if err := http.ListenAndServe(*listenAddr, nil); err != nil {
		log.Fatalf("unable to start http server: %v", err)
	}
}
