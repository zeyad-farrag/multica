//go:build tools

// Pin Architecture-mandated libraries (Story 1.1 AC1) so `go mod tidy`
// keeps them as direct dependencies. Each is exercised in a later story:
//   - sqlc: queries codegen (Stories 1.6+)
//   - rrule-go: work-item recurrence (Story 3.1+)
//   - gorilla/websocket: Multica WS subscriber (Story 1.7)
//   - google/uuid: ID generation across services
//   - prometheus/client_golang: metrics endpoint
//
// chi/v5 and pgx/v5 are pinned via direct use in cmd/server/.
package tools

import (
	_ "github.com/google/uuid"
	_ "github.com/gorilla/websocket"
	_ "github.com/prometheus/client_golang/prometheus"
	_ "github.com/sqlc-dev/sqlc/cmd/sqlc"
	_ "github.com/teambition/rrule-go"
)
