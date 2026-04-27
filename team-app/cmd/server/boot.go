package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// requiredEnvVars enumerates the variables the team-app server validates on boot
// (Story 1.1 AC5, AR8). AR8 nominally lists eight; this list includes
// TEAM_APP_SYSTEM_USER_ID for safety. Story 1.9 will add the system-PAT identity
// assertion against GET /api/me — this file only validates presence.
//
// Order is intentional and matches the Dev Notes "Environment Variables" table.
var requiredEnvVars = []string{
	"DEFAULT_ORG_SLUG",
	"DEFAULT_ORG_NAME",
	"ORG_CREATION_ENABLED",
	"TEAM_APP_SHARED_SECRET",
	"TEAM_APP_URL",
	"MULTICA_BASE_URL",
	"MULTICA_WS_URL",
	"TEAM_APP_SYSTEM_USER_PAT",
	"TEAM_APP_SYSTEM_USER_ID",
}

// MissingEnvVarError is returned by validateEnv when a required variable is
// missing, empty, or (for ORG_CREATION_ENABLED) unparseable.
type MissingEnvVarError struct {
	Name   string
	Reason string
}

func (e *MissingEnvVarError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("missing_env_var=%s reason=%s", e.Name, e.Reason)
	}
	return fmt.Sprintf("missing_env_var=%s", e.Name)
}

// validateEnv reads the required environment variables and returns the first
// validation failure as a MissingEnvVarError. Empty values are treated as
// missing; ORG_CREATION_ENABLED must additionally be a parseable boolean
// (per AR8: "false" in v1, but accept any case-insensitive true/false).
func validateEnv() error {
	for _, name := range requiredEnvVars {
		val := os.Getenv(name)
		if val == "" {
			return &MissingEnvVarError{Name: name}
		}
		if name == "ORG_CREATION_ENABLED" {
			if _, err := strconv.ParseBool(val); err != nil {
				return &MissingEnvVarError{Name: name, Reason: "unparseable_bool"}
			}
		}
	}
	return nil
}

// readDatabaseURL returns DATABASE_URL or a MissingEnvVarError when unset/empty.
// DATABASE_URL is consumed by pgxpool.New rather than the AR8 env-var contract,
// but the boot-validation shape ("missing_env_var=<NAME>" + non-zero exit) is
// uniform with validateEnv.
func readDatabaseURL() (string, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return "", &MissingEnvVarError{Name: "DATABASE_URL"}
	}
	return dsn, nil
}

// missingEnvVar reports whether err is a MissingEnvVarError for the given name.
func missingEnvVar(err error, name string) bool {
	var m *MissingEnvVarError
	if !errors.As(err, &m) {
		return false
	}
	return m.Name == name
}
