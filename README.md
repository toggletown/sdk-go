# ToggleTown Go SDK

Official Go SDK for [ToggleTown](https://toggletown.com) feature flags.

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
    client := toggletown.NewClient("tt_live_your_api_key", nil)
    if err := client.Initialize(); err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    enabled := client.GetBooleanFlag("new-feature", false, map[string]interface{}{
        "user_id": "user-123",
        "plan":    "pro",
    })

    if enabled {
        // New feature code
    }
}
```

## Flag Types

```go
enabled := client.GetBooleanFlag("feature-x", false, context)
variant := client.GetStringFlag("checkout-flow", "control", context)
limit := client.GetNumberFlag("rate-limit", 100.0, context)
config := client.GetJSONFlag("dashboard-config", defaultConfig, context)
```

## Documentation

Full documentation with configuration, targeting rules, framework integration (net/http, Gin, Echo), and best practices:

**[Go SDK Guide](https://docs.toggletown.com/sdks/go)**

## License

MIT
