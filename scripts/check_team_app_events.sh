#!/usr/bin/env bash
set -euo pipefail

events_file="server/pkg/protocol/events.go"

assert_team_app_event() {
  local name="$1"
  local value="$2"

  if ! grep -Eq "^[[:space:]]*${name}[[:space:]]*=[[:space:]]*\"${value}\"" "$events_file"; then
    echo "ERROR: ${name} must remain ${value} in ${events_file}. Coordinate with the team-app integration owner before renaming or changing this event contract." >&2
    exit 1
  fi

  if ! awk -v name="$name" '
    /TEAM_APP_INTEGRATION:/ { marker = NR }
    $0 ~ "^[[:space:]]*" name "[[:space:]]*=" {
      if (marker > 0 && NR - marker <= 3) {
        found = 1
      }
      exit
    }
    END { exit found ? 0 : 1 }
  ' "$events_file"; then
    echo "ERROR: ${name} is missing a nearby // TEAM_APP_INTEGRATION: marker in ${events_file}. Coordinate with the team-app integration owner before changing this event contract." >&2
    exit 1
  fi
}

assert_team_app_event "EventCommentCreated" "comment:created"
assert_team_app_event "EventWorkspaceUpdated" "workspace:updated"
assert_team_app_event "EventMemberAdded" "member:added"
assert_team_app_event "EventMemberUpdated" "member:updated"
assert_team_app_event "EventMemberRemoved" "member:removed"

echo "✅ team-app integration event contract intact"
