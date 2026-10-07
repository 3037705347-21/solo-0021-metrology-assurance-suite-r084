# Metrology Assurance Suite

Metrology Assurance Suite is a Go HTTP service for bounded laboratory
measurement assurance work. It registers devices, confirms on-site
commissioning before a device can enter formal verification, opens verification
cases, records measurements, handles failed tolerance decisions, and seals
traceable evidence records.

## Run

```bash
go run ./cmd/metrology-service --addr 127.0.0.1:18121
```

The service listens on `127.0.0.1:18121` by default.

## Build And Verify

```bash
go build ./...
go run ./cmd/workflowcheck --workflow register-device
go run ./cmd/workflowcheck --workflow commission-device
go run ./cmd/workflowcheck --workflow open-verification-case
go run ./cmd/workflowcheck --workflow record-measurement
go run ./cmd/workflowcheck --workflow reopen-verification-case
go run ./cmd/workflowcheck --workflow seal-verification
```

The workflow runner starts a fresh in-process HTTP service and exercises the
public API. It is a production smoke command, not a unit test.

Tests are intentionally deferred for this initialization baseline. The later
engineering-task stage will add the unit and integration test suite and replace
or extend these smoke checks with red/green verification tests.

## API

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

New devices are registered as `pending_commissioning`. A responsible party
submits the installed location, foundation status, responsible person,
evidence, and outcome to `POST /devices/{id}/commissioning`. Rejected
confirmations are retained as history without changing the device; only an
accepted confirmation moves the device to `active`, and only commissioned
devices can open cases. Confirmations on an already enabled device are
rejected as conflicts and write nothing.

## Layout

- `cmd/metrology-service`: executable HTTP server.
- `cmd/workflowcheck`: bounded workflow verification commands.
- `internal/httpapi`: route and JSON boundary.
- `internal/assurance`: workflow orchestration.
- `internal/domain`: state rules and invariants.
- `internal/repository`: copy-on-write in-memory persistence.
- `internal/contracts`: API request and response models.
- `internal/bootstrap`: concrete dependency wiring.

## Inputs And Outputs

Inputs are JSON HTTP requests. Outputs are JSON response envelopes with either a
resource view or a stable error object:

```json
{
  "error": {
    "code": "state_conflict",
    "message": "case is not awaiting measurement"
  }
}
```

The service has no environment variables and no external dependencies.
