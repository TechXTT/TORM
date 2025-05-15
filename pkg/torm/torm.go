package torm

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/TechXTT/TORM/pkg/torm/generator"
	"github.com/TechXTT/TORM/pkg/torm/runtime"
)

// RunMigrate handles the "torm migrate" command.
func RunMigrate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: torm migrate <dev|deploy|reset|status> [--dir <migrations-dir>]")
		os.Exit(1)
	}
	sub := args[0]
	fs := flag.NewFlagSet("migrate "+sub, flag.ExitOnError)
	dir := fs.String("dir", "migrations", "migrations directory")
	fs.Parse(args[1:])

	db, err := runtime.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	mgr, err := runtime.NewManager(db, *dir)
	if err != nil {
		log.Fatalf("failed to load migrations: %v", err)
	}

	switch sub {
	case "dev":
		if err := mgr.Up(); err != nil {
			log.Fatalf("migrate dev failed: %v", err)
		}
	case "deploy":
		if err := mgr.Up(); err != nil {
			log.Fatalf("migrate deploy failed: %v", err)
		}
	case "reset":
		if err := mgr.Down(); err != nil {
			log.Fatalf("migrate reset (down) failed: %v", err)
		}
		if err := mgr.Up(); err != nil {
			log.Fatalf("migrate reset (up) failed: %v", err)
		}
	case "status":
		state, err := mgr.Status()
		if err != nil {
			log.Fatalf("migrate status failed: %v", err)
		}
		fmt.Println(state)
	default:
		fmt.Fprintf(os.Stderr, "Unknown migrate subcommand %q\n", sub)
		os.Exit(1)
	}
}

// RunDB handles the "torm db" command.
func RunDB(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: torm db <push|pull|drop> [--schema <schema>]")
		os.Exit(1)
	}
	sub := args[0]
	fs := flag.NewFlagSet("db "+sub, flag.ExitOnError)
	schemaFile := fs.String("schema", "schema.prisma", "schema file path")
	fs.Parse(args[1:])

	switch sub {
	case "push":
		log.Printf("Executing db push against schema %s", *schemaFile)
		// TODO: implement push
	case "pull":
		log.Printf("Executing db pull against schema %s", *schemaFile)
		// TODO: implement pull
	case "drop":
		log.Printf("Dropping database as per schema %s", *schemaFile)
		// TODO: implement drop
	default:
		fmt.Fprintf(os.Stderr, "Unknown db subcommand %q\n", sub)
		os.Exit(1)
	}
}

// RunGenerate handles the "torm generate" command.
func RunGenerate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	schemaFile := fs.String("schema", "schema.prisma", "schema file path")
	outDir := fs.String("out", "client", "output directory")
	fs.Parse(args)

	log.Printf("Generating Go client from schema %s into %s", *schemaFile, *outDir)
	if err := generator.Generate(*schemaFile, *outDir); err != nil {
		log.Fatalf("code generation failed: %v", err)
	}

	fmt.Println("Code generation complete.")
	fmt.Println("Formatting generated code...")
	fmtCmd := exec.Command("go", "fmt", "./"+*outDir)
	fmtCmd.Stdout = os.Stdout
	fmtCmd.Stderr = os.Stderr
	if err := fmtCmd.Run(); err != nil {
		log.Fatalf("go fmt failed: %v", err)
	}

	fmt.Println("Running go mod tidy...")
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		log.Fatalf("go mod tidy failed: %v", err)
	}
}

// RunStudio handles the "torm studio" command.
func RunStudio() {
	log.Println("Launching TORM Studio...")
	cmd := exec.Command("torm-studio")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("failed to launch studio: %v", err)
	}
}
