package admin

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"temporal-iqhost/internal/vm"
)

type commandRequest struct {
	RequestID string     `json:"request_id"`
	Command   vm.Command `json:"command"`
}

func (server *Server) command(writer http.ResponseWriter, request *http.Request) {
	if !server.sameOrigin(request) {
		writeError(writer, 403, "Межсайтовый запрос запрещён")
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(writer, 400, "Ожидается application/json")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 16*1024)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var body commandRequest
	if err = decoder.Decode(&body); err != nil {
		writeError(writer, 400, "Некорректный JSON команды")
		return
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		writeError(writer, 400, "Ожидается одна команда")
		return
	}
	if !validCommand(body) {
		writeError(writer, 400, "Проверьте UUID, ID VM, инициатора, команду и положительные размеры ресурсов")
		return
	}
	// Ключ относится ко всей команде, а не к отдельной VM. Без этой проверки
	// смена vm_id на отсутствующую VM вернула бы 404 при попытке её блокировки.
	prior, err := server.Store.Operation(request.Context(), body.RequestID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		requestError(writer, request, err)
		return
	}
	if err == nil && prior.VMID != body.Command.VMID {
		writeError(writer, http.StatusConflict, "UUID уже использован для другой VM")
		return
	}
	operation, err := server.Store.Accept(request.Context(), body.RequestID, body.Command)
	if err != nil {
		requestError(writer, request, err)
		return
	}
	slog.Info("admin command accepted", "operation_id", operation.ID, "vm_id", operation.VMID, "user_id", operation.UserID, "kind", operation.Kind)
	writeJSON(writer, 202, operation)
}

func validCommand(body commandRequest) bool {
	if _, err := uuid.Parse(body.RequestID); err != nil {
		return false
	}
	command := body.Command
	if strings.TrimSpace(command.VMID) == "" || !utf8.ValidString(command.VMID) || strings.ContainsRune(command.VMID, 0) {
		return false
	}
	if len("vm-operation/"+string(command.Kind)+"/"+url.PathEscape(command.VMID)+"/"+body.RequestID) > 1000 {
		return false
	}
	if !utf8.ValidString(command.UserID) || len(strings.TrimSpace(command.UserID)) > 256 || strings.ContainsRune(command.UserID, 0) {
		return false
	}
	switch command.Kind {
	case vm.CreateVM:
		return command.CPU > 0 && command.MemoryMiB > 0 && command.DiskGiB > 0
	case vm.StartVM, vm.StopVM, vm.DeleteVM, vm.Cleanup:
		return true
	default:
		return false
	}
}

func (server *Server) sameOrigin(request *http.Request) bool {
	if request.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if server.DevOrigin != "" && origin == server.DevOrigin {
		return true
	}
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+request.Host
}
