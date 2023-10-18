package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/Hytm/demo-app-ws/pkg/faker"
	"github.com/Hytm/demo-app-ws/pkg/status"
	"github.com/Hytm/demo-app-ws/pkg/websocket"

	"github.com/joho/godotenv"
)

const (
	CONCURRENCY = "CONCURRENCY"
	WAIT        = "WAIT"
	DB          = "DB"

	DOMAIN = "https://demoapp.bid/"
)

var (
	f *faker.Faker
	s *status.Status
)

func serveWs(pool *websocket.Pool, w http.ResponseWriter, r *http.Request) {
	fmt.Println("WebSocket Endpoint Hit")
	ws, err := websocket.Upgrade(w, r)
	if err != nil {
		fmt.Fprintf(w, "%+V\n", err)
	}

	client := &websocket.Client{
		Conn: ws,
		Pool: pool,
	}

	pool.Register <- client
	client.Read()
}

func setupRoutes() {
	pool := websocket.NewPool()
	go pool.Start()

	c, err := strconv.Atoi(os.Getenv(CONCURRENCY))
	if err != nil {
		c = faker.DefaultConcurrency
	}
	w, err := strconv.Atoi(os.Getenv(WAIT))
	if err != nil {
		w = faker.DefaultWait
	}

	s = status.NewStatus()
	go s.RunHealthCheck()

	f = faker.NewFaker(c, w, pool, os.Getenv(DB))
	go f.Start()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(pool, w, r)
	})

	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Stopping the faker")
		f.Stop()
		http.Redirect(w, r, DOMAIN, http.StatusSeeOther)
	})

	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Starting the faker with concurrency: ", f.Concurrency, " and wait: ", f.Wait)
		go f.Start()
		http.Redirect(w, r, DOMAIN, http.StatusSeeOther)

	})

	fs := http.FileServer(http.Dir("./frontend"))
	http.Handle("/", fs)
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	fmt.Println("Cockroaches and Gophers!")
	setupRoutes()

	http.ListenAndServe(":8080", nil)
}
