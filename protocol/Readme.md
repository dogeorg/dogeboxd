# Protobuf schemas for the dogeboxd http api

Update the schemas here then run `make sync-api`

## DTO schemas vs the current JSON wire format

The messages in `dogebox/v1/` are the master definitions for the payloads
dogeboxd serves today, but most endpoints still serialise Go structs with
`encoding/json` rather than protojson. Until endpoints migrate to
connectrpc, generated client types (e.g. dpanel's `src/gen/`) need thin
adapters for the known divergences below.

| Field | JSON wire (Go struct tag) | Generated from proto |
| --- | --- | --- |
| `PupState.WebUIs` | `webUIs` | `webUis` |
| `JobRecord.PupID` | `pupID` | `pupId` |
| `ActionProgress.StepTaken` | `step_taken` (nanoseconds number) | `stepTaken` (Duration) |
| `PupManifestConfigField.Default` | `default` | `defaultValue` |
| enum-typed fields | string constants (e.g. `"installing"`, `"in_progress"`) | proto enum values |
| `time.Time` fields | RFC3339 strings | `google.protobuf.Timestamp` |

Other wire quirks worth knowing:

- The `stats` websocket change carries a bare `PupStats[]` array as its
  `update`, not a `StatsUpdate` wrapper.
- The JobManager emits `job:*` (colon) change types; the SystemUpdater
  completion path still emits the legacy `job_completed` underscore variant.
- `/ws/log/*` sockets send plain JSON-encoded strings, not `Change`
  envelopes.
