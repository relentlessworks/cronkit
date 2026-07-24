package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// wantsJSON checks if the client wants JSON response.
func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return true
	}
	return r.URL.Query().Get("format") == "json"
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeText writes a plain text response.
func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

// writeError writes an error response in the appropriate format.
func writeError(w http.ResponseWriter, r *http.Request, status int, msg, hint string) {
	if wantsJSON(r) {
		writeJSON(w, status, map[string]string{
			"error": msg,
			"hint":  hint,
		})
		return
	}
	writeText(w, status, "error: "+msg+" | hint: "+hint)
}

// writeRecord writes a single record in the appropriate format.
func writeRecord(w http.ResponseWriter, r *http.Request, status int, record string, data interface{}) {
	if wantsJSON(r) {
		writeJSON(w, status, data)
		return
	}
	writeText(w, status, record)
}

// writeRecords writes multiple records in the appropriate format.
func writeRecords(w http.ResponseWriter, r *http.Request, status int, records []string, data interface{}) {
	if wantsJSON(r) {
		writeJSON(w, status, data)
		return
	}
	body := strings.Join(records, "\n")
	if body == "" {
		body = "(no records)"
	}
	writeText(w, status, body)
}
