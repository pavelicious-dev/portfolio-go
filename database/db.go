package database

import (
	"context"
	"os"
	"time"

	"github.com/gofiber/fiber/v3/log"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var client *mongo.Client

func GetDB() *mongo.Database {
	return client.Database("ib")
}

func Connect() error {
	uri := os.Getenv("MONGODB_URI")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connection, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	client = connection

	log.Info("Connected to MongoDB")
	return err
}

func Close() error {
	defer log.Info("Disconnected from MongoDB")

	return client.Disconnect(context.Background())
}
