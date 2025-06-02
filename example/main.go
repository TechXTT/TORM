package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	_ "github.com/lib/pq"

	"github.com/TechXTT/TORM/example/models"
	_ "github.com/joho/godotenv/autoload" // Automatically load .env file
)

func main() {
	// Bootstrap
	if os.Getenv("DATABASE_URL") == "" {
		log.Fatal("DATABASE_URL must be set")
	}
	client := models.NewClient()

	creatorSvc, err := client.CreatorService()
	if err != nil {
		log.Fatalf("CreatorService init: %v", err)
	}
	postSvc, err := client.PostService()
	if err != nil {
		log.Fatalf("PostService init: %v", err)
	}

	// --- Creator endpoints ---
	http.HandleFunc("/creators", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			list, err := creatorSvc.FindMany(r.Context(), nil, nil, 0, 0)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			respondJSON(w, list)
		case http.MethodPost:
			var input map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			rec, err := creatorSvc.Create(r.Context(), input)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			respondJSON(w, rec)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/creators/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/creators/")
		switch r.Method {
		case http.MethodGet:
			rec, err := creatorSvc.FindUnique(r.Context(), map[string]interface{}{"id": id})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if rec == nil {
				http.NotFound(w, r)
				return
			}
			respondJSON(w, rec)
		case http.MethodPut:
			var input map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			rec, err := creatorSvc.Update(r.Context(), map[string]interface{}{"id": id}, input)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			respondJSON(w, rec)
		case http.MethodDelete:
			if err := creatorSvc.Delete(r.Context(), map[string]interface{}{"id": id}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// --- Post endpoints ---
	http.HandleFunc("/posts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			list, err := postSvc.FindMany(r.Context(), nil, nil, 0, 0)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			respondJSON(w, list)
		case http.MethodPost:
			var input map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			rec, err := postSvc.Create(r.Context(), input)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			respondJSON(w, rec)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/posts/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/posts/")
		switch r.Method {
		case http.MethodGet:
			rec, err := postSvc.FindUnique(r.Context(), map[string]interface{}{"id": id})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if rec == nil {
				http.NotFound(w, r)
				return
			}
			respondJSON(w, rec)
		case http.MethodPut:
			var input map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			rec, err := postSvc.Update(r.Context(), map[string]interface{}{"id": id}, input)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			respondJSON(w, rec)
		case http.MethodDelete:
			if err := postSvc.Delete(r.Context(), map[string]interface{}{"id": id}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	log.Println("🦊 Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// respondJSON is a helper to write JSON responses.
func respondJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
