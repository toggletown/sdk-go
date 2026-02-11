// Package toggletown provides a Go SDK for ToggleTown feature flags.
//
// Example usage:
//
//	client := toggletown.NewClient("tt_live_your_api_key", nil)
//	if err := client.Initialize(); err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	enabled := client.GetBooleanFlag("new-feature", false, map[string]interface{}{
//	    "user_id": "user-123",
//	    "plan":    "pro",
//	})
package toggletown

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	// DefaultAPIURL is the default ToggleTown API URL
	DefaultAPIURL = "https://api.toggletown.com"
	// DefaultPollingInterval is the default polling interval
	DefaultPollingInterval = 30 * time.Second
)

// Rule represents a targeting rule
type Rule struct {
	Attribute  string      `json:"attribute"`
	Operator   string      `json:"operator"`
	Value      interface{} `json:"value"`
	Percentage *int        `json:"percentage,omitempty"`
	RollValue  interface{} `json:"rollValue,omitempty"`
}

// FlagConfig represents a flag configuration from the API
type FlagConfig struct {
	Key               string      `json:"key"`
	Type              string      `json:"type"`
	Enabled           bool        `json:"enabled"`
	DefaultValue      interface{} `json:"defaultValue"`
	Rules             []Rule      `json:"rules"`
	RolloutPercentage int         `json:"rolloutPercentage"`
}

// FlagsResponse is the API response for fetching flags
type FlagsResponse struct {
	Flags map[string]FlagConfig `json:"flags"`
}

// Config holds configuration options for the client
type Config struct {
	// APIURL is the base URL for the ToggleTown API
	APIURL string
	// PollingInterval is how often to refresh flags
	PollingInterval time.Duration
	// OnError is called when flag fetching fails during polling
	OnError func(error)
	// HTTPClient allows using a custom HTTP client
	HTTPClient *http.Client
}

// Client is the ToggleTown feature flag client
type Client struct {
	apiKey          string
	apiURL          string
	pollingInterval time.Duration
	onError         func(error)
	httpClient      *http.Client

	flags       map[string]FlagConfig
	mu          sync.RWMutex
	initialized bool
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewClient creates a new ToggleTown client
func NewClient(apiKey string, config *Config) *Client {
	c := &Client{
		apiKey:          apiKey,
		apiURL:          DefaultAPIURL,
		pollingInterval: DefaultPollingInterval,
		httpClient:      http.DefaultClient,
		flags:           make(map[string]FlagConfig),
		stopChan:        make(chan struct{}),
	}

	if config != nil {
		if config.APIURL != "" {
			c.apiURL = config.APIURL
		}
		if config.PollingInterval > 0 {
			c.pollingInterval = config.PollingInterval
		}
		if config.OnError != nil {
			c.onError = config.OnError
		}
		if config.HTTPClient != nil {
			c.httpClient = config.HTTPClient
		}
	}

	return c
}

// Initialize fetches flags and starts background polling
func (c *Client) Initialize() error {
	if c.initialized {
		return nil
	}

	// Initial fetch - returns error on failure
	if err := c.fetchFlagsInitial(); err != nil {
		return err
	}

	c.initialized = true
	c.startPolling()
	return nil
}

// IsInitialized returns whether the client has been initialized
func (c *Client) IsInitialized() bool {
	return c.initialized
}

// Close stops polling and releases resources
func (c *Client) Close() {
	if c.stopChan != nil {
		close(c.stopChan)
		c.wg.Wait()
	}
}

func (c *Client) fetchFlagsInitial() error {
	req, err := http.NewRequest("GET", c.apiURL+"/api/v1/sdk/flags", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch flags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var flagsResp FlagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&flagsResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	c.mu.Lock()
	c.flags = flagsResp.Flags
	c.mu.Unlock()

	return nil
}

func (c *Client) fetchFlags() {
	req, err := http.NewRequest("GET", c.apiURL+"/api/v1/sdk/flags", nil)
	if err != nil {
		if c.onError != nil {
			c.onError(err)
		}
		return
	}

	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.onError != nil {
			c.onError(err)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if c.onError != nil {
			body, _ := io.ReadAll(resp.Body)
			c.onError(fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body)))
		}
		return
	}

	var flagsResp FlagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&flagsResp); err != nil {
		if c.onError != nil {
			c.onError(err)
		}
		return
	}

	c.mu.Lock()
	c.flags = flagsResp.Flags
	c.mu.Unlock()
}

func (c *Client) startPolling() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.pollingInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.fetchFlags()
			case <-c.stopChan:
				return
			}
		}
	}()
}

func (c *Client) getFlagConfig(key string) (FlagConfig, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config, ok := c.flags[key]
	return config, ok
}

// GetBooleanFlag returns a boolean flag value
func (c *Client) GetBooleanFlag(key string, defaultValue bool, context map[string]interface{}) bool {
	config, ok := c.getFlagConfig(key)
	if !ok {
		return defaultValue
	}

	value := evaluateFlag(config, context)
	if v, ok := value.(bool); ok {
		return v
	}
	return defaultValue
}

// GetStringFlag returns a string flag value
func (c *Client) GetStringFlag(key string, defaultValue string, context map[string]interface{}) string {
	config, ok := c.getFlagConfig(key)
	if !ok {
		return defaultValue
	}

	value := evaluateFlag(config, context)
	if v, ok := value.(string); ok {
		return v
	}
	return defaultValue
}

// GetNumberFlag returns a number flag value
func (c *Client) GetNumberFlag(key string, defaultValue float64, context map[string]interface{}) float64 {
	config, ok := c.getFlagConfig(key)
	if !ok {
		return defaultValue
	}

	value := evaluateFlag(config, context)
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return defaultValue
	}
}

// GetJSONFlag returns a JSON flag value
func (c *Client) GetJSONFlag(key string, defaultValue interface{}, context map[string]interface{}) interface{} {
	config, ok := c.getFlagConfig(key)
	if !ok {
		return defaultValue
	}

	value := evaluateFlag(config, context)
	if value != nil {
		return value
	}
	return defaultValue
}

// GetAllFlags returns all flag configurations (for debugging)
func (c *Client) GetAllFlags() map[string]FlagConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]FlagConfig, len(c.flags))
	for k, v := range c.flags {
		result[k] = v
	}
	return result
}
