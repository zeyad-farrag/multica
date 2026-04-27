package main

import (
	"errors"
	"testing"
)

// completeEnv returns a fresh map of all required env vars set to placeholder
// values that pass validation. Callers may delete or override entries to
// exercise specific failure modes.
func completeEnv() map[string]string {
	return map[string]string{
		"DEFAULT_ORG_SLUG":         "acme",
		"DEFAULT_ORG_NAME":         "Acme Inc",
		"ORG_CREATION_ENABLED":     "false",
		"TEAM_APP_SHARED_SECRET":   "test-shared-secret",
		"TEAM_APP_URL":             "http://localhost:8080",
		"MULTICA_BASE_URL":         "http://localhost:8081",
		"MULTICA_WS_URL":           "ws://localhost:8081/ws",
		"TEAM_APP_SYSTEM_USER_PAT": "mpat_test_pat",
		"TEAM_APP_SYSTEM_USER_ID":  "00000000-0000-0000-0000-000000000001",
	}
}

// applyEnv sets each entry in m via t.Setenv. t.Setenv unsets the variable
// at test cleanup, restoring the prior value.
func applyEnv(t *testing.T, m map[string]string) {
	t.Helper()
	for _, name := range requiredEnvVars {
		if v, ok := m[name]; ok {
			t.Setenv(name, v)
		} else {
			t.Setenv(name, "")
		}
	}
}

func TestValidateEnv_AllSet_ReturnsNil(t *testing.T) {
	applyEnv(t, completeEnv())
	if err := validateEnv(); err != nil {
		t.Fatalf("validateEnv() with full env returned %v; want nil", err)
	}
}

func TestValidateEnv_MissingVar_ReturnsExpectedError(t *testing.T) {
	for _, name := range requiredEnvVars {
		t.Run(name, func(t *testing.T) {
			env := completeEnv()
			delete(env, name)
			applyEnv(t, env)

			err := validateEnv()
			if err == nil {
				t.Fatalf("validateEnv() with %s missing returned nil; want MissingEnvVarError", name)
			}
			var miss *MissingEnvVarError
			if !errors.As(err, &miss) {
				t.Fatalf("validateEnv() returned %T (%v); want *MissingEnvVarError", err, err)
			}
			if miss.Name != name {
				t.Fatalf("validateEnv() reported missing %q; want %q", miss.Name, name)
			}
			if !missingEnvVar(err, name) {
				t.Fatalf("missingEnvVar(err, %q) = false; want true", name)
			}
		})
	}
}

func TestValidateEnv_EmptyVar_TreatedAsMissing(t *testing.T) {
	env := completeEnv()
	env["DEFAULT_ORG_SLUG"] = ""
	applyEnv(t, env)

	err := validateEnv()
	var miss *MissingEnvVarError
	if !errors.As(err, &miss) {
		t.Fatalf("validateEnv() with empty DEFAULT_ORG_SLUG returned %v; want *MissingEnvVarError", err)
	}
	if miss.Name != "DEFAULT_ORG_SLUG" {
		t.Fatalf("validateEnv() reported missing %q; want DEFAULT_ORG_SLUG", miss.Name)
	}
}

func TestValidateEnv_OrgCreationEnabled_UnparseableBool_FailsFast(t *testing.T) {
	cases := []string{"yes", "no", "1.0", "enabled", "TRUEISH"}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			env := completeEnv()
			env["ORG_CREATION_ENABLED"] = v
			applyEnv(t, env)

			err := validateEnv()
			var miss *MissingEnvVarError
			if !errors.As(err, &miss) {
				t.Fatalf("validateEnv() with ORG_CREATION_ENABLED=%q returned %v; want *MissingEnvVarError", v, err)
			}
			if miss.Name != "ORG_CREATION_ENABLED" {
				t.Fatalf("validateEnv() reported missing %q; want ORG_CREATION_ENABLED", miss.Name)
			}
			if miss.Reason != "unparseable_bool" {
				t.Fatalf("validateEnv() reason %q; want unparseable_bool", miss.Reason)
			}
		})
	}
}

func TestValidateEnv_OrgCreationEnabled_AcceptsCaseInsensitiveBool(t *testing.T) {
	cases := []string{"true", "TRUE", "True", "false", "FALSE", "False", "0", "1", "t", "f"}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			env := completeEnv()
			env["ORG_CREATION_ENABLED"] = v
			applyEnv(t, env)

			if err := validateEnv(); err != nil {
				t.Fatalf("validateEnv() with ORG_CREATION_ENABLED=%q returned %v; want nil", v, err)
			}
		})
	}
}
