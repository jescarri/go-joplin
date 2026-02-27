package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jescarri/go-joplin/internal/config"
)

// Client is an HTTP client for the Joplin Server API.
type Client struct {
	baseURL    string
	username   string
	password   string
	sessionID  string
	httpClient *http.Client
}

// NewClient creates a new Joplin Server API client.
func NewClient(cfg *config.Config) (*Client, error) {
	baseURL := strings.TrimRight(cfg.ServerURL, "/")

	return &Client{
		baseURL:  baseURL,
		username: cfg.Username,
		password: cfg.Password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// doRequest performs an authenticated HTTP request.
func (c *Client) doRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	if c.sessionID != "" {
		req.Header.Set("X-API-AUTH", c.sessionID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// doRequestOctet performs an authenticated request with octet-stream content type.
func (c *Client) doRequestOctet(method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	if c.sessionID != "" {
		req.Header.Set("X-API-AUTH", c.sessionID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}

	return c.httpClient.Do(req)
}

// Get performs an authenticated GET and returns the response body (SyncBackend interface).
func (c *Client) Get(path string) ([]byte, error) {
	return c.get(path)
}

// Put performs an authenticated PUT with binary content (SyncBackend interface).
func (c *Client) Put(path string, content []byte) error {
	return c.put(path, content)
}

// Delete performs an authenticated DELETE (SyncBackend interface).
func (c *Client) Delete(path string) error {
	return c.delete(path)
}

// SyncTarget returns the Joplin sync target id (9 = Joplin Server).
func (c *Client) SyncTarget() int {
	return 9
}

// get performs an authenticated GET request and returns the response body.
func (c *Client) get(path string) ([]byte, error) {
	resp, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, string(data))
	}
	return data, nil
}

// put performs an authenticated PUT request with binary content.
func (c *Client) put(path string, content []byte) error {
	resp, err := c.doRequestOctet("PUT", path, bytes.NewReader(content))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT %s: status %d: %s", path, resp.StatusCode, string(data))
	}
	return nil
}

// delete performs an authenticated DELETE request.
func (c *Client) delete(path string) error {
	resp, err := c.doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: status %d: %s", path, resp.StatusCode, string(data))
	}
	return nil
}

// post performs an authenticated POST request with JSON body.
func (c *Client) post(path string, payload interface{}) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}

	resp, err := c.doRequest("POST", path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(data))
	}
	return data, nil
}
