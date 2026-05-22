# runtime

[![ci](https://github.com/pilot-protocol/runtime/actions/workflows/ci.yml/badge.svg)](https://github.com/pilot-protocol/runtime/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/pilot-protocol/runtime/branch/main/graph/badge.svg)](https://codecov.io/gh/pilot-protocol/runtime)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

Runtime glue for the Pilot Protocol daemon. Wires the major subsystems
together — handshake, policy, identity, events, streams — into a single
`Runtime` value the daemon's `main.go` constructs and hands to the
plugin register.

## Install

```go
import "github.com/pilot-protocol/runtime"
```

## Usage

```go
rt, err := runtime.New(runtime.Config{
    IdentityPath: "~/.pilot/identity.json",
    PolicyDir:    "~/.pilot/policy",
    // ...
})
if err != nil {
    return err
}
rt.Register(handshakeService)
rt.Register(policyService)
rt.Run(ctx)
```

## Layout

| File | What it does |
|---|---|
| `runtime.go` | `Runtime` struct, plugin registration, lifecycle. |
| `identity.go` | Load/save Ed25519 identity from disk. |
| `events.go` | In-process event bus implementation. |
| `streams.go` | `coreapi.Streams` listener registry on top of the driver. |
| `handshake.go` | Adapter that exposes the handshake plugin to the runtime. |
| `policy.go` | Adapter that exposes the policy plugin to the runtime. |
| `policy_manager.go` | Per-network policy-file load and reload helpers. |

## License

AGPL-3.0-or-later. See [LICENSE](LICENSE).
