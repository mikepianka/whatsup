package main

import (
	"fmt"
	"log"
	"os"

	"github.com/mikepianka/whatsup"
)

func main() {
	cfgData, err := os.ReadFile("config.json")
	if err != nil {
		log.Fatalf("Error reading config.json: %v", err)
	}

	cfg, err := whatsup.ParseConfig(cfgData)
	if err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}

	err = whatsup.Sup(cfg)
	if err != nil {
		log.Fatalf("Error checking endpoints: %v", err)
	}

	fmt.Printf("Checked %d endpoints.\n", len(cfg.Endpoints))
}
