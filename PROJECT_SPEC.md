# Metrology Assurance Suite

## Project Goal

Metrology Assurance Suite is a local HTTP service that keeps a traceable record
of laboratory measurement assurance work. It registers devices, confirms each
device on site before it enters service, opens a verification case for one
commissioned device, records technician measurements, requests a repeat
measurement when tolerance is exceeded, and seals the final evidence record
after review.

The service is intentionally self-contained. It uses an in-memory repository
with atomic copy-on-write mutations and does not require a database or an
external service.

## Users

- Laboratory steward: registers devices, confirms commissioning, and opens
  verification cases.
- Responsible party: confirms a device's install location, foundation state,
  accountable owner, and evidence basis to put it into service.
- Measurement technician: records observed values for an open case.
- Quality verifier: reviews accepted measurements and seals the case.

## Core Entities

- Device: a laboratory asset with a unique asset tag, model, room, steward, and
  a single lifecycle state (`pending_commissioning -> active -> retired`).
- CommissioningConfirmation: the immutable site record that puts a device into
  service, naming the responsible party, install location, foundation state,
  evidence basis, and confirmer. Confirmations are append-only and stay
  readable after the device is enabled or retired.
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
tags, and creates a device in `pending_commissioning`. A registered device is
not yet in service and cannot open a case.

### 2. Confirm Device Commissioning

The responsible party sends `POST /devices/{id}/confirmations` with the
responsible party, install location, foundation state, evidence basis, and the
confirmer. The service validates the confirmation and, in one atomic commit,
appends the immutable confirmation, moves the device to `active`, stamps its
commissioning time once, and records an audit event. A malformed request, an
unknown device, or a repeat confirmation for an already commissioned device
fails without committing, so neither device facts nor history are rewritten.
The full confirmation history stays readable at
`GET /devices/{id}/confirmations` after the device is active.

### 3. Open Verification Case

The steward sends `POST /cases` with a device ID, nominal value, tolerance, and
owner. The service verifies that the device exists, is `active` (i.e. has
passed commissioning), and has no unfinished case. A `pending_commissioning`
device is rejected with a dedicated `device_not_commissioned` conflict. A valid
request creates a case in `awaiting_measurement`. The case view embeds the same
device lifecycle fact used by commissioning and retirement.

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

- `pending_commissioning -> active -> retired`
- only `active` devices can open verification cases.
- `retired` is terminal; a retired device retains its commissioning fact.

VerificationCase state:

- `awaiting_measurement -> review_ready`
- `awaiting_measurement -> recheck_required`
- `recheck_required -> awaiting_measurement`
- `review_ready -> sealed`
- `sealed` is terminal.

Required invariants:

- Asset tags are unique after case-insensitive normalization.
- A device is registered as `pending_commissioning` and is enabled exactly once
  by a successful commissioning confirmation.
- A device that has not passed commissioning cannot open a verification case.
- A failed or repeated confirmation commits nothing: it never rewrites the
  device status, commissioning time, or confirmation history.
- Commissioning confirmations are append-only and remain traceable after the
  device is active or retired.
- Commissioning, case reads, and later retirement all use the single device
  lifecycle fact (`Device.status` and its commissioning timestamp).
- A device has at most one unfinished verification case.
- Measurements are accepted only while a case is awaiting measurement.
- A tolerance must be positive.
- A case can have at most one evidence seal.
- A sealed case cannot accept measurements, reopen, or seal again.
- Every mutation that changes a case also appends an audit event in the same
  repository commit.

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
- `POST /devices/{id}/confirmations`
- `GET /devices/{id}/confirmations`
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
both success and failure behavior. The seal check also races two concurrent
seal requests to prove that the repository mutation lock permits exactly one
seal. The commissioning check races two concurrent confirmations for one
pending device to prove exactly one enabling confirmation is committed.

Tests are intentionally deferred for this initialization baseline. The later
engineering-task stage will add unit and integration tests, including red/green
verification for the workflows and their failure paths.
