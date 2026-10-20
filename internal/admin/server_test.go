package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/postgres"
	"temporal-iqhost/internal/testdb"
	"temporal-iqhost/internal/vm"
)

func TestRequestValidationAndOrigin(t *testing.T) {
	handler := (&Server{}).Handler()
	cases := []struct {
		name, body, origin, media, fetch string
		status                           int
	}{
		{"bad JSON", "{", "", "application/json", "", 400},
		{"unknown fields", `{"extra":true}`, "", "application/json", "", 400},
		{"missing UUID", `{"command":{"kind":"create"}}`, "", "application/json", "", 400},
		{"cross origin", `{}`, "http://evil.test", "application/json", "", 403},
		{"null origin", `{}`, "null", "application/json", "", 403},
		{"cross site", `{}`, "", "application/json", "cross-site", 403},
		{"form", `{}`, "", "application/x-www-form-urlencoded", "", 400},
		{"two objects", `{} {}`, "", "application/json", "", 400},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "http://localhost/api/v1/commands", bytes.NewBufferString(item.body))
			request.Header.Set("Origin", item.origin)
			request.Header.Set("Content-Type", item.media)
			request.Header.Set("Sec-Fetch-Site", item.fetch)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, item.status, response.Code)
		})
	}
	for _, target := range []string{"/api/v1/vms?limit=201", "/api/v1/operations?offset=-1", "/api/v1/operations/no-uuid", "/api/v1/vm"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", target, nil))
		require.Equal(t, 400, response.Code)
	}
}

func TestSPAFallback(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "index.html"), []byte("<html>admin</html>"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "assets"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "assets", "app.js"), []byte("const app = true;"), 0o600))
	handler := (&Server{StaticDir: directory}).Handler()
	for _, path := range []string{"/vms", "/vm?vm_id=a/b", "/operations/123", "/resources", "/assets/app.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 200, response.Code, path)
	}
	for _, path := range []string{"/api/missing", "/assets/missing.js", "/other"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 404, response.Code, path)
		require.NotContains(t, response.Body.String(), "<html>admin")
	}
}

func TestCommandsAndReadAPI(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.New(t, "TEST_DATABASE_URL")
	require.NoError(t, postgres.Migrate(ctx, dsn, false))
	store, err := postgres.Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(store.Pool.Close)
	require.NoError(t, store.Seed(ctx))
	handler := (&Server{Store: store}).Handler()
	submit := func(body commandRequest) *httptest.ResponseRecorder {
		raw, marshalErr := json.Marshal(body)
		require.NoError(t, marshalErr)
		request := httptest.NewRequest("POST", "/api/v1/commands", bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	body := commandRequest{RequestID: uuid.NewString(), Command: vm.Command{VMID: "admin/тест", Kind: vm.CreateVM, CPU: 4, MemoryMiB: 8192, DiskGiB: 20, UserID: "operator"}}
	require.Equal(t, 202, submit(body).Code)
	require.Equal(t, 202, submit(body).Code)
	var operations, deliveries int
	require.NoError(t, store.Pool.QueryRow(ctx, "SELECT count(*) FROM vm_operations").Scan(&operations))
	require.NoError(t, store.Pool.QueryRow(ctx, "SELECT count(*) FROM outbox").Scan(&deliveries))
	require.Equal(t, 1, operations)
	require.Equal(t, 1, deliveries)
	changed := body
	changed.Command.CPU++
	require.Equal(t, 409, submit(changed).Code)
	changed = body
	changed.Command.VMID = "another-missing-vm"
	changed.Command.Kind = vm.StartVM
	require.Equal(t, 409, submit(changed).Code)
	changed = body
	changed.RequestID = uuid.NewString()
	require.Equal(t, 409, submit(changed).Code)
	for _, path := range []string{"/healthz", "/api/v1/vms", "/api/v1/vm?vm_id=admin%2F%D1%82%D0%B5%D1%81%D1%82", "/api/v1/operations?user_id=operator", "/api/v1/operations/" + body.RequestID, "/api/v1/resources"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 200, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), `"request":`)
		require.NotContains(t, response.Body.String(), `"lease_token":`)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/operations/"+body.RequestID, nil))
	var detail OperationDetail
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &detail))
	require.Nil(t, detail.Delivery.DeliveredAt)
	require.Empty(t, detail.Steps)
	missing := body
	missing.RequestID = uuid.NewString()
	missing.Command = vm.Command{VMID: "missing", Kind: vm.StartVM}
	require.Equal(t, 404, submit(missing).Code)
	checkConcurrentAndRecovery(t, store, submit, body)
}

func checkConcurrentAndRecovery(t *testing.T, store *postgres.Store, submit func(commandRequest) *httptest.ResponseRecorder, body commandRequest) {
	t.Helper()
	ctx := context.Background()
	// Конкурентные HTTP-команды проходят через ту же блокировку VM, что и CLI.
	concurrent := body
	concurrent.Command.VMID = "concurrent"
	var group sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		group.Go(func() { request := concurrent; request.RequestID = uuid.NewString(); codes <- submit(request).Code })
	}
	group.Wait()
	close(codes)
	successes := 0
	for code := range codes {
		if code == 202 {
			successes++
		} else {
			require.Equal(t, 409, code)
		}
	}
	require.Equal(t, 1, successes)
	_, err := store.Pool.Exec(ctx, "UPDATE vm_operations SET status='rollback-blocked' WHERE id=$1", body.RequestID)
	require.NoError(t, err)
	_, err = store.Pool.Exec(ctx, "UPDATE vms SET recovery_required=true WHERE id=$1", body.Command.VMID)
	require.NoError(t, err)
	blocked := body
	blocked.RequestID = uuid.NewString()
	blocked.Command.Kind = vm.StartVM
	require.Equal(t, 409, submit(blocked).Code)
	blocked.RequestID = uuid.NewString()
	blocked.Command.Kind = vm.Cleanup
	require.Equal(t, 202, submit(blocked).Code)
}

func TestSameOrigin(t *testing.T) {
	server := &Server{DevOrigin: "http://127.0.0.1:5173"}
	for origin, expected := range map[string]bool{"http://localhost:8082": true, "https://localhost:8082": false, "http://localhost:8082.evil": false, "http://127.0.0.1:5173": true} {
		request := httptest.NewRequest(http.MethodPost, "http://localhost:8082/api/v1/commands", nil)
		request.Header.Set("Origin", origin)
		require.Equal(t, expected, server.sameOrigin(request), origin)
	}
}
