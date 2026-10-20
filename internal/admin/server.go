// Package admin обслуживает локальный интерфейс без подключения к Temporal.
package admin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"temporal-iqhost/internal/postgres"
	"temporal-iqhost/internal/vm"
)

type Server struct {
	Store     *postgres.Store
	StaticDir string
	// DevOrigin разрешает только явно заданный адрес Vite при локальной разработке.
	DevOrigin string
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("GET /api/v1/vms", server.machines)
	mux.HandleFunc("GET /api/v1/vm", server.machine)
	mux.HandleFunc("GET /api/v1/operations", server.operations)
	mux.HandleFunc("GET /api/v1/operations/{id}", server.operation)
	mux.HandleFunc("GET /api/v1/resources", server.resources)
	mux.HandleFunc("POST /api/v1/commands", server.command)
	mux.HandleFunc("/api/", func(writer http.ResponseWriter, request *http.Request) {
		writeError(writer, 404, "Маршрут не найден")
	})
	mux.HandleFunc("/", server.static)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(request.URL.Path, "/api/") {
			writer.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(writer, request)
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func requestError(writer http.ResponseWriter, request *http.Request, err error) {
	var rejected *vm.RejectedError
	var databaseError *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(writer, 404, "Объект не найден")
	case errors.As(err, &rejected):
		writeError(writer, 409, rejected.Message)
	case errors.As(err, &databaseError) && databaseError.Code == "23505":
		writeError(writer, 409, "Конфликт ключа команды или активной операции")
	default:
		// Ошибка драйвера может содержать SQL и параметры: в HTTP и логах оставляем только тип.
		slog.Error("admin request failed", "method", request.Method, "path", request.URL.Path, "error_type", errorType(err))
		writeError(writer, 500, "Не удалось выполнить запрос")
	}
}

func errorType(err error) string {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		return "database/" + databaseError.Code
	}
	return "internal"
}

func pagination(request *http.Request) (int32, int32, error) {
	limit, offset := int64(50), int64(0)
	var err error
	if value := request.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.ParseInt(value, 10, 32)
		if err != nil {
			return 0, 0, err
		}
	}
	if value := request.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.ParseInt(value, 10, 32)
		if err != nil {
			return 0, 0, err
		}
	}
	if limit < 1 || limit > 200 || offset < 0 {
		return 0, 0, errors.New("invalid pagination")
	}
	return int32(limit), int32(offset), nil
}

type page[T any] struct {
	Items  []T   `json:"items"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

func (server *Server) static(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writeError(writer, 405, "Метод не поддерживается")
		return
	}
	// Fallback разрешён только известным страницам. API и отсутствующие assets не получают HTML.
	path := request.URL.Path
	if path == "/" || path == "/vms" || path == "/vm" || path == "/operations" || path == "/resources" || strings.HasPrefix(path, "/operations/") {
		writer.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(writer, request, filepath.Join(server.StaticDir, "index.html"))
		return
	}
	if strings.HasPrefix(path, "/assets/") {
		if _, err := os.Stat(filepath.Join(server.StaticDir, filepath.Clean(path))); err != nil {
			http.NotFound(writer, request)
			return
		}
		http.FileServer(http.Dir(server.StaticDir)).ServeHTTP(writer, request)
		return
	}
	http.NotFound(writer, request)
}
