# ieee-2030_5-core

IEEE 2030.5 (SEP2) Go core library for the PNNL GRIDAPPSD project.

## What this is

This module houses the shared, non-server-specific packages extracted from the
IEEE 2030.5 server implementation: the `pkg/sep2` type tree, `pkg/sep2tls`
TLS primitives (including the RFC 3610 CCM AEAD and the Go stdlib crypto/tls fork
that registers the CCM-8 cipher suite), `pkg/sep2srv` handler primitives (paging,
function-set handlers), and `pkg/sep2client/notify` (the notification receiver
primitive). Both the server and the client simulator consume this module.

## Status

Library extraction in progress. See `noor-extraction-plan-2026-06-18.md` in the
workspace at `projects/ieee-2030_5/ieee-2030_5-go/artifacts/outputs/` for the
full phase sequencing. This repository currently holds only the skeleton scaffold
(Phase B0); real Go packages arrive starting with Phase B1 (pkg/sep2 move).

## Parent server repository

`gitlab.pnnl.gov/arista/ieee-2030_5/ieee-2030_5-server`

## License

BSD-2-Clause. Copyright Battelle Memorial Institute. See LICENSE.
