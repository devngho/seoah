package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"gomoku-engine/internal/game"
	"gomoku-engine/internal/renju"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, game.ErrorDTO{Error: msg})
}

// withCORS: SolidJS 개발 서버(다른 포트)에서의 fetch를 허용.
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// decodeBody: 요청 본문을 JSON으로 읽는다. 본문이 비어 있으면(=전부 기본값)
// 에러로 보지 않고 그냥 넘어간다.
func decodeBody(r *http.Request, dst interface{}) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	err := json.NewDecoder(r.Body).Decode(dst)
	if err == io.EOF {
		return nil
	}
	return err
}

func handleCreateGame(m *game.GameManager) http.HandlerFunc {
	type request struct {
		Renju    *game.RenjuConfigDTO `json:"renju"`
		Depth    int                  `json:"depth"`
		MaxNodes int64                `json:"maxNodes"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := decodeBody(r, &body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "잘못된 요청 본문입니다")
			return
		}

		var cfg *renju.RenjuConfig
		if body.Renju != nil {
			c := body.Renju.ToConfig()
			cfg = &c
		}

		state, err := m.CreateGame(cfg, body.Depth, body.MaxNodes)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, state)
	}
}

func handleGetState(m *game.GameManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := m.GetState(r.PathValue("id"))
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, state)
	}
}

func handlePlaceMove(m *game.GameManager) http.HandlerFunc {
	type request struct {
		Row int `json:"row"`
		Col int `json:"col"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := decodeBody(r, &body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "잘못된 요청 본문입니다")
			return
		}

		state, err := m.PlaceMove(r.PathValue("id"), body.Row, body.Col)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, state)
	}
}

func handleBestMove(m *game.GameManager) http.HandlerFunc {
	type request struct {
		Place bool `json:"place"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := decodeBody(r, &body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "잘못된 요청 본문입니다")
			return
		}

		res, err := m.BestMove(r.PathValue("id"), body.Place)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func main() {
	port := 8090
	manager := game.NewGameManager()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/games", handleCreateGame(manager))
	mux.HandleFunc("GET /api/games/{id}", handleGetState(manager))
	mux.HandleFunc("POST /api/games/{id}/move", handlePlaceMove(manager))
	mux.HandleFunc("POST /api/games/{id}/best-move", handleBestMove(manager))

	addr := fmt.Sprintf(":%d", port)
	fmt.Println("Server is running on port " + addr + "...")
	if err := http.ListenAndServe(addr, withCORS(mux.ServeHTTP)); err != nil {
		log.Fatalf("Failed to run server: %v\n", err)
	}
}
