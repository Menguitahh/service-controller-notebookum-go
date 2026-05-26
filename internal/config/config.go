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
}
//eliminar monolith url
func Load() Config {
	return Config{
		Port:                     env("PORT", "5000"),
		CorrelationHeader:        env("X_CORRELATION_HEADER", "X-Correlation-ID"),
		UserIDHeader:             env("X_USER_ID_HEADER", "X-User-ID"),		
		ExtractorURL:             env("EXTRACTOR_URL", "https://extractor.universidad.localhost"),
		AIURL:                    env("AI_URL", "https://ai.universidad.localhost"),
		PersistenceURL:           env("PERSISTENCE_URL", "https://persistence.universidad.localhost"),
		UserServiceURL:           env("USER_SERVICE_URL", "https://users.universidad.localhost"),
	}
}

