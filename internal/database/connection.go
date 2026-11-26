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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	usersCollection := GetCollection("users")
	emailIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	if _, err := usersCollection.Indexes().CreateOne(ctx, emailIndex); err != nil {
		return fmt.Errorf("failed to create email index: %w", err)
	}

	productsCollection := GetCollection("products")
	productIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "vendedor_id", Value: 1}}},
	}
	if _, err := productsCollection.Indexes().CreateMany(ctx, productIndexes); err != nil {
		return fmt.Errorf("failed to create product indexes: %w", err)
	}

	ventasCollection := GetCollection("ventas")
	ventaIndexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "comprador_id", Value: 1}, {Key: "fecha", Value: -1}}},
		{Keys: bson.D{{Key: "vendedor_id", Value: 1}, {Key: "fecha", Value: -1}}},
	}
	if _, err := ventasCollection.Indexes().CreateMany(ctx, ventaIndexes); err != nil {
		return fmt.Errorf("failed to create sales indexes: %w", err)
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
