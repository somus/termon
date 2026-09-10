package website

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"golang.org/x/time/rate"

	"termon.sh/internal/telemetry"
)

func eventHandler(recorder telemetry.Recorder) http.Handler {
	limiter := rate.NewLimiter(20, 40)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !limiter.Allow() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var input struct {
			VisitID string `json:"visit_id"`
			Name    string `json:"event"`
			Outcome string `json:"outcome"`
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		event, err := telemetry.NewWebsiteEvent(input.VisitID, input.Name, input.Outcome)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		recorder.Record(event)
		w.WriteHeader(http.StatusNoContent)
	})
}
