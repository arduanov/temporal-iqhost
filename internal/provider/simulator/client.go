package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"temporal-iqhost/internal/provider"
)

// Client не повторяет запросы: повторами управляет Activity.
type Client struct {
	url  string
	http *http.Client
}

var _ provider.Provider = (*Client)(nil)

func NewClient(url string) *Client {
	return &Client{
		url:  strings.TrimRight(url, "/"),
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

func (client *Client) run(ctx context.Context, command provider.Request, step string) (provider.State, error) {
	if command.Step != step {
		return provider.State{}, provider.Permanent("invalid command identity")
	}
	body, err := json.Marshal(command)
	if err != nil {
		return provider.State{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.url+"/commands", bytes.NewReader(body))
	if err != nil {
		return provider.State{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return provider.State{}, err
	}
	defer func() { _ = response.Body.Close() }()
	var result commandResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return provider.State{}, fmt.Errorf("decode simulator response: %w", err)
	}
	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnprocessableEntity {
		return provider.State{}, provider.Permanent(result.Error)
	}
	if response.StatusCode != http.StatusOK {
		return provider.State{}, fmt.Errorf("simulator HTTP %d: %s", response.StatusCode, result.Error)
	}
	return result.State, nil
}

func (client *Client) Fence(ctx context.Context, request provider.Request) error {
	_, commandErr := client.run(ctx, request, provider.StepFence)
	return commandErr
}

func (client *Client) Create(ctx context.Context, request provider.Request) (provider.State, error) {
	return client.run(ctx, request, provider.StepCreate)
}

func (client *Client) AttachNetwork(ctx context.Context, request provider.Request) (provider.State, error) {
	return client.run(ctx, request, provider.StepAttachNetwork)
}

func (client *Client) Start(ctx context.Context, request provider.Request) (provider.State, error) {
	return client.run(ctx, request, provider.StepStart)
}

func (client *Client) WaitReady(ctx context.Context, request provider.Request) (provider.State, error) {
	return client.run(ctx, request, provider.StepWaitReady)
}

func (client *Client) Stop(ctx context.Context, request provider.Request) (provider.State, error) {
	return client.run(ctx, request, provider.StepStop)
}

func (client *Client) DetachNetwork(ctx context.Context, request provider.Request) (provider.State, error) {
	return client.run(ctx, request, provider.StepDetachNetwork)
}

func (client *Client) Delete(ctx context.Context, request provider.Request) (provider.State, error) {
	return client.run(ctx, request, provider.StepDeleteInstance)
}

func (client *Client) Inspect(ctx context.Context, request provider.Request) (provider.State, error) {
	return client.run(ctx, request, provider.StepInspect)
}
