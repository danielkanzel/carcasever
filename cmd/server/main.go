package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"carcasever/internal/server"
	"carcasever/internal/store"
)

func main() {
	uri := env("MONGO_URI", "mongodb://127.0.0.1:27017")
	dbName := env("MONGO_DB", "carcasever")
	port := env("PORT", "8080")
	web := env("WEB_DIR", "web")

	var db *store.Mongo
	var err error
	for try := 1; try <= 30; try++ {
		db, err = store.Connect(context.Background(), uri, dbName)
		if err == nil {
			break
		}
		log.Printf("mongo недоступна (%d/30): %v", try, err)
		time.Sleep(time.Second)
	}
	if err != nil {
		log.Fatal(err)
	}

	srv := server.New(db, web)
	log.Printf("бесконечный каркассон слушает :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, srv.Handler()))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
