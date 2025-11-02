package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	BaseURL string
	client  = &http.Client{Timeout: 5 * time.Second}
)

func Setup(baseURL string) {
	BaseURL = baseURL
}

func Get[T any](endpoint string) (*T, error) {
	url := BaseURL + endpoint
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET failed [%d]: %s", resp.StatusCode, body)
	}

	var data T
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &data, nil
}

func Post[T any](endpoint string, body any) (*T, error) {
	return send[T]("POST", endpoint, body)
}

func Put[T any](endpoint string, body any) (*T, error) {
	return send[T]("PUT", endpoint, body)
}

func Delete(endpoint string) error {
	req, err := http.NewRequest("DELETE", BaseURL+endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE failed [%d]: %s", resp.StatusCode, b)
	}
	return nil
}

// internal send helper for POST/PUT
func send[T any](method, endpoint string, body any) (*T, error) {
	url := BaseURL + endpoint

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s failed [%d]: %s", method, resp.StatusCode, b)
	}
	var data T
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &data, nil
}
