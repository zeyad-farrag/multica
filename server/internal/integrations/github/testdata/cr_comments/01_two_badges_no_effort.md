_⚠️ Potential issue_ | _🟠 Major_

<details>
<summary>🧩 Analysis chain</summary>

🏁 Script executed:

```shell
#!/bin/bash
set -euo pipefail
awk '
/^FROM node:22-alpine AS runner/{in_runner=1; next}
(/^FROM / && in_runner){exit}
in_runner && /^USER[[:space:]]+/ {found=1}
END {
  if (found) {
    print "PASS: runner stage declares USER"
    exit 0
  }
  print "FAIL: runner stage has no USER declaration"
  exit 1
}' frontend/Dockerfile
```

Repository: zeyad-farrag/TimeTrack

Length of output: 109

---

🏁 Script executed:

```shell
cat -n frontend/Dockerfile
```

Repository: zeyad-farrag/TimeTrack

Length of output: 802

---

🌐 Web query:

`node:22-alpine default user root or node permissions`

💡 Result:

The node:22-alpine Docker image defaults to running as the root user. It creates a non-root 'node' user (UID 1000, GID 1000) but does not set a USER instruction in the Dockerfile, so the default user is root. The entrypoint (docker-entrypoint.sh) does not switch users. To run as the non-root 'node' user, use docker run -u node or add USER node in a derived Dockerfile. This design provides flexibility for building child images that need root privileges (e.g., installing packages), while allowing runtime as non-root.

Citations:

- 1: https://github.com/nodejs/docker-node/issues/1169
- 2: https://github.com/nodejs/docker-node/blob/bf78d7603fbea92cd3652edb3b2edadd6f5a3fe8/22/alpine3.22/Dockerfile
- 3: https://hub.docker.com/layers/library/node/22.22.0-alpine3.22/images/sha256-4b21c87490019d3aaf5f1feeb7069a74925525476d23c47bf8df06a8fea774c8
- 4: https://github.com/nodejs/docker-node
- 5: https://github.com/nodejs/docker-node/blob/main/docker-entrypoint.sh

---



</details>

**Run the frontend runtime stage as a non-root user.**

The runner stage has no explicit `USER`, so the container process runs with elevated privileges. The `node:22-alpine` base image creates a non-root `node` user but defaults to root; add `USER node` and set proper ownership on copied artifacts.

<details>
<summary>Proposed hardening patch</summary>

```diff
 FROM node:22-alpine AS runner

 WORKDIR /app
 ENV NODE_ENV=production
 ENV HOSTNAME=0.0.0.0
 ENV PORT=3000
-COPY --from=builder /app/.next/standalone ./
-COPY --from=builder /app/.next/static ./.next/static
-COPY --from=builder /app/public ./public
+COPY --from=builder --chown=node:node /app/.next/standalone ./
+COPY --from=builder --chown=node:node /app/.next/static ./.next/static
+COPY --from=builder --chown=node:node /app/public ./public
 EXPOSE 3000
+USER node
 CMD ["node", "server.js"]
```
</details>

<details>
<summary>🧰 Tools</summary>

<details>
<summary>🪛 Checkov (3.2.525)</summary>

[low] 1-26: Ensure that HEALTHCHECK instructions have been added to container images

(CKV_DOCKER_2)

---

[low] 1-26: Ensure that a user for the container has been created

(CKV_DOCKER_3)

</details>

</details>

<details>
<summary>🤖 Prompt for AI Agents</summary>

```
Verify each finding against the current code and only fix it if needed.

In `@frontend/Dockerfile` around lines 16 - 26, The runner stage runs as root by
default; change it to run as the non-root node user and ensure copied files are
owned by that user. Update the Dockerfile runner stage (the lines with FROM
node:22-alpine AS runner, WORKDIR /app, and the three COPY statements) to either
use COPY --chown=node:node for the artifacts copied from builder or run a chown
-R node:node /app after the COPYs, then add USER node before EXPOSE/CMD so the
container process (CMD ["node","server.js"]) runs unprivileged. Ensure ownership
is set for /app, ./.next/static and ./public.
```

</details>

<!-- fingerprinting:phantom:poseidon:hawk -->

<!-- This is an auto-generated comment by CodeRabbit -->
