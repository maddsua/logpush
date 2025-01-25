package rest

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

type RPCHandler struct {
	RPCProcedures
	Token        string
	AuthAttempts AuthAttempts
	mux          *http.ServeMux
}

func (this *RPCHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {

	if locked, expires := this.AuthAttempts.Locked(req.RemoteAddr); locked {
		writer.Header().Add("Retry-After", expires.Format(time.RFC1123))
		writeJsonError(writer, errors.New("too many failed auth requests"), http.StatusTooManyRequests)
		return
	}

	authType, token, _ := strings.Cut(req.Header.Get("Authorization"), " ")
	if strings.ToLower(strings.TrimSpace(authType)) != "bearer" {
		writeJsonError(writer, errors.New("rpc token required"), http.StatusUnauthorized)
		return
	} else if strings.ToLower(strings.TrimSpace(token)) != this.Token {
		this.AuthAttempts.Increment(req.RemoteAddr)
		writeJsonError(writer, errors.New("invalid rpc token"), http.StatusForbidden)
		return
	}

	if this.mux == nil {
		this.initMux()
	}

	this.mux.ServeHTTP(writer, req)
}

func (this *RPCHandler) initMux() {

	this.mux = http.NewServeMux()

	this.mux.HandleFunc("GET /streams", this.StreamsList)
	this.mux.HandleFunc("PUT /streams", this.StreamsAdd)
	this.mux.HandleFunc("GET /streams/{id}", this.StreamsGet)
	this.mux.HandleFunc("DELETE /streams/{id}", this.StreamsDelete)
	this.mux.HandleFunc("POST /streams/{id}/labels", this.StreamsSetLabels)
}

func writeJsonData(writer http.ResponseWriter, data any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	json.NewEncoder(writer).Encode(map[string]any{
		"data": data,
	})
}

func writeJsonError(writer http.ResponseWriter, err error, status int) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	json.NewEncoder(writer).Encode(map[string]any{
		"data": nil,
		"error": map[string]string{
			"message": err.Error(),
		},
	})
}

func assetJSON(req *http.Request) error {

	if !strings.Contains(req.Header.Get("Content-Type"), "json") {
		return errors.New("expected json")
	}

	return nil
}

type AuthAttempts struct {
	entries            map[string]*authCounter
	mtx                sync.Mutex
	writesAfterCleanup int

	Attempts int
	Period   time.Duration
}

type authCounter struct {
	Value   int
	Expires time.Time
}

func (this *AuthAttempts) Locked(id string) (bool, *time.Time) {

	this.mtx.Lock()
	defer this.mtx.Unlock()

	if this.entries == nil {
		this.entries = map[string]*authCounter{}
	}

	entry := this.entries[id]
	if entry == nil {
		return false, nil
	}

	if entry.Expires.Before(time.Now()) {
		delete(this.entries, id)
		return false, nil
	}

	return entry.Value > this.Attempts, &entry.Expires
}

func (this *AuthAttempts) Increment(id string) {

	this.mtx.Lock()
	defer this.mtx.Unlock()

	if this.entries == nil {
		this.entries = map[string]*authCounter{}
	}

	entry := this.entries[id]
	if entry == nil {
		this.writesAfterCleanup++
		entry = &authCounter{}
	}

	entry.Value++
	entry.Expires = time.Now().Add(this.Period)

	this.entries[id] = entry

	if this.writesAfterCleanup > 100 {

		now := time.Now()
		for key, value := range this.entries {
			if value == nil || value.Expires.Before(now) {
				delete(this.entries, key)
			}
		}

		this.writesAfterCleanup = 0
	}
}
