# ieee-2030_5-core

[![Build, vet, and test](https://github.com/GRIDAPPSD/ieee-2030_5-core-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/GRIDAPPSD/ieee-2030_5-core-go/actions/workflows/ci.yml)
[![CodeQL](https://github.com/GRIDAPPSD/ieee-2030_5-core-go/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/GRIDAPPSD/ieee-2030_5-core-go/actions/workflows/codeql.yml)
[![Go 1.26.3](https://img.shields.io/badge/go-1.26.3-00ADD8?logo=go)](https://go.dev)
[![Release v0.7.0](https://img.shields.io/badge/release-v0.7.0-blue)](https://github.com/GRIDAPPSD/ieee-2030_5-core-go/releases/latest)
[![License](https://img.shields.io/badge/license-Battelle%20BSD-blue)](LICENSE)

This repo is private: the workflow badges above render for viewers with
repository access and show nothing for anonymous visitors.

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

### The IEEE 2030.5 schema is not included

The normative IEEE 2030.5 XML Schema (`sep.xsd`) is copyrighted by IEEE and is
**not distributed with this repository**, in any release artifact, or in any
binary built from this module. It is used only as a test fixture: the
wire-format gate in `internal/xsdgate` reads it at test time and validates
marshalled SEP2 resources against it.

The commands above run green without it. The schema-gated tests report as
SKIP, naming `SEP2_SCHEMA_PATH`, and every other test runs normally.

To run the gate, obtain the standard at no charge through the IEEE GET
Program (see [NOTICE](NOTICE)) and either drop the file at `schema/sep.xsd`,
which is gitignored, or point at it:

```
export SEP2_SCHEMA_PATH=/path/to/sep.xsd
go test ./...
```

Verify the copy first: `schema/PROVENANCE.md` records the digest the tests
check and the exact model release this module gates against. A copy that is
not that document is rejected with one clear error.

Because a skip is not a validation, CI or anyone else who intends to supply
the schema should also set `SEP2_SCHEMA_REQUIRED=1`, which turns a missing or
misconfigured copy into a hard failure instead of a silent skip. The single
test to look at is `TestSchemaGateArmed` in `internal/xsdgate`: it passes only
when a verified schema was loaded.

Continuous integration supplies the schema the same way. It reassembles a
licensed copy from repository secrets into a file outside the checkout, points
`SEP2_SCHEMA_PATH` at it, and sets `SEP2_SCHEMA_REQUIRED=1`, so a decode or
path failure fails the run rather than skipping quietly. A pull request from a
fork receives no secrets, so its run skips the gated tests and still reports
green. The schema is never written into the working tree, never committed, and
never appears in a log or a build artifact.

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
