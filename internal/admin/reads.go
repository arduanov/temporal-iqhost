package admin

import (
	"net/http"

	"github.com/google/uuid"

	"temporal-iqhost/internal/postgres/dbgen"
)

func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	if err := server.Store.Pool.Ping(request.Context()); err != nil {
		writeError(writer, 503, "База данных недоступна")
		return
	}
	writeJSON(writer, 200, map[string]string{"status": "ok"})
}

func (server *Server) machines(writer http.ResponseWriter, request *http.Request) {
	limit, offset, err := pagination(request)
	if err != nil {
		writeError(writer, 400, "limit: 1–200, offset: от 0")
		return
	}
	query := request.URL.Query()
	recovery := query.Get("recovery_required")
	if recovery != "" && recovery != "true" && recovery != "false" {
		writeError(writer, 400, "Некорректный фильтр восстановления")
		return
	}
	rows, err := server.Store.Q.AdminVMs(request.Context(), dbgen.AdminVMsParams{
		Search:       query.Get("search"),
		State:        query.Get("state"),
		RecoveryOnly: recovery == "true",
		PageLimit:    limit,
		PageOffset:   offset,
	})
	if err != nil {
		requestError(writer, request, err)
		return
	}
	result := page[Machine]{Items: []Machine{}, Limit: limit, Offset: offset}
	for _, row := range rows {
		result.Items = append(result.Items, machineDTO(row))
	}
	writeJSON(writer, 200, result)
}

func (server *Server) machine(writer http.ResponseWriter, request *http.Request) {
	id := request.URL.Query().Get("vm_id")
	if id == "" {
		writeError(writer, 400, "Укажите vm_id")
		return
	}
	rows, err := server.Store.Q.AdminVMs(request.Context(), dbgen.AdminVMsParams{ExactID: id, PageLimit: 1})
	if err != nil {
		requestError(writer, request, err)
		return
	}
	if len(rows) == 0 {
		writeError(writer, 404, "VM не найдена")
		return
	}
	writeJSON(writer, 200, machineDTO(rows[0]))
}

func (server *Server) operations(writer http.ResponseWriter, request *http.Request) {
	limit, offset, err := pagination(request)
	if err != nil {
		writeError(writer, 400, "limit: 1–200, offset: от 0")
		return
	}
	query := request.URL.Query()
	rows, err := server.Store.Q.AdminOperations(request.Context(), dbgen.AdminOperationsParams{
		VmID:       query.Get("vm_id"),
		UserID:     query.Get("user_id"),
		Status:     query.Get("status"),
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		requestError(writer, request, err)
		return
	}
	result := page[Operation]{Items: []Operation{}, Limit: limit, Offset: offset}
	for _, row := range rows {
		result.Items = append(result.Items, operationDTO(row))
	}
	writeJSON(writer, 200, result)
}

func (server *Server) operation(writer http.ResponseWriter, request *http.Request) {
	id := request.PathValue("id")
	if _, err := uuid.Parse(id); err != nil {
		writeError(writer, 400, "Некорректный UUID")
		return
	}
	snapshot, err := server.Store.AdminOperation(request.Context(), id)
	if err != nil {
		requestError(writer, request, err)
		return
	}
	writeJSON(writer, 200, operationDetailDTO(snapshot))
}

func (server *Server) resources(writer http.ResponseWriter, request *http.Request) {
	snapshot, err := server.Store.AdminResources(request.Context())
	if err != nil {
		requestError(writer, request, err)
		return
	}
	result, err := resourcesDTO(snapshot)
	if err != nil {
		requestError(writer, request, err)
		return
	}
	writeJSON(writer, 200, result)
}
