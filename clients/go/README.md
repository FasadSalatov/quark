# Sael — Go server SDK

Reference Go implementation of the Sael Protocol v0.1.

## Install

```bash
go get github.com/FasadSalatov/sael/clients/go
```

## Usage

```go
package main

import (
    "context"
    "net/http"
    "strings"

    sael "github.com/FasadSalatov/sael/clients/go"
)

func main() {
    srv := sael.NewServer()

    srv.RegisterTool(sael.Tool{
        Name:        "echo.upper",
        Description: "Returns input text in uppercase",
        Effects:     []string{"pure"},
        Handler: func(ctx context.Context, in map[string]any) (any, error) {
            text, _ := in["text"].(string)
            return strings.ToUpper(text), nil
        },
    })

    sael.RegisterDemoTools(srv)

    http.Handle("/sael/ws", srv)
    http.ListenAndServe(":3011", nil)
}
```

## Features

- WebSocket transport
- Streaming + one-shot invocations
- Server-side pipeline composition
- Subscriptions
- Capability-based security
- Backpressure

See [`docs/spec.md`](../../docs/spec.md) for full protocol details.

## License

MIT
