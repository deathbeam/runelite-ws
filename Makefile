all:
	go get github.com/gorilla/websocket
	go get github.com/go-redis/redis
	go build -o runelite-ws main.go
