_🧹 Nitpick_ | _🔵 Trivial_ | _⚡ Quick win_

**Exclude unrelated files from the API build context.**

Line 6 (`COPY . .`) sends the entire repository to the Docker daemon, which increases build time and can pull local-only artifacts into build cache layers. Add a root `.dockerignore` to keep the backend image build deterministic and lean.
 

<details>
<summary>Proposed `.dockerignore` starter</summary>

```diff
+# .dockerignore
+.git
+frontend
+**/node_modules
+**/.next
+**/.env.local
+.env
+*.log
```
</details>

<details>
<summary>🤖 Prompt for AI Agents</summary>

```
Verify each finding against the current code and only fix it if needed.

In `@Dockerfile` around lines 4 - 7, The Dockerfile uses a broad COPY . . which
sends the entire repository into the build context and can leak local artifacts
and slow builds; create a root .dockerignore that excludes node_modules, .git,
build artifacts, IDE files, and any non-backend folders so the API image build
context is minimal, then keep the Dockerfile's stanza (COPY go.mod go.sum ./;
RUN go mod download; COPY . .; RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go
build -o /team-app-server ./cmd/server) but replace the broad COPY . . with the
same COPY after adding the .dockerignore to ensure unrelated files are excluded
from the build context.
```

</details>

<!-- fingerprinting:phantom:poseidon:hawk -->

<!-- This is an auto-generated comment by CodeRabbit -->
