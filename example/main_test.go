package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/TechXTT/TORM/example/models"
	"github.com/TechXTT/TORM/v2/pkg/runtime"
)

func TestGeneratedClient(t *testing.T) {
	dsn := os.Getenv("TORM_EXAMPLE_DATABASE_URL")
	if dsn == "" {
		t.Skip("TORM_EXAMPLE_DATABASE_URL is required for example integration")
	}
	ctx := context.Background()
	client, err := models.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	creators, err := client.CreatorService()
	if err != nil {
		t.Fatal(err)
	}
	posts, err := client.PostService()
	if err != nil {
		t.Fatal(err)
	}
	creator, err := creators.Insert(ctx, models.CreatorCreate{
		Username:  runtime.Set("ada"),
		Email:     runtime.Set("ada@example.test"),
		FirstName: runtime.Set(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if creator.FirstName == nil || *creator.FirstName != "" {
		t.Fatal("expected present empty string")
	}
	updated, err := creators.Patch(ctx, models.CreatorById(creator.Id), models.CreatorUpdate{FirstName: runtime.Null[string]()})
	if err != nil || updated.FirstName != nil || updated.UpdatedAt.Before(creator.UpdatedAt) {
		t.Fatalf("null patch or updatedAt failed: %v, %v", updated, err)
	}
	post, err := posts.Insert(ctx, models.PostCreate{
		Title:    runtime.Set("TORM"),
		Content:  runtime.Set(""),
		AuthorId: runtime.Set(creator.Id),
	})
	if err != nil || post.Published || post.Content != "" {
		t.Fatalf("insert/default failed: %v, %v", post, err)
	}
	loaded, err := creators.Fetch(ctx, models.CreatorByEmail(creator.Email), models.CreatorInclude{Posts: true})
	if err != nil || len(loaded.Posts) != 1 || loaded.Posts[0].Id != post.Id {
		t.Fatalf("creator relation: %v, %v", loaded, err)
	}
	matches, err := posts.Find(ctx, models.PostQuery{
		FindOptions: runtime.FindOptions[models.Post]{
			Filter: models.PostFields.Published.Eq(false),
			Order:  []runtime.Order[models.Post]{models.PostFields.Title.Asc()},
			Limit:  2,
		},
		Include: models.PostInclude{Author: true},
	})
	if err != nil || len(matches) != 1 || matches[0].Author.Id != creator.Id {
		t.Fatalf("post relation or filter: %v, %v", matches, err)
	}
	if _, err := posts.FindMany(ctx, map[string]any{"unknown": true}, nil, 0, 0); err == nil {
		t.Fatal("legacy filters should validate field names")
	}
	if err := creators.DeleteBy(ctx, models.CreatorById(creator.Id)); err == nil {
		t.Fatal("author relation must prevent deletion with dependent posts")
	}
	revert := errors.New("rollback example changes")
	err = client.Transaction(ctx, func(tx *models.Client) error {
		scoped, err := tx.PostService()
		if err != nil {
			return err
		}
		if err := scoped.DeleteBy(ctx, models.PostById(post.Id)); err != nil {
			return err
		}
		return revert
	})
	if !errors.Is(err, revert) {
		t.Fatalf("transaction returned %v", err)
	}
	count, err := posts.CountWhere(ctx, models.PostFields.Id.Eq(post.Id))
	if err != nil || count != 1 {
		t.Fatalf("deletion escaped rollback: %d, %v", count, err)
	}
	result := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := creators.UpsertBy(ctx, models.CreatorByEmail("concurrent@example.test"),
				models.CreatorCreate{Username: runtime.Set("concurrent")},
				models.CreatorUpdate{Username: runtime.Set("concurrent")})
			result <- err
		}()
	}
	for range 2 {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	count, err = creators.CountWhere(ctx, models.CreatorFields.Email.Eq("concurrent@example.test"))
	if err != nil || count != 1 {
		t.Fatalf("upsert did not converge: %d, %v", count, err)
	}
}
