-- TIM-6 Initial schema: 4 mirror tables + 8 domain tables (v2 keying, pre-org).
--
-- This migration is the pre-org, v2-keyed shape per spec §6.2 / §5.0.4. The 002 migration
-- DROPs and re-CREATEs the domain tables under the v3 shape (organization_id columns,
-- _user_id rename suffix, time_confirm approval-workflow fields, time_confirm_history
-- append-only trigger). Do NOT add organization_id / _user_id rename in this file —
-- those land in 002_org_layer.up.sql.
--
-- Mirror tables carry NO foreign keys to the Multica database. Mirror UUIDs equal Multica
-- UUIDs by convention but live in a separate database (architecture §149-§157, §771-§775).
-- Cross-database FKs are forbidden — the relationships below are documented in comments
-- only.

BEGIN;

----------------------------------------------------------------
-- Mirror tables (4) — workspace-keyed in both 001 and 002.
----------------------------------------------------------------

-- Mirror of Multica.workspace. id matches Multica workspace.id (cross-DB; no FK).
-- work_week is standalone-owned (Enforcement Guideline #5): never written by the WS
-- subscriber, reconciler, or UpsertMirrorWorkspace — only by team-app handlers.
CREATE TABLE mirror_workspace (
    id            UUID PRIMARY KEY,
    slug          TEXT NOT NULL,
    name          TEXT NOT NULL DEFAULT '',
    work_week     JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Mirror of Multica.user. id matches Multica user.id (cross-DB; no FK).
CREATE TABLE mirror_user (
    id            UUID PRIMARY KEY,
    email         TEXT NOT NULL,
    name          TEXT NOT NULL,
    avatar_url    TEXT,
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Mirror of Multica.member. id, workspace_id, user_id all match Multica (cross-DB; no FK).
CREATE TABLE mirror_member (
    id            UUID PRIMARY KEY,
    workspace_id  UUID NOT NULL,
    user_id       UUID NOT NULL,
    role          TEXT NOT NULL CHECK (role IN ('owner','admin','member')),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_mirror_member_workspace ON mirror_member (workspace_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_mirror_member_user      ON mirror_member (user_id)      WHERE deleted_at IS NULL;

-- Mirror of Multica.issue. id, workspace_id, parent_issue_id, assignee_id, creator_id all
-- match Multica (cross-DB; no FK).
CREATE TABLE mirror_issue (
    id                UUID PRIMARY KEY,
    workspace_id      UUID NOT NULL,
    parent_issue_id   UUID,
    status            TEXT NOT NULL,
    assignee_type     TEXT,
    assignee_id       UUID,
    creator_type      TEXT NOT NULL,
    creator_id        UUID NOT NULL,
    estimate_minutes  INT,
    title             TEXT NOT NULL,
    due_date          TIMESTAMPTZ,
    priority          TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ,
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_mirror_issue_workspace ON mirror_issue (workspace_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_mirror_issue_assignee  ON mirror_issue (assignee_type, assignee_id, status)
    WHERE deleted_at IS NULL
      AND status IN ('todo','in_progress','planning','ready_for_dev','fixing','testing');
CREATE INDEX idx_mirror_issue_parent    ON mirror_issue (parent_issue_id)
    WHERE parent_issue_id IS NOT NULL AND deleted_at IS NULL;

----------------------------------------------------------------
-- Domain tables (8) — v2 keying. NO organization_id columns; member_id (NOT user_id);
-- assignee_id / creator_id / confirmed_by / created_by carry no _user_id suffix in 001.
-- The 002 migration drops and re-CREATEs these under the v3 shape.
----------------------------------------------------------------

-- member_schedule: PK on member_id (one schedule per member). v2: no organization_id.
CREATE TABLE member_schedule (
    member_id    UUID NOT NULL,
    timezone     TEXT NOT NULL DEFAULT 'Africa/Cairo',
    work_days    INT[] NOT NULL DEFAULT '{0,1,2,3,4}',
    work_start   TIME NOT NULL DEFAULT '09:00',
    work_end     TIME NOT NULL DEFAULT '18:00',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (member_id),
    CHECK (work_end > work_start),
    CHECK (array_length(work_days, 1) BETWEEN 1 AND 7),
    CHECK (work_days <@ ARRAY[0,1,2,3,4,5,6]::INT[])
);

-- member_leave: keyed on member_id, created_by stores Multica user.id (no _user_id suffix in v2).
CREATE TABLE member_leave (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id    UUID NOT NULL,
    starts_on    DATE NOT NULL,
    ends_on      DATE NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('vacation','sick','public_holiday','other')),
    note         TEXT,
    created_by   UUID NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_on >= starts_on)
);
CREATE INDEX idx_member_leave_range ON member_leave (member_id, starts_on, ends_on);

-- work_item: assignee_id / creator_id (no _user_id suffix in v2). recurrence_parent_id is
-- a self-FK; time_entry.work_item_id and time_confirm_history.time_confirm_id are the
-- other domain-to-domain FKs in 001.
CREATE TABLE work_item (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id          UUID NOT NULL,
    title                 TEXT NOT NULL,
    description           TEXT,
    category              TEXT NOT NULL CHECK (category IN
                            ('meeting','ceremony','review','admin','learning','other')),
    assignee_id           UUID NOT NULL,
    creator_id            UUID NOT NULL,
    status                TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN
                            ('scheduled','in_progress','done','cancelled')),
    scheduled_for         TIMESTAMPTZ NOT NULL,
    duration_minutes      INT NOT NULL CHECK (duration_minutes > 0
                                              AND duration_minutes % 15 = 0),
    recurrence            JSONB,
    recurrence_parent_id  UUID REFERENCES work_item(id) ON DELETE CASCADE,
    instance_date         DATE,
    cancelled_override    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
      (recurrence_parent_id IS NULL AND instance_date IS NULL) OR
      (recurrence_parent_id IS NOT NULL AND instance_date IS NOT NULL
        AND recurrence IS NULL)
    ),
    CHECK (NOT (cancelled_override AND recurrence IS NOT NULL))
);
CREATE INDEX idx_work_item_workspace             ON work_item (workspace_id);
CREATE INDEX idx_work_item_assignee_time         ON work_item (assignee_id, scheduled_for);
CREATE INDEX idx_work_item_recurrence_parent     ON work_item (recurrence_parent_id, instance_date)
    WHERE recurrence_parent_id IS NOT NULL;
CREATE UNIQUE INDEX idx_work_item_override_unique
    ON work_item (recurrence_parent_id, instance_date)
    WHERE recurrence_parent_id IS NOT NULL;

-- time_entry: member_id / confirmed_by (no _user_id suffix in v2). 15-minute granularity
-- enforced via DB CHECK (NFR26, NFR27). XOR check disambiguates kind ↔ issue_id /
-- work_item_id / work_item_title_snapshot.
CREATE TABLE time_entry (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id                UUID NOT NULL,
    member_id                   UUID NOT NULL,
    kind                        TEXT NOT NULL CHECK (kind IN ('issue','work_item','other')),
    issue_id                    UUID,
    work_item_id                UUID REFERENCES work_item(id) ON DELETE SET NULL,
    work_item_title_snapshot    TEXT,
    source                      TEXT NOT NULL CHECK (source IN ('auto','manual')),
    user_edited                 BOOLEAN NOT NULL DEFAULT FALSE,
    started_at                  TIMESTAMPTZ NOT NULL,
    ended_at                    TIMESTAMPTZ NOT NULL,
    minutes                     INT NOT NULL CHECK (minutes > 0 AND minutes % 15 = 0),
    confirmed                   BOOLEAN NOT NULL DEFAULT FALSE,
    confirmed_at                TIMESTAMPTZ,
    confirmed_by                UUID,
    locked_at                   TIMESTAMPTZ,
    notes                       TEXT,
    conflict_group              UUID,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ended_at > started_at),
    CHECK (
      (kind = 'issue'
         AND issue_id IS NOT NULL
         AND work_item_id IS NULL
         AND work_item_title_snapshot IS NULL) OR
      (kind = 'work_item'
         AND issue_id IS NULL
         AND work_item_title_snapshot IS NOT NULL) OR
      (kind = 'other'
         AND issue_id IS NULL
         AND work_item_id IS NULL
         AND work_item_title_snapshot IS NULL)
    ),
    CHECK (
      confirmed = FALSE OR
      (confirmed_at IS NOT NULL AND locked_at IS NOT NULL)
    ),
    CHECK (kind <> 'other' OR source = 'manual')
);
CREATE INDEX idx_time_entry_member_started  ON time_entry (member_id, started_at);
CREATE INDEX idx_time_entry_issue           ON time_entry (issue_id)       WHERE issue_id IS NOT NULL;
CREATE INDEX idx_time_entry_work_item       ON time_entry (work_item_id)   WHERE work_item_id IS NOT NULL;
CREATE INDEX idx_time_entry_unconfirmed     ON time_entry (workspace_id, member_id) WHERE confirmed = FALSE;
CREATE INDEX idx_time_entry_conflict_group  ON time_entry (conflict_group) WHERE conflict_group IS NOT NULL;

-- AC4: auto-fill idempotency partial unique index. Uses member_id (NOT user_id) — the
-- rename to user_id happens in 002.
CREATE UNIQUE INDEX idx_time_entry_auto_issue_key
    ON time_entry (member_id, issue_id, started_at)
    WHERE kind = 'issue'
      AND source = 'auto'
      AND confirmed = FALSE
      AND user_edited = FALSE;

-- time_confirm: v2 shape — no status / employee_comment / manager_comment / manager_user_id
-- / submitted_at / reviewed_at / withdrawn_at. The v3 approval-workflow fields land in 002.
-- Keyed on (workspace_id, member_id, week_start) — NOT (organization_id, user_id, week_start).
CREATE TABLE time_confirm (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id      UUID NOT NULL,
    member_id         UUID NOT NULL,
    week_start        DATE NOT NULL,
    confirmed_at      TIMESTAMPTZ,
    auto_confirmed    BOOLEAN NOT NULL DEFAULT FALSE,
    total_minutes     INT NOT NULL DEFAULT 0,
    no_hours_week     BOOLEAN NOT NULL DEFAULT FALSE,
    reminder_sent_at  TIMESTAMPTZ,
    rollup_sent_at    TIMESTAMPTZ,
    skip_reason       TEXT,
    notes             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, member_id, week_start)
);

-- time_confirm_history: table only in 001. The append-only trigger
-- (time_confirm_history_block_mutations) is added in 002 per spec §6.2.
CREATE TABLE time_confirm_history (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    time_confirm_id  UUID NOT NULL REFERENCES time_confirm(id) ON DELETE CASCADE,
    action           TEXT NOT NULL CHECK (action IN (
                       'submitted','approved','rejected','resubmitted',
                       'withdrawn','reopened','auto_confirmed'
                     )),
    prev_status      TEXT NOT NULL,
    new_status       TEXT NOT NULL,
    by_user_id       UUID,
    comment          TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_time_confirm_history_confirm ON time_confirm_history (time_confirm_id, created_at);

-- workload_anomaly: keyed on member_id in v2 (NOT user_id). FR83 dedupe lives on
-- idx_workload_anomaly_open below.
CREATE TABLE workload_anomaly (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id     UUID NOT NULL,
    member_id        UUID NOT NULL,
    kind             TEXT NOT NULL CHECK (kind IN (
                       'excessive_moves',
                       'suspicious_autofill',
                       'no_activity',
                       'over_allocation',
                       'unconfirmed_streak',
                       'confirm_total_mismatch',
                       'gate_service_unavailable',
                       'mirror_drift_detected',
                       'approval_overdue',
                       'rejection_loop',
                       'workspace_unlinked_with_entries'
                     )),
    severity         TEXT NOT NULL CHECK (severity IN ('info','warning','alert')),
    detected_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    window_start     TIMESTAMPTZ,
    window_end       TIMESTAMPTZ,
    details          JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolved_at      TIMESTAMPTZ,
    resolved_by      UUID,
    resolution_note  TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_workload_anomaly_workspace ON workload_anomaly (workspace_id, detected_at DESC) WHERE resolved_at IS NULL;
CREATE INDEX idx_workload_anomaly_member    ON workload_anomaly (member_id, detected_at DESC);

-- AC5: anomaly dedupe partial unique index. Uses member_id (NOT user_id) — the rename
-- and addition of organization_id happen in 002.
CREATE UNIQUE INDEX idx_workload_anomaly_open
    ON workload_anomaly (member_id, kind, window_start, window_end)
    WHERE resolved_at IS NULL;

-- activity_log: actor_user_id stores Multica user.id directly (NOT mirror_member.id), so
-- it stays as actor_user_id in both 001 and 002 per spec §5.3.7. v2: no organization_id.
CREATE TABLE activity_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    UUID NOT NULL,
    issue_id        UUID,
    actor_type      TEXT NOT NULL CHECK (actor_type IN ('member','system')),
    actor_user_id   UUID NOT NULL,
    action          TEXT NOT NULL,
    details         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_activity_log_workspace ON activity_log (workspace_id, created_at DESC);
CREATE INDEX idx_activity_log_actor     ON activity_log (actor_user_id, created_at DESC);

COMMIT;
