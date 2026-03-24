# Progress Log

## Completed

- [x] Task-001: Extract constants, config, and helpers
- [x] Task-002: Extract session and cookie management
- [x] Task-003: Extract portfolio handlers and data
- [x] Task-004: Extract soccer handlers
- [x] Task-005: Extract Google OAuth and Calendar

## Current Iteration

- Iteration: 6
- Working on: Task-006 (pending assignment)
- Started: pending

## Last Completed

- Task-005: Extract Google OAuth and Calendar
- Tests: ✅ All passing (cached)
- Build: ✅ Compiles cleanly
- Key decisions:
  - `google_oauth.go` (392 lines, 24 functions): connection store types/methods, OAuth handlers, token management, DynamoDB store
  - `google_calendar.go` (558 lines, 22 functions): Calendar API types, event CRUD, calendar list/selection, event payload builder
  - Removed 10 unused imports from main.go (bytes, crypto/rand, oauth2, partials, awsconfig, attributevalue, dynamodb, dynamodbTypes)
  - main.go reduced from ~2254 to 1337 lines (50 functions remaining)
  - Route registrations remain in main()

## Blockers

- None

## Notes for Next Iteration

- main.go (1337 lines, 50 functions) still has: LPS client, LPS decode, schedule resolvers, schedule time utils, ICS builder, main()
- All route registrations remain in main() inside main.go
- Next candidates: lps_client.go, lps_schedule.go, lps_decode.go, schedule.go, schedule_time.go, schedule_ics.go
