package render

import (
	"encoding/json"
	"net/http"
)

// JSON пишет JSON ответ.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// JSONOk пишет JSON 200 ответ.
func JSONOk(w http.ResponseWriter, data any) {
	JSON(w, http.StatusOK, data)
}

// JSONError пишет JSON ответ с ошибкой.
func JSONError(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Redirect делает HTTP redirect.
func Redirect(w http.ResponseWriter, r *http.Request, url string) {
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// Error отдаёт простую HTML ошибку.
func Error(w http.ResponseWriter, status int, msg string) {
	http.Error(w, msg, status)
}
