package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func main() {
	mux := http.NewServeMux()
	var srv http.Server
	srv.Addr = ":8088"
	srv.Handler = mux

	config := apiConfig{}

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

		type respValue struct {
			CleanedBody string `json:"cleaned_body"`
			Error       string `json:"error"`
		}

		respondWithError := func(w http.ResponseWriter, status int, errString string) {
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

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		fmt.Println("Error starting server:", err)
	}

}
