_🧹 Nitpick_ | _🔵 Trivial_ | _⚖️ Poor tradeoff_

**Docker helpers duplicated across scripts.**

`resolve_docker_access()` and `run_docker()` are identical to lines 30-45 in `init-worktree-env.sh`. Consider extracting to a shared utility (e.g., `scripts/lib/docker-helpers.sh`) and sourcing it in both scripts to reduce maintenance burden.

<details>
<summary>🤖 Prompt for AI Agents</summary>

```
Verify each finding against the current code and only fix it if needed.

In `@scripts/ensure-postgres.sh` around lines 71 - 89, Create a new shared helper
file (e.g., scripts/lib/docker-helpers.sh) containing the resolve_docker_access
and run_docker functions, move the function definitions out of
scripts/ensure-postgres.sh and init-worktree-env.sh, and then source that helper
from both scripts (e.g., . "$(dirname "$0")/lib/docker-helpers.sh") before they
call those functions; ensure the helper is executable/readable and that any
references to the docker_cmd variable remain consistent with the original
functions.
```

</details>

<!-- fingerprinting:phantom:medusa:ocelot:432e8aa6-ad16-44f9-b1d1-dd0968f0689d -->

<!-- d98c2f50 -->

<!-- This is an auto-generated comment by CodeRabbit -->
