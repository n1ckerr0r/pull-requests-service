package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBUrl string
	Port  string
}

func Load() Config {
	if err := godotenv.Load(); err != nil {
		log.Println(".env file was not found")
	}

	db, ok := os.LookupEnv("DATABASE_URL")
	if !ok {
		db, ok = os.LookupEnv("DB_URL")
		if !ok {
			log.Fatal("DATABASE_URL and DB_URL environment variables were not set")
		}
	}
	port, ok := os.LookupEnv("PORT")
	if !ok {
		port = "8080"
	}

	return Config{
		DBUrl: db,
		Port:  port,
	}
}
