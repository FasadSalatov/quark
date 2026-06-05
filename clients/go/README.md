# Quark — Go server SDK

Reference Go implementation of the Quark Protocol v0.1.

## Install

```bash
go get github.com/FasadSalatov/quark/clients/go
```

## Usage

```go
package main

import (
    "context"
    "net/http"
    "strings"

    quark "github.com/FasadSalatov/quark/clients/go"
)

func main() {
    srv := quark.NewServer()

    srv.RegisterTool(quark.Tool{
        Name:        "echo.upper",
        Description: "Returns input text in uppercase",
        Effects:     []string{"pure"},
        Handler: func(ctx context.Context, in map[string]any) (any, error) {
            text, _ := in["text"].(string)
            return strings.ToUpper(text), nil
        },
    })

    quark.RegisterDemoTools(srv)

    http.Handle("/quark/ws", srv)
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
