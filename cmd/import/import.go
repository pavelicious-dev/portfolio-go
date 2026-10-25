package main

import (
	"context"
	"log"
	"os"

	"portfolio/config"
	"portfolio/database"
	"portfolio/ingest"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if len(os.Args) != 2 {
		log.Fatal("usage: import <folder with .qfx files>")
	}

	db, err := database.Open(config.DBPath())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = ingest.ImportFolder(context.Background(), db, os.Args[1], ingest.Options{
		BaseCurrency: config.BaseCurrency(),
	})
	if err != nil {
		log.Fatal(err)
	}
}
