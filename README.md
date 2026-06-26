# ieee-2030_5-core

[![Pipeline](https://gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/badges/main/pipeline.svg)](https://gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/-/commits/main)
[![Coverage](https://gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/badges/main/coverage.svg)](https://gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core/-/commits/main)
[![Go 1.26](https://img.shields.io/badge/go-1.26-blue)](https://go.dev)
[![License](https://img.shields.io/badge/license-Battelle%20BSD-blue)](LICENSE)

Shared Go library for the IEEE 2030.5 (SEP2) server and client implementations
at PNNL. Both the reference server (`ieee-2030_5-server`) and the client
simulator (`ieee-2030_5-client`) import this module.

## What this is

This module houses the packages that are common to both the server and the
client: the SEP2 type tree, the XML codec, the TLS stack, the certificate
utilities, the store interfaces and their in-memory implementation, the
server-side HTTP handlers and list-paging helpers, the server-assembly scaffold,
and the inbound notification receiver.

## Packages

| Import path | Purpose |
|---|---|
| `pkg/sep2` | SEP2 resource types: DER, end device, configuration, messaging, metering, power, registration, and all list variants |
| `pkg/sep2/encoding` | XML encoder/decoder, namespace constants, and response-body helpers |
| `pkg/sep2cert` | SEP2 X.509 certificate generation, OID definitions, and PEM I/O |
| `pkg/sep2client/notify` | Inbound HTTPS notification receiver for CSIP subscription/notification flows |
| `pkg/sep2srv/assembly` | Server-assembly scaffold: `BuildProtocolRouter`, `Stores`, `RouterConfig`, `AuthPolicy` |
| `pkg/sep2srv/handlers/*` | Per-function-set HTTP handlers (configuration, dcap, DER, device info, end device, flow reservation, FSA, list, log event, messaging, metering, power status, registration, self device, time, singleton, subscription) |
| `pkg/sep2srv/paging` | List paging helpers (`AllResults`, cursor extraction) |
| `pkg/sep2tls` | SEP2 TLS stack: CCM AEAD cipher, custom Go TLS fork that registers the CCM-8 suite, peer identity, and verification |
| `pkg/sep2tls/ccm` | RFC 3610 CCM AEAD implementation |
| `pkg/store` | Store interfaces (end device, subscription) |
| `pkg/store/memory` | In-memory store implementation with persistence hooks |

## Build and test

```
go build ./...
go test ./...
go test -race ./...
```

All packages compile and pass tests with the standard `go` toolchain. No
external services or network access are required to run the test suite.

## Importing this module

This module follows the server and client repositories into production together.
During the pre-1.0 period, consumers reference it via a `replace` directive
in their `go.mod`:

```
require gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core v0.0.0

replace gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-core => ../ieee-2030_5-core
```

Adjust the relative path to match your local checkout layout.

## Related repositories

- `gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-server`: reference IEEE 2030.5 server
- `gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-client`: client simulator

## License

Battelle BSD (modified BSD with a Battelle name-use clause and DOE disclaimer).
Copyright Battelle Memorial Institute, operated by Battelle for the U.S.
Department of Energy under Contract DE-AC05-76RL01830. See LICENSE.
