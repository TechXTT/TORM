package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/TechXTT/TORM/example/models"
	"github.com/TechXTT/TORM/v2/pkg/runtime"
	"github.com/google/uuid"
	_ "github.com/joho/godotenv/autoload"
)

type creatorInput struct {
	Username *string
	Email    *string
}

func (input creatorInput) values() models.CreatorCreate {
	var values models.CreatorCreate

	if input.Username != nil {
		values.Username = runtime.Set(*input.Username)
	}
	if input.Email != nil {
		values.Email = runtime.Set(*input.Email)
	}
	return values
}

type postInput struct {
	Title     *string
	Content   *string
	Published *bool
	AuthorID  *uuid.UUID
}

func (input postInput) values() models.PostCreate {
	var values models.PostCreate

	if input.Title != nil {
		values.Title = runtime.Set(*input.Title)
	}
	if input.Content != nil {
		values.Content = runtime.Set(*input.Content)
	}
	if input.Published != nil {
		values.Published = runtime.Set(*input.Published)
	}
	if input.AuthorID != nil {
		values.AuthorId = runtime.Set(*input.AuthorID)
	}
	return values
}

type endpoints[M, I any] struct {
	list   func(context.Context) ([]*M, error)
	get    func(context.Context, uuid.UUID) (*M, error)
	insert func(context.Context, I) (*M, error)
	patch  func(context.Context, uuid.UUID, I) (*M, error)
	remove func(context.Context, uuid.UUID) error
}

func register[M, I any](mux *http.ServeMux, path string, handlers endpoints[M, I]) {
	mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		rows, err := handlers.list(r.Context())
		respond(w, http.StatusOK, rows, err)
	})
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
		var input I

		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		item, err := handlers.insert(r.Context(), input)
		respond(w, http.StatusCreated, item, err)
	})
	mux.HandleFunc("GET "+path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid UUID", http.StatusBadRequest)
			return
		}
		item, err := handlers.get(r.Context(), id)
		respondItem(w, r, item, err)
	})
	mux.HandleFunc("PUT "+path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input I

		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil || json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		item, err := handlers.patch(r.Context(), id, input)
		respondItem(w, r, item, err)
	})
	mux.HandleFunc("DELETE "+path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "invalid UUID", http.StatusBadRequest)
			return
		}
		if err := handlers.remove(r.Context(), id); err != nil {
			respond(w, http.StatusNoContent, nil, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func respondItem[M any](w http.ResponseWriter, r *http.Request, item *M, err error) {
	if (item == nil && err == nil) || errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	respond(w, http.StatusOK, item, err)
}

func respond(w http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		log.Printf("database operation: %v", err)
		http.Error(w, "database operation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func main() {
	client, err := models.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	creators, err := client.CreatorService()
	if err != nil {
		log.Fatal(err)
	}
	posts, err := client.PostService()
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	register(mux, "/creators", endpoints[models.Creator, creatorInput]{
		list: func(ctx context.Context) ([]*models.Creator, error) {
			return creators.Find(ctx, models.CreatorQuery{})
		},
		get: func(ctx context.Context, id uuid.UUID) (*models.Creator, error) {
			return creators.Fetch(ctx, models.CreatorById(id), models.CreatorInclude{Posts: true})
		},
		insert: func(ctx context.Context, input creatorInput) (*models.Creator, error) {
			return creators.Insert(ctx, input.values())
		},
		patch: func(ctx context.Context, id uuid.UUID, input creatorInput) (*models.Creator, error) {
			return creators.Patch(ctx, models.CreatorById(id), models.CreatorUpdate(input.values()))
		},
		remove: func(ctx context.Context, id uuid.UUID) error {
			return creators.DeleteBy(ctx, models.CreatorById(id))
		},
	})
	register(mux, "/posts", endpoints[models.Post, postInput]{
		list: func(ctx context.Context) ([]*models.Post, error) {
			return posts.Find(ctx, models.PostQuery{})
		},
		get: func(ctx context.Context, id uuid.UUID) (*models.Post, error) {
			return posts.Fetch(ctx, models.PostById(id), models.PostInclude{Author: true})
		},
		insert: func(ctx context.Context, input postInput) (*models.Post, error) {
			return posts.Insert(ctx, input.values())
		},
		patch: func(ctx context.Context, id uuid.UUID, input postInput) (*models.Post, error) {
			return posts.Patch(ctx, models.PostById(id), models.PostUpdate(input.values()))
		},
		remove: func(ctx context.Context, id uuid.UUID) error {
			return posts.DeleteBy(ctx, models.PostById(id))
		},
	})
	log.Fatal(http.ListenAndServe(":8080", mux))
}
