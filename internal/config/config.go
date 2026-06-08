package config

import "os"

type Config struct {
	Port              string
	CorrelationHeader string
	UserIDHeader      string
	ExtractorURL      string
	AIURL             string
	PersistenceURL    string
	UserServiceURL    string
	RedisHost         string
	RedisPort         string
	RedisPassword     string
}

func Load() Config {
	return Config{
		Port:              env("PORT", "5000"),
		CorrelationHeader: env("X_CORRELATION_HEADER", "X-Correlation-ID"),
		UserIDHeader:      env("X_USER_ID_HEADER", "X-User-ID"),
		ExtractorURL:      env("EXTRACTOR_URL", "http://extractor.universidad.localhost:5000"),
		AIURL:             env("AI_URL", "http://ai.universidad.localhost:5000"),
		PersistenceURL:    env("PERSISTENCE_URL", "http://persistence-java.universidad.localhost:8080"),
		UserServiceURL:    env("USER_SERVICE_URL", "http://users.universidad.localhost:5000"),
		RedisHost:         env("REDIS_HOST", ""),
		RedisPort:         env("REDIS_PORT", "6379"),
		RedisPassword:     env("REDIS_PASSWORD", ""),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}