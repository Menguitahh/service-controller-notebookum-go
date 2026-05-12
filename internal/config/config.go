package config

import "os"

type Config struct {
	Port                     string
	CorrelationHeader        string
	UserIDHeader             string
	MonolithURL              string 
	ExtractorURL             string
	AIURL                    string
	PersistenceURL           string
	UserServiceURL           string
	StranglerEnableMSRouting bool
}
#eliminar monolith url
func Load() Config {
	return Config{
		Port:                     env("PORT", "5000"),
		CorrelationHeader:        env("X_CORRELATION_HEADER", "X-Correlation-ID"),
		UserIDHeader:             env("X_USER_ID_HEADER", "X-User-ID"),
		MonolithURL:              env("MONOLITH_URL", "http://monolith:8000"),
		ExtractorURL:             env("EXTRACTOR_URL", "http://extractor:5001"),
		AIURL:                    env("AI_URL", "http://ai:5002"),
		PersistenceURL:           env("PERSISTENCE_URL", "http://persistence:5003"),
		UserServiceURL:           env("USER_SERVICE_URL", "http://user-service:5004"),
		StranglerEnableMSRouting: env("STRANGLER_ENABLE_MS_ROUTING", "false") == "true",
	}
}
#no tener en cuenta los certificados de los servicios
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
