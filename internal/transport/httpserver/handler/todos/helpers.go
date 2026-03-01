package todos

import (
	"net/http"
	"time"

	commonhandler "family-app-go/internal/transport/httpserver/handler/common"
)

func writeError(w http.ResponseWriter, status int, code, message string) {
	commonhandler.WriteError(w, status, code, message)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	commonhandler.WriteJSON(w, status, payload)
}

func decodeJSON(r *http.Request, dst interface{}) error {
	return commonhandler.DecodeJSON(r, dst)
}

func parseDateParam(value string) (*time.Time, error) {
	return commonhandler.ParseDateParam(value)
}

func parseCSV(value string) []string {
	return commonhandler.ParseCSV(value)
}

func parseIntParam(value string, fallback int) (int, error) {
	return commonhandler.ParseIntParam(value, fallback)
}
