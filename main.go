package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/stfmarkov/boot-dev-chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
}

type respValue struct {
	CleanedBody string `json:"cleaned_body"`
	Error       string `json:"error"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func respondWithError(w http.ResponseWriter, status int, errString string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := respValue{
		Error: errString,
	}

	dat, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Write(dat)
}

func main() {
	godotenv.Load()

	dbURL := os.Getenv("DB_URL")

	db, err := sql.Open("postgres", dbURL)

	if err != nil {
		fmt.Println("Error connecting to database:", err)
		return
	}

	dbQueries := database.New(db)

	mux := http.NewServeMux()
	var srv http.Server
	srv.Addr = ":8088"
	srv.Handler = mux

	config := apiConfig{}
	config.dbQueries = dbQueries

	mux.Handle("/app/", config.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))

	healthHandler := func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))

	}

	mux.HandleFunc("GET /api/healthz", healthHandler)

	checkMetrics := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(
			"<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>",
			config.fileserverHits.Load())))
	}

	mux.HandleFunc("GET /admin/metrics", checkMetrics)

	resetMetrics := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		config.fileserverHits.Store(0)
	}

	mux.HandleFunc("POST /admin/reset", resetMetrics)

	churpValidator := func(w http.ResponseWriter, r *http.Request) {
		type params struct {
			Body string `json:"body"`
		}

		respondWithBody := func(w http.ResponseWriter, body string) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			resp := respValue{
				CleanedBody: body,
			}

			dat, err := json.Marshal(resp)
			if err != nil {
				w.WriteHeader(500)
				return
			}
			w.Write(dat)
		}

		replaceBadWords := func(str string) string {
			badWords := []string{"kerfuffle", "sharbert", "fornax"}

			words := strings.Split(str, " ")

			for i, word := range words {
				for _, badWord := range badWords {
					if strings.EqualFold(word, badWord) {
						words[i] = "****"
					}
				}
			}

			str = strings.Join(words, " ")

			return str
		}

		decoder := json.NewDecoder(r.Body)

		parameters := params{}

		err := decoder.Decode(&parameters)

		if err != nil {
			respondWithError(w, 500, "Invalid JSON")
			return
		}

		if len(parameters.Body) > 140 {
			respondWithError(w, 400, "Chirp is too long")
			return
		}

		respondWithBody(w, replaceBadWords(parameters.Body))
	}

	mux.HandleFunc("POST /api/validate_chirp", churpValidator)

	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		type params struct {
			Email string `json:"email"`
		}

		decoder := json.NewDecoder(r.Body)

		parameters := params{}

		err := decoder.Decode(&parameters)

		if err != nil {
			respondWithError(w, 500, "Invalid JSON")
			return
		}

		resp, err := config.dbQueries.CreateUser(r.Context(), parameters.Email)

		if err != nil {

			fmt.Println("Error creating user:", err)

			respondWithError(w, 500, "Error creating user")
			return
		}

		dat, err := json.Marshal(resp)
		if err != nil {
			respondWithError(w, 500, "Error marshalling response")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write(dat)
	})

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		fmt.Println("Error starting server:", err)
	}

}
