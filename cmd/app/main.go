package main

import (
	"log"
	"os"

	"github.com/n1ckerr0r/pull-requests-service/internal/config"
	"github.com/n1ckerr0r/pull-requests-service/internal/store"
	httptr "github.com/n1ckerr0r/pull-requests-service/internal/transport/http"
)

func main() {
	cfg := config.Load()

	if cfg.DBUrl == "" {
		cfg.DBUrl = os.Getenv("DATABASE_URL")
	}
	if cfg.DBUrl == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	st, err := store.NewStore(cfg.DBUrl)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer st.DB().Close()

	r := httptr.NewRouter(st)
	log.Printf("listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
