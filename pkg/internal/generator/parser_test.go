package generator

import (
	"testing"
)

func TestParseSchema_SimpleModels(t *testing.T) {
	raw := []byte(`
		// Comment line should be ignored
		model User {
			id        String   @id @default(uuid()) @db.Uuid
			name      String
			age       Int?
			createdAt DateTime @default(now())
			profile   Profile @relation(fields: [profileId], references: [id])
			profileId String   @db.Uuid
		}

		model Profile {
			id     String @id @default(uuid()) @db.Uuid
			bio    String?
		}
	`)

	ast, err := ParseSchema(raw)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// We expect two entities: User and Profile
	if len(ast.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(ast.Entities))
	}

	// Find User entity
	var userEntity Entity
	for _, e := range ast.Entities {
		if e.Name == "User" {
			userEntity = e
			break
		}
	}
	if userEntity.Name != "User" {
		t.Fatal("User entity not found")
	}

	// Verify fields within User
	expectedFields := []Field{
		{
			Name:       "id",
			Type:       "uuid.UUID",
			PrimaryKey: true,
			Default: func() *string {
				s := "uuid()"
				return &s
			}(),
		},
		{
			Name: "name",
			Type: "string",
		},
		{
			Name: "age",
			Type: "int",
		},
		{
			Name:    "createdAt",
			Type:    "time.Time",
			Default: func() *string { s := "now()"; return &s }(),
			NotNull: true,
		},
		// Note: "profile" relation is skipped by the parser (contains @relation)
		{
			Name: "profileId",
			Type: "uuid.UUID",
		},
	}

	if len(userEntity.Fields) != len(expectedFields) {
		t.Fatalf("expected %d fields for User, got %d", len(expectedFields), len(userEntity.Fields))
	}

	for i, f := range userEntity.Fields {
		exp := expectedFields[i]
		if f.Name != exp.Name {
			t.Errorf("field[%d].Name = %q, want %q", i, f.Name, exp.Name)
		}
		if f.Type != exp.Type {
			t.Errorf("field[%d].Type = %q, want %q", i, f.Type, exp.Type)
		}
		if f.PrimaryKey != exp.PrimaryKey {
			t.Errorf("field[%d].PrimaryKey = %v, want %v", i, f.PrimaryKey, exp.PrimaryKey)
		}
		if (f.Default == nil) != (exp.Default == nil) {
			t.Errorf("field[%d].Default nil mismatch: got %v, want %v", i, f.Default == nil, exp.Default == nil)
		} else if f.Default != nil && exp.Default != nil && *f.Default != *exp.Default {
			t.Errorf("field[%d].Default = %q, want %q", i, *f.Default, *exp.Default)
		}
	}

	// Verify Profile entity
	var profileEntity Entity
	for _, e := range ast.Entities {
		if e.Name == "Profile" {
			profileEntity = e
			break
		}
	}
	if profileEntity.Name != "Profile" {
		t.Fatal("Profile entity not found")
	}
	if len(profileEntity.Fields) != 2 {
		t.Fatalf("expected 2 fields for Profile, got %d", len(profileEntity.Fields))
	}

	// Check primary key and optional bio
	if profileEntity.Fields[0].Name != "id" || profileEntity.Fields[0].Type != "uuid.UUID" {
		t.Errorf("Profile.id parsed incorrectly: %+v", profileEntity.Fields[0])
	}
	if profileEntity.Fields[1].Name != "bio" || profileEntity.Fields[1].Type != "string" {
		t.Errorf("Profile.bio parsed incorrectly: %+v", profileEntity.Fields[1])
	}
}
