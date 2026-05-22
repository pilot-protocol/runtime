# runtime

Pilot Protocol runtime glue. Wires the major subsystems together for
the daemon process: handshake, policy, identity, events, streams. This
is the package the daemon's `main.go` constructs and hands to the plugin
register.

## Layout

| File | What it does |
|---|---|
| `runtime.go` | `Runtime` struct + plugin registration / lifecycle. |
| `identity.go` | Load/save Ed25519 identity from disk. |
| `events.go` | In-process event bus implementation. |
| `streams.go` | `coreapi.Streams` listener registry on top of the driver. |
| `handshake.go` | Adapter that exposes the handshake plugin to the runtime. |
| `policy.go` | Adapter that exposes the policy plugin to the runtime. |
| `policy_manager.go` | Per-network policy-file load + reload helpers. |

## Import paths

```go
import "github.com/pilot-protocol/runtime"

rt, err := runtime.New(runtime.Config{
    IdentityPath: "~/.pilot/identity.json",
    PolicyDir:    "~/.pilot/policy",
    // ...
})
rt.Register(handshakeService)
rt.Register(policyService)
rt.Run(ctx)
```

This package is intentionally the last to extract — it depends on
both `pilot-protocol/handshake` and `pilot-protocol/policy`.

## Releasing

Tag a SemVer version (e.g. `v0.1.0`); web4 pulls it in via
`require github.com/pilot-protocol/runtime v0.1.0`. During
co-development the protocol repo uses `replace ../runtime`.
