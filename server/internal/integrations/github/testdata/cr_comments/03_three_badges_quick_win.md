_⚠️ Potential issue_ | _🟡 Minor_ | _⚡ Quick win_

**Differentiate missing vs invalid env failures in logs.**

Invalid boolean values for `ORG_CREATION_ENABLED` are currently reported as `missing_env_var`, which makes root-cause triage noisier than necessary.

 

<details>
<summary>Suggested patch</summary>

```diff
@@
-	if err := validateEnv(); err != nil {
-		var envErr missingEnvError
-		if errors.As(err, &envErr) {
-			logger.Error("missing_env_var", "var", envErr.Name)
-		} else {
-			logger.Error("missing_env_var", "error", err)
-		}
+	if err := validateEnv(); err != nil {
+		var missingErr missingEnvError
+		var invalidErr invalidEnvError
+		switch {
+		case errors.As(err, &missingErr):
+			logger.Error("missing_env_var", "var", missingErr.Name)
+		case errors.As(err, &invalidErr):
+			logger.Error("invalid_env_var", "var", invalidErr.Name, "value", invalidErr.Value)
+		default:
+			logger.Error("env_validation_failed", "error", err)
+		}
 		os.Exit(1)
 	}
@@
 type missingEnvError struct {
 	Name string
 }
@@
 func (e missingEnvError) Error() string {
 	return fmt.Sprintf("missing_env_var=%s", e.Name)
 }
+
+type invalidEnvError struct {
+	Name  string
+	Value string
+}
+
+func (e invalidEnvError) Error() string {
+	return fmt.Sprintf("invalid_env_var=%s", e.Name)
+}
@@
 		if name == "ORG_CREATION_ENABLED" {
 			if _, err := strconv.ParseBool(value); err != nil {
-				return missingEnvError{Name: name}
+				return invalidEnvError{Name: name, Value: value}
 			}
 		}
```
</details>


Also applies to: 99-102

<!-- fingerprinting:phantom:poseidon:hawk -->

<!-- This is an auto-generated comment by CodeRabbit -->
