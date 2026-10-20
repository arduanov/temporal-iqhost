package simulator

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"temporal-iqhost/internal/provider"
)

func TestHTTPSharedStateAndClientRestart(t *testing.T) {
	server := httptest.NewServer(NewHandler(NewWithDelays(Delays{})))
	defer server.Close()
	first := NewClient(server.URL)
	ctx := context.Background()
	require.NoError(t, first.Fence(ctx, request(provider.StepFence)))
	created, err := first.Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	second := NewClient(server.URL)
	replay, err := second.Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	require.Equal(t, created, replay)
	_, err = second.AttachNetwork(ctx, request(provider.StepAttachNetwork))
	require.NoError(t, err)
	_, err = second.Start(ctx, request(provider.StepStart))
	require.NoError(t, err)
	state, err := first.WaitReady(ctx, request(provider.StepWaitReady))
	require.NoError(t, err)
	require.True(t, state.PoweredOn)
	state, err = first.Inspect(ctx, request(provider.StepInspect))
	require.NoError(t, err)
	require.True(t, state.PoweredOn)
	_, err = second.Stop(ctx, request(provider.StepStop))
	require.NoError(t, err)
	_, err = second.DetachNetwork(ctx, request(provider.StepDetachNetwork))
	require.NoError(t, err)
	state, err = second.Delete(ctx, request(provider.StepDeleteInstance))
	require.NoError(t, err)
	require.Equal(t, provider.State{}, state)
}

type brokenProvider struct{ provider.Provider }

func (brokenProvider) Create(context.Context, provider.Request) (provider.State, error) {
	return provider.State{}, errors.New("temporary failure")
}

func TestHTTPErrorClassification(t *testing.T) {
	server := httptest.NewServer(NewHandler(NewWithDelays(Delays{})))
	defer server.Close()
	for _, body := range []string{"{", "null", "{}", `{"Step":"unknown"}`, `{} {}`} {
		response, err := http.Post(server.URL+"/commands", "application/json", strings.NewReader(body))
		require.NoError(t, err)
		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		_ = response.Body.Close()
	}
	response, err := http.Get(server.URL + "/healthz")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	_ = response.Body.Close()
	client := NewClient(server.URL)
	var permanent *provider.PermanentError
	_, err = client.Create(context.Background(), request(provider.StepCreate))
	require.ErrorAs(t, err, &permanent)
	failing := httptest.NewServer(NewHandler(brokenProvider{}))
	defer failing.Close()
	_, err = NewClient(failing.URL).Create(context.Background(), request(provider.StepCreate))
	require.Error(t, err)
	require.False(t, errors.As(err, &permanent))
	require.Contains(t, err.Error(), "500")
	server.Close()
	_, err = client.Create(context.Background(), request(provider.StepCreate))
	require.Error(t, err)
	require.False(t, errors.As(err, &permanent))
}

func TestHTTPCancellation(t *testing.T) {
	simulator := NewWithDelays(Delays{Create: time.Second})
	server := httptest.NewServer(NewHandler(simulator))
	defer server.Close()
	client := NewClient(server.URL)
	ctx := context.Background()
	require.NoError(t, client.Fence(ctx, request(provider.StepFence)))
	waiting, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	_, err := client.Create(waiting, request(provider.StepCreate))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	state, err := client.Inspect(ctx, request(provider.StepInspect))
	require.NoError(t, err)
	require.False(t, state.Exists)
}

func TestHTTPLostResponse(t *testing.T) {
	simulator := NewWithDelays(Delays{})
	handler := NewHandler(simulator)
	var lose atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if lose.CompareAndSwap(true, false) {
			// Эффект выполнен, но соединение закрывается до отправки ответа.
			recorded := httptest.NewRecorder()
			handler.ServeHTTP(recorded, request)
			connection, _, err := writer.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		handler.ServeHTTP(writer, request)
	}))
	defer server.Close()
	ctx := context.Background()
	client := NewClient(server.URL)
	require.NoError(t, client.Fence(ctx, request(provider.StepFence)))
	lose.Store(true)
	_, err := client.Create(ctx, request(provider.StepCreate))
	require.Error(t, err)
	replay, err := NewClient(server.URL).Create(ctx, request(provider.StepCreate))
	require.NoError(t, err)
	require.True(t, replay.Exists)
	simulator.mutex.Lock()
	require.Len(t, simulator.results, 2)
	simulator.mutex.Unlock()
}

func TestHTTPPermanentStatus(t *testing.T) {
	server := httptest.NewServer(NewHandler(NewWithDelays(Delays{})))
	defer server.Close()
	response, err := http.Post(server.URL+"/commands", "application/json",
		strings.NewReader(`{"Step":"create","Generation":1,"OperationID":"op","Phase":"forward","Key":"op/forward/create"}`))
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusUnprocessableEntity, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"error"`)
}
