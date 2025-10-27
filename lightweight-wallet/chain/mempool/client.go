package mempool

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/btcsuite/btcd/wire"
	"golang.org/x/time/rate"
)

// Config holds configuration for the mempool.space client.
type Config struct {
	// BaseURL is the base URL for the mempool.space API.
	// Default: https://mempool.space/api
	BaseURL string

	// RateLimit is the number of requests per second allowed.
	// Default: 10
	RateLimit int

	// Timeout is the HTTP request timeout.
	// Default: 30 seconds
	Timeout time.Duration

	// RetryAttempts is the number of retry attempts for failed requests.
	// Default: 3
	RetryAttempts int

	// RetryDelay is the delay between retry attempts.
	// Default: 1 second
	RetryDelay time.Duration
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		BaseURL:       "https://mempool.space/api",
		RateLimit:     10,
		Timeout:       30 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    time.Second,
	}
}

// Client is an HTTP client for the mempool.space API with rate limiting.
type Client struct {
	cfg *Config

	httpClient  *http.Client
	rateLimiter *rate.Limiter

	mu sync.RWMutex
}

// NewClient creates a new mempool.space API client.
func NewClient(cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Create rate limiter (requests per second)
	limiter := rate.NewLimiter(rate.Limit(cfg.RateLimit), cfg.RateLimit)

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		rateLimiter: limiter,
	}
}

// doRequest performs an HTTP request with rate limiting and retries.
func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	url := c.cfg.BaseURL + path

	var lastErr error
	for attempt := 0; attempt <= c.cfg.RetryAttempts; attempt++ {
		// Wait for rate limiter
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter error: %w", err)
		}

		// Create request
		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		// Perform request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request failed: %w", err)
			if attempt < c.cfg.RetryAttempts {
				time.Sleep(c.cfg.RetryDelay * time.Duration(attempt+1))
				continue
			}
			return nil, lastErr
		}

		defer resp.Body.Close()

		// Read response body
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		// Check status code
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}

		// Handle specific error codes
		switch resp.StatusCode {
		case 429: // Too Many Requests
			lastErr = fmt.Errorf("rate limited by server (429)")
			if attempt < c.cfg.RetryAttempts {
				// Exponential backoff for rate limiting
				time.Sleep(c.cfg.RetryDelay * time.Duration(attempt+1) * 2)
				continue
			}
		case 404:
			return nil, fmt.Errorf("resource not found (404): %s", string(respBody))
		case 500, 502, 503, 504:
			lastErr = fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))
			if attempt < c.cfg.RetryAttempts {
				time.Sleep(c.cfg.RetryDelay * time.Duration(attempt+1))
				continue
			}
		default:
			return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
		}
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", c.cfg.RetryAttempts, lastErr)
}

// GetCurrentHeight retrieves the current blockchain height.
func (c *Client) GetCurrentHeight(ctx context.Context) (uint32, error) {
	respBody, err := c.doRequest(ctx, "GET", "/blocks/tip/height", nil)
	if err != nil {
		return 0, err
	}

	var height uint32
	if err := json.Unmarshal(respBody, &height); err != nil {
		return 0, fmt.Errorf("failed to parse height: %w", err)
	}

	return height, nil
}

// GetBlockHash retrieves the block hash for a given height.
func (c *Client) GetBlockHash(ctx context.Context, height int64) (string, error) {
	path := fmt.Sprintf("/block-height/%d", height)
	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}

	// Response is just the block hash as a string
	return string(respBody), nil
}

// GetBlock retrieves a block by its hash.
func (c *Client) GetBlock(ctx context.Context, blockHash string) (*BlockResponse, error) {
	path := fmt.Sprintf("/block/%s", blockHash)
	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var block BlockResponse
	if err := json.Unmarshal(respBody, &block); err != nil {
		return nil, fmt.Errorf("failed to parse block: %w", err)
	}

	return &block, nil
}

// GetTransaction retrieves a transaction by its ID.
func (c *Client) GetTransaction(ctx context.Context, txid string) (*TransactionResponse, error) {
	path := fmt.Sprintf("/tx/%s", txid)
	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var tx TransactionResponse
	if err := json.Unmarshal(respBody, &tx); err != nil {
		return nil, fmt.Errorf("failed to parse transaction: %w", err)
	}

	return &tx, nil
}

// BroadcastTransaction broadcasts a raw transaction to the network.
func (c *Client) BroadcastTransaction(ctx context.Context, tx *wire.MsgTx) error {
	// Serialize transaction to hex
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return fmt.Errorf("failed to serialize transaction: %w", err)
	}
	txHex := hex.EncodeToString(buf.Bytes())

	// POST transaction as raw hex string
	_, err := c.doRequest(ctx, "POST", "/tx", []byte(txHex))
	if err != nil {
		return fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	return nil
}

// GetFeeEstimates retrieves fee estimates for different confirmation targets.
func (c *Client) GetFeeEstimates(ctx context.Context) (*FeeEstimates, error) {
	respBody, err := c.doRequest(ctx, "GET", "/v1/fees/recommended", nil)
	if err != nil {
		return nil, err
	}

	var fees FeeEstimates
	if err := json.Unmarshal(respBody, &fees); err != nil {
		return nil, fmt.Errorf("failed to parse fee estimates: %w", err)
	}

	return &fees, nil
}
