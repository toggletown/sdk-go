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
	// OnStale is called once when flags become stale (age exceeds MaxStaleAge after a poll failure)
	OnStale func(lastUpdatedAt time.Time, age time.Duration)
	// MaxStaleAge is the maximum age before flags are considered stale (default: 5 minutes)
	MaxStaleAge time.Duration
	// HTTPClient allows using a custom HTTP client
	HTTPClient *http.Client
}

const DefaultMaxStaleAge = 5 * time.Minute

// ConnectionStatus represents the staleness status of cached flags
type ConnectionStatus struct {
	Status        string    // "fresh" or "stale"
	LastUpdatedAt time.Time // zero value if never updated
	Age           time.Duration
}

// Client is the ToggleTown feature flag client
type Client struct {
	apiKey          string
	apiURL          string
	pollingInterval time.Duration
	onError         func(error)
	onStale         func(lastUpdatedAt time.Time, age time.Duration)
	maxStaleAge     time.Duration
	httpClient      *http.Client

	flags         map[string]FlagConfig
	mu            sync.RWMutex
	initialized   bool
	stopChan      chan struct{}
	wg            sync.WaitGroup
	lastUpdatedAt time.Time
	staleFired    bool
}

// NewClient creates a new ToggleTown client
func NewClient(apiKey string, config *Config) *Client {
	c := &Client{
		apiKey:          apiKey,
		apiURL:          DefaultAPIURL,
		pollingInterval: DefaultPollingInterval,
		maxStaleAge:     DefaultMaxStaleAge,
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
		if config.OnStale != nil {
			c.onStale = config.OnStale
		}
		if config.MaxStaleAge > 0 {
			c.maxStaleAge = config.MaxStaleAge
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
	c.lastUpdatedAt = time.Now()
	c.staleFired = false
	c.mu.Unlock()

	return nil
}

func (c *Client) fetchFlags() {
	req, err := http.NewRequest("GET", c.apiURL+"/api/v1/sdk/flags", nil)
	if err != nil {
		if c.onError != nil {
			c.onError(err)
		}
		c.checkStaleness()
		return
	}

	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.onError != nil {
			c.onError(err)
		}
		c.checkStaleness()
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if c.onError != nil {
			body, _ := io.ReadAll(resp.Body)
			c.onError(fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body)))
		}
		c.checkStaleness()
		return
	}

	var flagsResp FlagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&flagsResp); err != nil {
		if c.onError != nil {
			c.onError(err)
		}
		c.checkStaleness()
		return
	}

	c.mu.Lock()
	c.flags = flagsResp.Flags
	c.lastUpdatedAt = time.Now()
	c.staleFired = false
	c.mu.Unlock()
}

func (c *Client) checkStaleness() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastUpdatedAt.IsZero() {
		return
	}
	age := time.Since(c.lastUpdatedAt)
	if age > c.maxStaleAge && !c.staleFired {
		c.staleFired = true
		if c.onStale != nil {
			c.onStale(c.lastUpdatedAt, age)
		}
	}
}

// GetLastUpdatedAt returns the timestamp of the last successful flag fetch
func (c *Client) GetLastUpdatedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastUpdatedAt
}

// IsStale returns whether cached flags are stale (age exceeds MaxStaleAge)
func (c *Client) IsStale() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lastUpdatedAt.IsZero() {
		return false
	}
	return time.Since(c.lastUpdatedAt) > c.maxStaleAge
}

// GetStatus returns the connection status including staleness info
func (c *Client) GetStatus() ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lastUpdatedAt.IsZero() {
		return ConnectionStatus{Status: "fresh", Age: 0}
	}
	age := time.Since(c.lastUpdatedAt)
	status := "fresh"
	if age > c.maxStaleAge {
		status = "stale"
	}
	return ConnectionStatus{
		Status:        status,
		LastUpdatedAt: c.lastUpdatedAt,
		Age:           age,
	}
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
