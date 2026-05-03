_🧹 Nitpick_ | _🔵 Trivial_ | _💤 Low value_

**Arg parsing logic is correct but fragile.**

The bounds check at line 96 (`i+2 <= len(os.Args[1:])`) is correct for accessing `os.Args[i+2]`. However, mixing slice iteration indices with original array indices is error-prone.

Consider simplifying:

<details>
<summary>♻️ Optional refactor for clarity</summary>

```diff
 func shouldRunEstimateMinutesWithoutDB() bool {
-	for i, arg := range os.Args[1:] {
-		if strings.HasPrefix(arg, "-test.run=") && strings.Contains(arg, "TestUpdateIssueEstimateMinutes_RoundTrip") {
-			return true
-		}
-		if arg == "-test.run" && i+2 <= len(os.Args[1:]) && strings.Contains(os.Args[i+2], "TestUpdateIssueEstimateMinutes_RoundTrip") {
-			return true
-		}
-	}
-	return false
+	for i := 1; i < len(os.Args); i++ {
+		if strings.HasPrefix(os.Args[i], "-test.run=") && strings.Contains(os.Args[i], "TestUpdateIssueEstimateMinutes_RoundTrip") {
+			return true
+		}
+		if os.Args[i] == "-test.run" && i+1 < len(os.Args) && strings.Contains(os.Args[i+1], "TestUpdateIssueEstimateMinutes_RoundTrip") {
+			return true
+		}
+	}
+	return false
 }
```

</details>

<!-- suggestion_start -->

<details>
<summary>📝 Committable suggestion</summary>

> ‼️ **IMPORTANT**
> Carefully review the code before committing. Ensure that it accurately replaces the highlighted code, contains no missing lines, and has no issues with indentation. Thoroughly test & benchmark the code to ensure it meets the requirements.

```suggestion
func shouldRunEstimateMinutesWithoutDB() bool {
	for i := 1; i < len(os.Args); i++ {
		if strings.HasPrefix(os.Args[i], "-test.run=") && strings.Contains(os.Args[i], "TestUpdateIssueEstimateMinutes_RoundTrip") {
			return true
		}
		if os.Args[i] == "-test.run" && i+1 < len(os.Args) && strings.Contains(os.Args[i+1], "TestUpdateIssueEstimateMinutes_RoundTrip") {
			return true
		}
	}
	return false
}
```

</details>

<!-- suggestion_end -->

<details>
<summary>🤖 Prompt for AI Agents</summary>

```
Verify each finding against the current code and only fix it if needed.

In `@server/internal/handler/handler_test.go` around lines 91 - 101, The loop in
shouldRunEstimateMinutesWithoutDB mixes indices of os.Args[1:] with os.Args
which is fragile; change the loop to iterate over the original os.Args using an
index starting at 1 (e.g. for i := 1; i < len(os.Args); i++) and use i+1 <
len(os.Args) for the second-case bounds check, then check
strings.HasPrefix(os.Args[i], "-test.run=") / strings.Contains(...) and the
separate "-test.run" branch using os.Args[i+1]; this keeps index arithmetic
straightforward and avoids mixing slice vs original indices.
```

</details>

<!-- fingerprinting:phantom:medusa:ocelot:24d8ef53-d4c4-411f-b85a-0662e84ba688 -->

<!-- d98c2f50 -->

<!-- This is an auto-generated comment by CodeRabbit -->
