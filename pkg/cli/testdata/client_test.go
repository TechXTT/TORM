package client_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"example.test/generated/models"
	"github.com/TechXTT/TORM/v2/pkg/runtime"
)

func TestSchemaTypesAndRelations(t *testing.T) {
	ctx := context.Background()
	client, err := models.Open(os.Getenv("TORM_EXAMPLE_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	authors, _ := client.AuthorService()
	profiles, _ := client.ProfileService()
	tags, _ := client.TagService()
	entries, _ := client.EntryService()
	entryTags, _ := client.EntryTagService()
	author, err := authors.Insert(ctx, models.AuthorCreate{Contact: runtime.Set("author@example.test")})
	if err != nil || author.Id.Version() != 7 {
		t.Fatalf("UUID v7: %v, %v", author, err)
	}
	profile, err := profiles.Insert(ctx, models.ProfileCreate{UserId: runtime.Set(author.Id)})
	if err != nil || profile.Bio != nil {
		t.Fatalf("nullable profile: %v, %v", profile, err)
	}
	tag, err := tags.Insert(ctx, models.TagCreate{Name: runtime.Set("Topic")})
	if err != nil {
		t.Fatal(err)
	}
	if err := authors.ConnectTags(ctx, author.Id, tag.Id); err != nil {
		t.Fatal(err)
	}
	entry, err := entries.Insert(ctx, models.EntryCreate{
		Tenant:   runtime.Set(3),
		Slug:     runtime.Set("first"),
		Rank:     runtime.Set(int64(1<<53 + 1)),
		Amounts:  runtime.Set([]runtime.Decimal{"9.20", "0.00"}),
		AuthorId: runtime.Set(author.Id),
		Metadata: runtime.Set(json.RawMessage("{\"num\": 2}")),
		Contents: runtime.Set([]byte{0, 1, 255}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Visibility != models.VisibilityPUBLIC || entry.Rank != 1<<53+1 || entry.Labels[0] != "{ }" || entry.Amounts[0] != "9.20" ||
		entry.Metadata == nil || !json.Valid(*entry.Metadata) || entry.Contents == nil || (*entry.Contents)[2] != 255 {
		t.Fatalf("incorrect PostgreSQL scalar round-trip: %+v", entry)
	}
	if _, err := entryTags.Insert(ctx, models.EntryTagCreate{
		Tenant: runtime.Set(entry.Tenant), Slug: runtime.Set(entry.Slug), TagId: runtime.Set(tag.Id),
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := entries.Fetch(ctx, models.EntryByEntryKey(3, "first"), models.EntryInclude{Author: true, Uses: true})
	if err != nil || loaded.Author.Id != author.Id || len(loaded.Uses) != 1 || loaded.Uses[0].TagId != tag.Id {
		t.Fatalf("explicit composite-key relation: %+v, %v", loaded, err)
	}
	reverse, err := authors.Fetch(ctx, models.AuthorById(author.Id), models.AuthorInclude{Tags: true, Profile: true, Entries: true})
	if err != nil || reverse.Profile.Id != profile.Id || len(reverse.Tags) != 1 || reverse.Tags[0].Id != tag.Id || len(reverse.Entries) != 1 {
		t.Fatalf("author relations: %+v, %v", reverse, err)
	}
	if err := authors.DisconnectTags(ctx, author.Id, tag.Id); err != nil {
		t.Fatal(err)
	}
	if err := authors.DeleteBy(ctx, models.AuthorById(author.Id)); err != nil {
		t.Fatal(err)
	}
	loaded, err = entries.Fetch(ctx, models.EntryByEntryKey(3, "first"), models.EntryInclude{Author: true})
	if err != nil || loaded.Author != nil || loaded.AuthorId != nil {
		t.Fatalf("SetNull did not clear the key: %+v, %v", loaded, err)
	}
	if item, err := profiles.Fetch(ctx, models.ProfileById(profile.Id), models.ProfileInclude{}); err != nil || item != nil {
		t.Fatalf("cascade did not delete the profile: %+v, %v", item, err)
	}
}
