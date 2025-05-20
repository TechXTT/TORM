// pkg/prisma/client.go
package prisma

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/TechXTT/TORM/pkg/config"
	_ "github.com/lib/pq"
)

// Client wraps a sql.DB connection.
type Client struct {
	DB *sql.DB
}

// NewClient constructs a DB client by reading the DSN
// from prisma/schema.prisma (fallback to env if specified).
func NewClient() *Client {
	config, err := config.Load("prisma/schema.prisma")
	if err != nil {
		panic(fmt.Sprintf("prisma: %v", err))
	}
	db, err := sql.Open("postgres", config.DSN)
	if err != nil {
		panic(fmt.Sprintf("failed to open DB: %v", err))
	}
	return &Client{DB: db}
}

// Connect verifies the database connection.
func (c *Client) Connect(ctx context.Context) error {
	return c.DB.PingContext(ctx)
}

// Close closes the database connection.
func (c *Client) Close(ctx context.Context) error {
	return c.DB.Close()
}
