package config

import "os"

type Config struct {
	Port   string
	DBPath string
}

func Load() Config {
	port := os.Getenv("CHURNDESK_PORT")
	if port == "" {
		port = "8080"
	}
	dbPath := os.Getenv("CHURNDESK_DB_PATH")
	if dbPath == "" {
		dbPath = "/data/churndesk.db"
	}
	return Config{Port: port, DBPath: dbPath}
}
