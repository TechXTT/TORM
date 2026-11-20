package config

import (
	"fmt"
	"os"

	"github.com/TechXTT/TORM/v2/pkg/internal/schema"
	"github.com/joho/godotenv"
)

type Config struct {
	DSN        string
	SchemaPath string
	// Deprecated: use SchemaPath.
	SchemaDir string
}

func Load(schemaPath string) (*Config, error) {
	source, err := schema.Load(schemaPath, false)
	if err != nil {
		return nil, err
	}
	LoadEnv()
	dsn, err := ResolveDSN("", source.Source.Environment, source.Source.URL)
	if err != nil {
		return nil, err
	}
	return &Config{DSN: dsn, SchemaPath: schemaPath, SchemaDir: schemaPath}, nil
}

func LoadEnv() {
	_ = godotenv.Load()
}

func ResolveDSN(override, legacyEnv, legacyURL string) (string, error) {
	for _, candidate := range []string{override, os.Getenv("TORM_DATABASE_URL"), os.Getenv("DATABASE_URL"), os.Getenv(legacyEnv), legacyURL} {
		if candidate != "" {
			return candidate, nil
		}
	}
	if legacyEnv != "" {
		return "", fmt.Errorf("set %s, TORM_DATABASE_URL, or DATABASE_URL", legacyEnv)
	}
	return "", fmt.Errorf("set TORM_DATABASE_URL or DATABASE_URL")
}
