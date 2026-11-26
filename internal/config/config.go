package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	MongoURI         string
	JWT_SECRET       string
	GOOGLE_CLIENT_ID string
	PORT             string
	CORS_ORIGINS     string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	return &Config{
		MongoURI: requireEnv("MONGO_URI"),
		JWT_SECRET: requireEnv("JWT_SECRET"),
		GOOGLE_CLIENT_ID: getEnv("GOOGLE_CLIENT_ID", ""),
		PORT:             getEnv("PORT", "8080"),
		CORS_ORIGINS:     getEnv("CORS_ORIGINS", "http://localhost:4200"),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func (c *Config) GetMongoURI() string {
	return c.MongoURI
}

func (c *Config) GetJWTKey() []byte {
	return []byte(c.JWT_SECRET)
}

func (c *Config) GetGoogleClientID() string {
	return c.GOOGLE_CLIENT_ID
}

func (c *Config) GetServerAddress() string {
	return fmt.Sprintf(":%s", c.PORT)
}

func (c *Config) GetCORSOrigins() []string {
	parts := strings.Split(c.CORS_ORIGINS, ",")
	origins := make([]string, 0, len(parts))
	for _, origin := range parts {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
func requireEnv(key string) string {
    value := strings.TrimSpace(os.Getenv(key))

    if value == "" {
        log.Fatalf("Falta configurar la variable requerida: %s", key)
    }

    return value
}
