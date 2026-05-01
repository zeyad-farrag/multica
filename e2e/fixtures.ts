/**
 * TestApiClient — lightweight API helper for E2E test data setup/teardown.
 *
 * Uses raw fetch so E2E tests have zero build-time coupling to the web app.
 */

import "./env";
import pg from "pg";

// `||` (not `??`) so an empty `NEXT_PUBLIC_API_URL=` in .env still falls
// back to localhost. dotenv sets unset-vs-empty both as "" — treating them
// the same matches user intent.
const API_BASE = process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`;
const DEV_VERIFICATION_CODE = process.env.MULTICA_E2E_VERIFICATION_CODE || "888888";
const DATABASE_URL = process.env.DATABASE_URL ?? "postgres://multica:multica@localhost:5432/multica?sslmode=disable";

function loginEmailCandidates(email: string): string[] {
  const normalized = email.trim().toLowerCase();
  const [localPart, domainPart] = normalized.split("@");

  if (!localPart || !domainPart) {
    return [normalized];
  }

  const suffix = `${Date.now()}-${Math.random().toString(16).slice(2, 10)}`;
  return [normalized, `${localPart}+e2e-${suffix}@${domainPart}`];
}

async function readVerificationCode(email: string): Promise<string | null> {
  const client = new pg.Client(DATABASE_URL);
  try {
    await client.connect();
    const result = await client.query(
      "SELECT code FROM verification_code WHERE email = $1 AND used = FALSE AND expires_at > now() ORDER BY created_at DESC LIMIT 1",
      [email],
    );
    return result.rows[0]?.code ?? null;
  } catch {
    return null;
  } finally {
    await client.end().catch(() => undefined);
  }
}

interface TestWorkspace {
  id: string;
  name: string;
  slug: string;
}

export class TestApiClient {
  private token: string | null = null;
  private workspaceSlug: string | null = null;
  private workspaceId: string | null = null;
  private createdIssueIds: string[] = [];

  async login(email: string, name: string) {
    let loginEmail = email.trim().toLowerCase();
    let sendCodeError: Error | null = null;

    for (const candidate of loginEmailCandidates(email)) {
      const sendRes = await fetch(`${API_BASE}/auth/send-code`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: candidate }),
      });

      if (sendRes.ok) {
        loginEmail = candidate;
        sendCodeError = null;
        break;
      }

      if (sendRes.status === 429) {
        sendCodeError = new Error(`send-code rate limited for ${candidate}`);
        continue;
      }

      const body = await sendRes.text();
      throw new Error(`send-code failed: ${sendRes.status} ${body}`);
    }

    if (sendCodeError) {
      throw sendCodeError;
    }

    // Prefer the documented non-production 888888 master code, but keep the
    // database read fallback for local servers that run with APP_ENV=production.
    let verifyRes = await fetch(`${API_BASE}/auth/verify-code`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: loginEmail, code: DEV_VERIFICATION_CODE }),
    });
    if (!verifyRes.ok) {
      const fallbackCode = await readVerificationCode(loginEmail);
      if (fallbackCode) {
        verifyRes = await fetch(`${API_BASE}/auth/verify-code`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ email: loginEmail, code: fallbackCode }),
        });
      }
    }
    if (!verifyRes.ok) {
      const body = await verifyRes.text();
      throw new Error(`verify-code failed: ${verifyRes.status} ${body}`);
    }
    const data = await verifyRes.json();

    this.token = data.token;

    // Update user name if needed
    if (name && data.user?.name !== name) {
      await this.authedFetch("/api/me", {
        method: "PATCH",
        body: JSON.stringify({ name }),
      });
    }

    return data;
  }

  async getWorkspaces(): Promise<TestWorkspace[]> {
    const res = await this.authedFetch("/api/workspaces");
    return res.json();
  }

  setWorkspaceId(id: string) {
    this.workspaceId = id;
  }

  setWorkspaceSlug(slug: string) {
    this.workspaceSlug = slug;
  }

  async ensureWorkspace(name = "E2E Workspace", slug = "e2e-workspace") {
    const workspaces = await this.getWorkspaces();
    const workspace = workspaces.find((item) => item.slug === slug) ?? workspaces[0];
    if (workspace) {
      this.workspaceId = workspace.id;
      this.workspaceSlug = workspace.slug;
      return workspace;
    }

    const res = await this.authedFetch("/api/workspaces", {
      method: "POST",
      body: JSON.stringify({ name, slug }),
    });
    if (res.ok) {
      const created = (await res.json()) as TestWorkspace;
      this.workspaceId = created.id;
      return created;
    }

    const refreshed = await this.getWorkspaces();
    const created = refreshed.find((item) => item.slug === slug) ?? refreshed[0];
    if (created) {
      this.workspaceId = created.id;
      return created;
    }

    throw new Error(`Failed to ensure workspace ${slug}: ${res.status} ${res.statusText}`);
  }

  async createIssue(title: string, opts?: Record<string, unknown>) {
    const res = await this.authedFetch("/api/issues", {
      method: "POST",
      body: JSON.stringify({ title, ...opts }),
    });
    const issue = await res.json();
    this.createdIssueIds.push(issue.id);
    return issue;
  }

  async deleteIssue(id: string) {
    await this.authedFetch(`/api/issues/${id}`, { method: "DELETE" });
  }

  /** Clean up all issues created during this test. */
  async cleanup() {
    for (const id of this.createdIssueIds) {
      try {
        await this.deleteIssue(id);
      } catch {
        /* ignore — may already be deleted */
      }
    }
    this.createdIssueIds = [];
  }

  getToken() {
    return this.token;
  }

  private async authedFetch(path: string, init?: RequestInit) {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      ...((init?.headers as Record<string, string>) ?? {}),
    };
    if (this.token) headers["Authorization"] = `Bearer ${this.token}`;
    if (this.workspaceSlug) headers["X-Workspace-Slug"] = this.workspaceSlug;
    else if (this.workspaceId) headers["X-Workspace-ID"] = this.workspaceId;
    return fetch(`${API_BASE}${path}`, { ...init, headers });
  }
}
