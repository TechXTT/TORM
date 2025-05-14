// File: internal/plugin/hooks.go
package plugin

import "context"

// Hooks defines lifecycle callbacks for entity operations
type Hooks interface {
	BeforeCreate(ctx context.Context, entity interface{}) error
	AfterCreate(ctx context.Context, entity interface{}) error
	BeforeUpdate(ctx context.Context, entity interface{}) error
	AfterUpdate(ctx context.Context, entity interface{}) error
	BeforeDelete(ctx context.Context, entity interface{}) error
	AfterDelete(ctx context.Context, entity interface{}) error
}
