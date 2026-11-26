package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var Client *mongo.Client
var DB *mongo.Database

func Connect(mongoURI string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientOptions := options.Client().ApplyURI(mongoURI)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	Client = client
	DB = client.Database("huasteca")

	log.Println("Connected to MongoDB")
	return nil
}

func GetCollection(name string) *mongo.Collection {
	return DB.Collection(name)
}

func CreateIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	usersCollection := GetCollection("users")
	userEmailIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	if _, err := usersCollection.Indexes().CreateOne(ctx, userEmailIndex); err != nil {
		return fmt.Errorf("failed to create email index: %w", err)
	}

	indexes := []struct {
		collection string
		keys bson.D
	}{
		{"products", bson.D{{Key: "vendedor_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{"ventas", bson.D{{Key: "comprador_id", Value: 1}, {Key: "fecha", Value: -1}}},
		{"ventas", bson.D{{Key: "vendedor_id", Value: 1}, {Key: "fecha", Value: -1}}},
	}
	for _, idx := range indexes {
		if _, err := GetCollection(idx.collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: idx.keys}); err != nil {
			return fmt.Errorf("failed to create index on %s: %w", idx.collection, err)
		}
	}
	log.Println("Indexes created successfully")
	return nil
}

func Disconnect(ctx context.Context) error {
	if Client != nil {
		return Client.Disconnect(ctx)
	}
	return nil
}
