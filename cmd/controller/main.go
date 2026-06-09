package main

import (
	"log"
	"os"
	"strconv"

	"service-controller-notebookum/internal/config"
	"service-controller-notebookum/internal/consul"
	"service-controller-notebookum/internal/web"
)

func main() {
	cfg := config.Load()

	port, err := strconv.Atoi(cfg.Port)
	if err != nil {
		port = 5000
	}
	consul.RegisterController(cfg.ConsulURL, port)

	router := web.NewRouter(cfg)

	addr := ":" + cfg.Port
	if err := router.Run(addr); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
