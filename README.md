# ToggleTown Go SDK

Official Go SDK for [ToggleTown.com](https://toggletown.com) feature flags.

## Installation

```bash
go get github.com/toggletown/sdk-go
```

## Quick Start

```go
package main

import (
    "log"
    toggletown "github.com/toggletown/sdk-go"
)

func main() {
    // Create client
    client := toggletown.NewClient("tt_live_your_api_key", nil)

    // Initialize (fetches flags and starts polling)
    if err := client.Initialize(); err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Evaluate flags
    enabled := client.GetBooleanFlag("new-feature", false, map[string]interface{}{
        "user_id": "user-123",
        "plan":    "pro",
    })

    if enabled {
        // New feature code
    }
}
```

## Configuration

```go
client := toggletown.NewClient("tt_live_your_api_key", &toggletown.Config{
    APIURL:          "https://api.toggletown.com",  // Custom API URL
    PollingInterval: 60 * time.Second,              // Default: 30s
    OnError: func(err error) {
        log.Printf("Flag fetch error: %v", err)
    },
    HTTPClient: &http.Client{
        Timeout: 10 * time.Second,
    },
})
```

## Flag Types

### Boolean Flags

```go
enabled := client.GetBooleanFlag("feature-x", false, context)
```

### String Flags

```go
variant := client.GetStringFlag("checkout-flow", "control", context)
```

### Number Flags

```go
limit := client.GetNumberFlag("rate-limit", 100.0, context)
```

### JSON Flags

```go
config := client.GetJSONFlag("dashboard-config", defaultConfig, context)
```

## Context

Pass user attributes for targeting rules:

```go
context := map[string]interface{}{
    "user_id": "user-123",      // Required for rollouts
    "plan":    "pro",
    "country": "US",
    "age":     25,
}

enabled := client.GetBooleanFlag("premium-feature", false, context)
```

## Targeting Rules

The SDK evaluates targeting rules locally. Supported operators:

| Operator | Description | Example |
|----------|-------------|---------|
| `equals` | Exact match | `plan equals "pro"` |
| `not_equals` | Not equal | `plan not_equals "free"` |
| `contains` | String contains | `email contains "@company.com"` |
| `not_contains` | String doesn't contain | `email not_contains "@test.com"` |
| `gt` | Greater than | `age gt 18` |
| `lt` | Less than | `age lt 65` |
| `in` | In list | `country in ["US", "CA", "UK"]` |
| `not_in` | Not in list | `country not_in ["CN", "RU"]` |

## Rollout Percentages

Flags can be gradually rolled out to a percentage of users. The rollout is deterministic based on `user_id` and flag key, ensuring consistent experiences.

```go
// User will always get the same result for a given flag
context := map[string]interface{}{"user_id": "user-123"}
enabled := client.GetBooleanFlag("new-checkout", false, context)
```

## HTTP Integration

### net/http Middleware

```go
func featureFlagMiddleware(client *toggletown.Client) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            userID := r.Header.Get("X-User-ID")
            ctx := context.WithValue(r.Context(), "flags", client)
            ctx = context.WithValue(ctx, "user_id", userID)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func handler(w http.ResponseWriter, r *http.Request) {
    client := r.Context().Value("flags").(*toggletown.Client)
    userID := r.Context().Value("user_id").(string)

    enabled := client.GetBooleanFlag("new-api", false, map[string]interface{}{
        "user_id": userID,
    })

    if enabled {
        // New API response
    }
}
```

### Gin Framework

```go
func FeatureFlagMiddleware(client *toggletown.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Set("flags", client)
        c.Next()
    }
}

func main() {
    client := toggletown.NewClient("tt_live_xxx", nil)
    client.Initialize()
    defer client.Close()

    r := gin.Default()
    r.Use(FeatureFlagMiddleware(client))

    r.GET("/api/data", func(c *gin.Context) {
        fc := c.MustGet("flags").(*toggletown.Client)
        userID := c.GetHeader("X-User-ID")

        enabled := fc.GetBooleanFlag("new-api", false, map[string]interface{}{
            "user_id": userID,
        })

        c.JSON(200, gin.H{"new_api": enabled})
    })

    r.Run()
}
```

### Echo Framework

```go
func FeatureFlagMiddleware(client *toggletown.Client) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            c.Set("flags", client)
            return next(c)
        }
    }
}

func main() {
    client := toggletown.NewClient("tt_live_xxx", nil)
    client.Initialize()
    defer client.Close()

    e := echo.New()
    e.Use(FeatureFlagMiddleware(client))

    e.GET("/api/data", func(c echo.Context) error {
        fc := c.Get("flags").(*toggletown.Client)
        userID := c.Request().Header.Get("X-User-ID")

        enabled := fc.GetBooleanFlag("new-api", false, map[string]interface{}{
            "user_id": userID,
        })

        return c.JSON(200, map[string]bool{"new_api": enabled})
    })

    e.Start(":8080")
}
```

## Thread Safety

The client is fully thread-safe. You can share a single client instance across all goroutines:

```go
var flagClient *toggletown.Client

func init() {
    flagClient = toggletown.NewClient("tt_live_xxx", nil)
    if err := flagClient.Initialize(); err != nil {
        log.Fatal(err)
    }
}

// Safe to call from any goroutine
func checkFeature(userID string) bool {
    return flagClient.GetBooleanFlag("feature", false, map[string]interface{}{
        "user_id": userID,
    })
}
```

## Error Handling

```go
client := toggletown.NewClient("tt_live_xxx", &toggletown.Config{
    OnError: func(err error) {
        // Log errors during background polling
        log.Printf("ToggleTown error: %v", err)

        // Send to error tracking service
        sentry.CaptureException(err)
    },
})

// Initialize returns error on first fetch failure
if err := client.Initialize(); err != nil {
    // Handle initialization failure
    log.Fatalf("Failed to initialize flags: %v", err)
}
```

## Debugging

```go
// Get all flag configurations
flags := client.GetAllFlags()
for key, config := range flags {
    fmt.Printf("Flag: %s, Enabled: %v, Rollout: %d%%\n",
        key, config.Enabled, config.RolloutPercentage)
}

// Check if client is initialized
if client.IsInitialized() {
    // Safe to evaluate flags
}
```

## Best Practices

1. **Initialize once** - Create one client per application
2. **Reuse the client** - Share across goroutines (it's thread-safe)
3. **Handle initialization errors** - The first fetch must succeed
4. **Always provide user_id** - Required for percentage rollouts
5. **Use appropriate defaults** - Choose safe fallback values
6. **Close on shutdown** - Call `client.Close()` for graceful shutdown

## License

MIT
