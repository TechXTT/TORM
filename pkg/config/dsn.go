package config

import (
	"io/ioutil"
	"log"
	"os"
	"regexp"

	"github.com/joho/godotenv"
)

// Config holds all settings for migrate and codegen.
type Config struct {
	DSN         string
	SchemaPath  string
	SchemaDir   string
	ModelOutDir string
}

// Load reads DSN from Prisma schema and fills defaults.
func Load(schemaFile string) (*Config, error) {
	data, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`url\s*=\s*(?:env\("([^"]+)"\)|"([^"]+)")`)
	m := re.FindStringSubmatch(string(data))
	godotenv.Load()
	var dsn string
	if len(m) == 3 {
		if m[1] != "" {
			dsn = os.Getenv(m[1])
		} else {
			dsn = m[2]
		}
	} else {
		log.Fatalf("could not parse datasource url from schema: %s", schemaFile)
	}

	return &Config{
		DSN:         dsn,
		SchemaPath:  schemaFile,
		SchemaDir:   "prisma/schema.prisma",
		ModelOutDir: "models",
	}, nil
}
