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
	BaseURL   string
	client    = &http.Client{Timeout: 5 * time.Second}
	authToken string
)

// Setup configures the base URL and bearer token once
func Setup(baseURL string, token string) {
	BaseURL = baseURL
	authToken = token
}

// ----------- Generic request helpers -----------

func Get[T any](endpoint string) (*T, error) {
	req, err := newRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	return doRequest[T](req)
}

func Post[T any](endpoint string, body any) (*T, error) {
	req, err := newRequest("POST", endpoint, body)
	if err != nil {
		return nil, err
	}
	return doRequest[T](req)
}

func Put[T any](endpoint string, body any) (*T, error) {
	req, err := newRequest("PUT", endpoint, body)
	if err != nil {
		return nil, err
	}
	return doRequest[T](req)
}

func Delete(endpoint string) error {
	req, err := newRequest("DELETE", endpoint, nil)
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

// ----------- Internal helpers -----------

func newRequest(method, endpoint string, body any) (*http.Request, error) {
	url := BaseURL + endpoint

	var buf io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		buf = bytes.NewBuffer(payload)
	}

	req, err := http.NewRequest(method, url, buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}

	return req, nil
}

func doRequest[T any](req *http.Request) (*T, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, req.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s failed [%d]: %s", req.Method, resp.StatusCode, body)
	}

	var data T
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &data, nil
}
