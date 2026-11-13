-- The attachment component (docs/design.md, the data): the category catalog's copy, the attachments with their
-- category snapshot, their content versions, the access credentials (digests only) and the holds.
CREATE TABLE category (
    resource_type   text NOT NULL,
    name            text NOT NULL,
    owner_component text NOT NULL,
    field           text NOT NULL,
    max_bytes       bigint NOT NULL,
    allowed_types   text[] NOT NULL,
    sensitivity     text NOT NULL,
    retention_class text NOT NULL,
    version         text NOT NULL,
    PRIMARY KEY (resource_type, name)
);
CREATE TABLE catalog_state (
    single     boolean PRIMARY KEY DEFAULT true CHECK (single),
    revision   text NOT NULL,
    fetched_at timestamptz NOT NULL
);

CREATE TABLE attachment (
    id               uuid PRIMARY KEY,
    owner_component  text NOT NULL,
    resource_type    text NOT NULL,
    record_id        text NOT NULL,
    category         text NOT NULL,
    category_version text NOT NULL,
    -- the category as it was when the attachment was made (AT03): a later change of the category is for new ones
    sensitivity      text NOT NULL CHECK (sensitivity IN ('normal', 'strict')),
    max_bytes        bigint NOT NULL,
    allowed_types    text[] NOT NULL,
    retention_class  text NOT NULL,
    state            text NOT NULL CHECK (state IN ('uploading', 'finalizing', 'pending_scan', 'available', 'rejected',
                         'quarantined', 'missing', 'detached', 'purged')),
    state_before_detach text,
    current_version  bigint NOT NULL DEFAULT 0,
    file_name        text NOT NULL,
    uploaded_by      text NOT NULL,
    detached_at      timestamptz,
    retention_start  date,
    purged_at        timestamptz,
    revision         bigint NOT NULL DEFAULT 1,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX attachment_parent ON attachment (owner_component, resource_type, record_id);
CREATE INDEX attachment_detached ON attachment (detached_at) WHERE state = 'detached';

CREATE TABLE version (
    attachment_id   uuid NOT NULL REFERENCES attachment (id),
    version         bigint NOT NULL,
    state           text NOT NULL CHECK (state IN ('uploading', 'finalizing', 'pending_scan', 'clean', 'rejected',
                        'quarantined', 'superseded')),
    staging_key     text NOT NULL,
    final_key       text NOT NULL,
    declared_size   bigint NOT NULL,
    declared_type   text NOT NULL,
    size            bigint,
    detected_type   text,
    sha256          text,
    unscanned       boolean NOT NULL DEFAULT false,
    scan_result     text,
    reason          text,
    session_expires timestamptz NOT NULL,
    finalizing_at   timestamptz,
    ended_at        timestamptz,
    object_deleted  boolean NOT NULL DEFAULT false,
    PRIMARY KEY (attachment_id, version)
);
CREATE INDEX version_uploading ON version (session_expires) WHERE state = 'uploading';
CREATE INDEX version_finalizing ON version (finalizing_at) WHERE state = 'finalizing';
CREATE INDEX version_dead ON version (ended_at) WHERE state IN ('rejected', 'quarantined') AND NOT object_deleted;

CREATE TABLE credential (
    digest        text PRIMARY KEY,
    operation     text NOT NULL CHECK (operation IN ('confirm', 'download')),
    principal     text NOT NULL,
    attachment_id uuid NOT NULL REFERENCES attachment (id),
    version       bigint NOT NULL,
    expires_at    timestamptz NOT NULL,
    used_at       timestamptz
);
CREATE INDEX credential_expires ON credential (expires_at);

CREATE TABLE hold (
    id              uuid PRIMARY KEY,
    owner_component text NOT NULL,
    resource_type   text NOT NULL,
    record_id       text NOT NULL,
    attachment_id   uuid REFERENCES attachment (id),
    reason          text NOT NULL,
    placed_at       timestamptz NOT NULL DEFAULT now(),
    released_at     timestamptz
);
CREATE INDEX hold_live ON hold (owner_component, resource_type, record_id) WHERE released_at IS NULL;
