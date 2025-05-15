package generator

import (
	"fmt"
	"os"

	"github.com/TechXTT/TORM/pkg/torm/internal/metadata"
)

// Generate reads a Prisma schema and outputs Go client code.
func Generate(schemaPath, outDir string) error {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	ast, err := metadata.ParseSchema(data)
	if err != nil {
		return err
	}
	fmt.Printf("Parsed %d entities\n", len(ast.Entities))
	// TODO: write models under outDir/client
	return nil
}
