package simulator

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"temporal-iqhost/internal/provider"
)

type commandResponse struct {
	State provider.State `json:"state"`
	Error string         `json:"error,omitempty"`
}

func NewHandler(adapter provider.Provider) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /commands", func(writer http.ResponseWriter, request *http.Request) {
		var command provider.Request
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
		if err := decoder.Decode(&command); err != nil {
			writeResponse(writer, http.StatusBadRequest, commandResponse{Error: "invalid JSON"})
			return
		}
		// Не принимаем несколько JSON-документов в одном запросе.
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeResponse(writer, http.StatusBadRequest, commandResponse{Error: "invalid JSON"})
			return
		}
		var state provider.State
		var err error
		switch command.Step {
		case provider.StepFence:
			err = adapter.Fence(request.Context(), command)
		case provider.StepCreate:
			state, err = adapter.Create(request.Context(), command)
		case provider.StepAttachNetwork:
			state, err = adapter.AttachNetwork(request.Context(), command)
		case provider.StepStart:
			state, err = adapter.Start(request.Context(), command)
		case provider.StepWaitReady:
			state, err = adapter.WaitReady(request.Context(), command)
		case provider.StepStop:
			state, err = adapter.Stop(request.Context(), command)
		case provider.StepDetachNetwork:
			state, err = adapter.DetachNetwork(request.Context(), command)
		case provider.StepDeleteInstance:
			state, err = adapter.Delete(request.Context(), command)
		case provider.StepInspect:
			state, err = adapter.Inspect(request.Context(), command)
		default:
			writeResponse(writer, http.StatusBadRequest, commandResponse{Error: "unknown step"})
			return
		}
		if err != nil {
			status := http.StatusInternalServerError
			var permanent *provider.PermanentError
			if errors.As(err, &permanent) {
				status = http.StatusUnprocessableEntity
			}
			writeResponse(writer, status, commandResponse{Error: err.Error()})
			return
		}
		writeResponse(writer, http.StatusOK, commandResponse{State: state})
	})
	return mux
}

func writeResponse(writer http.ResponseWriter, status int, response commandResponse) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(response)
}
