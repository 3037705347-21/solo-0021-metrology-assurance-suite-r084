# Metrology Assurance Suite

## Project Goal

Metrology Assurance Suite is a local HTTP service that keeps a traceable record
of laboratory measurement assurance work. It registers devices, confirms their
on-site commissioning before they can be used, opens a verification case for one
device, records technician measurements, requests a repeat measurement when
tolerance is exceeded, and seals the final evidence record after review.

The service is intentionally self-contained. It uses an in-memory repository
with atomic copy-on-write mutations and does not require a database or an
external service.

## Users

- Laboratory steward: registers devices and opens verification cases.
- Commissioning responsible party: confirms the installation location,
  foundation status, and evidence before a device enters formal verification.
- Measurement technician: records observed values for an open case.
- Quality verifier: reviews accepted measurements and seals the case.

## Core Entities

- Device: a laboratory asset with a unique asset tag, model, room, steward, and
  lifecycle state.
- CommissioningConfirmation: an immutable on-site confirmation fact naming the
  responsible party, installed location, foundation status, evidence, and the
  accepted or rejected outcome.
- VerificationCase: one bounded assurance activity for a device, with a nominal
  value, tolerance, current state, version, and assigned steward.
- Measurement: an observed value, computed deviation, acceptance decision,
  technician, and timestamp.
- EvidenceSeal: the immutable closing record for a reviewed case, including the
  selected accepted measurement.
- AuditEvent: an append-only trace of meaningful case and device changes.

## Workflows

### 1. Register Device

The steward sends `POST /devices` with an asset tag, model, room, and steward.
The service validates the request, normalizes the asset tag, rejects duplicate
tags, and creates a device in `pending_commissioning`. A new device is not yet
eligible to open a case.

### 2. Confirm Device Commissioning

The responsible party sends `POST /devices/{id}/commissioning` with the
responsible party, installed location, foundation status, confirmation evidence,
and an outcome. A rejected confirmation is appended to the device history with
an audit event but does not change any device fact, so the device stays pending
and may be confirmed again. An accepted confirmation appends the same immutable
fact and moves the device to `active`, recording the commissioning time. A
confirmation against a device that is already active or retired is a state
conflict and writes nothing. `GET /devices/{id}/commissioning` returns the
device together with every confirmation and device audit event in order, so the
full history remains traceable after the device is enabled.

### 3. Open Verification Case

The steward sends `POST /cases` with a device ID, nominal value, tolerance, and
owner. The service verifies that the device exists, has been commissioned and is
active, and has no unfinished case. A valid request creates a case in
`awaiting_measurement`. An uncommissioned device is rejected with
`device_not_commissioned`.

### 4. Record Measurement

A technician sends `POST /cases/{id}/measurements`. The case must be
`awaiting_measurement`. The service computes the signed deviation from the
nominal value. An in-tolerance measurement moves the case to
`review_ready`; an out-of-tolerance measurement moves it to
`recheck_required`.

### 5. Reopen Verification Case

The steward sends `POST /cases/{id}/reopen` after an out-of-tolerance result.
Only `recheck_required` cases may move back to `awaiting_measurement`. The
service records the actor and reason in the audit trail.

### 6. Seal Verification

The verifier sends `POST /cases/{id}/seal` after an accepted measurement. Only
`review_ready` cases may be sealed. The service selects the accepted
measurement with the smallest absolute deviation, creates exactly one evidence
seal, moves the case to `sealed`, and rejects later mutations.

## State And Rules

Device state:

- `pending_commissioning -> active`
- `active -> retired`
- `retired` is terminal.

VerificationCase state:

- `awaiting_measurement -> review_ready`
- `awaiting_measurement -> recheck_required`
- `recheck_required -> awaiting_measurement`
- `review_ready -> sealed`
- `sealed` is terminal.

Required invariants:

- Asset tags are unique after case-insensitive normalization.
- A new device is `pending_commissioning` and cannot open a case until an
  accepted commissioning confirmation enables it.
- Rejected and duplicate confirmations never overwrite device facts; rejected
  confirmations stay traceable, and a confirmation on a non-pending device
  writes nothing.
- Accepted confirmations and commissioning history are immutable and remain
  readable after the device is enabled; later retirement and case reads use the
  same single device lifecycle fact (status and commissioning time).
- A commissioned device has at most one unfinished verification case.
- Measurements are accepted only while a case is awaiting measurement.
- A tolerance must be positive.
- A case can have at most one evidence seal.
- A sealed case cannot accept measurements, reopen, or seal again.
- Every mutation that changes a device or a case also appends an audit event in
  the same repository commit.

## Modules

- `cmd/metrology-service`: process entrypoint and graceful HTTP shutdown.
- `cmd/workflowcheck`: bounded smoke checks that call the public HTTP API.
- `internal/httpapi`: routing, request decoding, and stable error mapping.
- `internal/assurance`: workflow orchestration and atomic decisions.
- `internal/domain`: entities, validation, state transitions, and errors.
- `internal/repository`: mutex-protected in-memory persistence with copy-on-write
  commits.
- `internal/contracts`: request and response contracts shared by the HTTP layer.

## Public Interfaces

- `GET /healthz`
- `POST /devices`
- `GET /devices/{id}`
- `POST /devices/{id}/commissioning`
- `GET /devices/{id}/commissioning`
- `POST /cases`
- `GET /cases/{id}`
- `POST /cases/{id}/measurements`
- `POST /cases/{id}/reopen`
- `POST /cases/{id}/seal`
- `GET /cases/{id}/events`

## Validation Plan

The baseline build command is `go build ./...`. Each declared workflow has a
bounded production smoke command under `cmd/workflowcheck`. The command starts
an HTTP test server in the same process, calls the public routes, and verifies
both success and failure behavior. The commissioning check covers a blocked
case on a pending device, a rejected confirmation that preserves device facts,
a successful enabling confirmation with full history, an idempotent-style
repeated confirmation conflict, and a concurrent accepted-confirmation race
that enables the device exactly once. The seal check also races two concurrent
seal requests to prove that the repository mutation lock permits exactly one
seal.

Tests are intentionally deferred for this initialization baseline. The later
engineering-task stage will add unit and integration tests, including red/green
verification for the workflows and their failure paths.
