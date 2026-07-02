create extension if not exists pgcrypto;
create extension if not exists vector;

create schema if not exists identity;
create schema if not exists document;
create schema if not exists ingestion;
create schema if not exists asset;
create schema if not exists graph;
create schema if not exists rag;
create schema if not exists rca;
create schema if not exists compliance;
create schema if not exists report;
create schema if not exists audit;
create schema if not exists notification;
create schema if not exists ai;

create table if not exists identity.organizations (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  industry text,
  created_at timestamptz not null default now()
);

create table if not exists identity.plants (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null references identity.organizations(id),
  name text not null,
  location text,
  created_at timestamptz not null default now()
);

create table if not exists identity.users (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null references identity.organizations(id),
  name text not null,
  email text not null unique,
  email_verified boolean not null default false,
  image text,
  mobile_no text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists identity.sessions (
  id text primary key,
  expires_at timestamptz not null,
  token text not null unique,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  ip_address text,
  user_agent text,
  user_id uuid not null references identity.users(id) on delete cascade
);

create table if not exists identity.accounts (
  id text primary key,
  account_id text not null,
  provider_id text not null,
  user_id uuid not null references identity.users(id) on delete cascade,
  access_token text,
  refresh_token text,
  id_token text,
  expires_at timestamptz,
  password text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists identity.verifications (
  id text primary key,
  identifier text not null,
  value text not null,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists identity.roles (
  id uuid primary key default gen_random_uuid(),
  name text not null unique,
  description text
);

create table if not exists identity.memberships (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null references identity.organizations(id),
  plant_id uuid references identity.plants(id),
  user_id uuid not null references identity.users(id),
  role_id uuid not null references identity.roles(id),
  created_at timestamptz not null default now()
);

create table if not exists document.documents (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  plant_id uuid not null,
  title text not null,
  document_type text,
  current_version_id uuid,
  status text not null default 'UPLOADED',
  uploaded_by uuid,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists document.document_versions (
  id uuid primary key default gen_random_uuid(),
  document_id uuid not null references document.documents(id),
  version_label text not null,
  file_url text not null,
  file_type text not null,
  file_sha256 text,
  ocr_confidence numeric,
  classification_confidence numeric,
  created_at timestamptz not null default now()
);

alter table document.documents
  add constraint documents_current_version_fk
  foreign key (current_version_id)
  references document.document_versions(id);

create table if not exists document.upload_sessions (
  id uuid primary key default gen_random_uuid(),
  document_id uuid references document.documents(id),
  organization_id uuid not null,
  plant_id uuid not null,
  object_key text,
  status text not null default 'CREATED',
  created_by uuid,
  created_at timestamptz not null default now(),
  completed_at timestamptz
);

create table if not exists ingestion.processing_jobs (
  id uuid primary key default gen_random_uuid(),
  document_id uuid not null,
  document_version_id uuid not null,
  status text not null default 'QUEUED',
  error_message text,
  attempts integer not null default 0,
  started_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz not null default now()
);

create table if not exists ingestion.document_pages (
  id uuid primary key default gen_random_uuid(),
  document_id uuid not null,
  document_version_id uuid not null,
  page_no integer not null,
  raw_text text,
  markdown_text text,
  ocr_confidence numeric,
  metadata_json jsonb not null default '{}',
  created_at timestamptz not null default now()
);

create table if not exists ingestion.document_chunks (
  id uuid primary key default gen_random_uuid(),
  document_id uuid not null,
  document_version_id uuid not null,
  page_no integer,
  chunk_index integer not null,
  chunk_text text not null,
  embedding vector(768),
  token_count integer,
  metadata_json jsonb not null default '{}',
  created_at timestamptz not null default now()
);

create index if not exists document_chunks_embedding_idx
  on ingestion.document_chunks
  using ivfflat (embedding vector_cosine_ops)
  with (lists = 100);

create index if not exists document_chunks_metadata_idx
  on ingestion.document_chunks
  using gin (metadata_json);

create table if not exists asset.assets (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  plant_id uuid not null,
  asset_tag text not null,
  asset_name text,
  asset_type text,
  location text,
  criticality text,
  risk_score numeric,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (plant_id, asset_tag)
);

create table if not exists asset.asset_aliases (
  id uuid primary key default gen_random_uuid(),
  asset_id uuid not null references asset.assets(id),
  alias text not null,
  source_document_id uuid,
  confidence numeric,
  created_at timestamptz not null default now()
);

create table if not exists graph.entities (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  plant_id uuid,
  document_id uuid,
  chunk_id uuid,
  entity_type text not null,
  entity_value text not null,
  normalized_value text,
  confidence numeric,
  page_no integer,
  created_at timestamptz not null default now()
);

create table if not exists graph.relationships (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  source_entity_id uuid not null references graph.entities(id),
  target_entity_id uuid not null references graph.entities(id),
  relationship_type text not null,
  confidence numeric,
  evidence_document_id uuid,
  evidence_chunk_id uuid,
  created_at timestamptz not null default now()
);

create index if not exists graph_entities_lookup_idx
  on graph.entities (organization_id, entity_type, normalized_value);

create table if not exists rag.queries (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  plant_id uuid,
  user_id uuid,
  query_text text not null,
  answer_text text,
  confidence numeric,
  retrieval_metadata jsonb not null default '{}',
  created_at timestamptz not null default now()
);

create table if not exists rag.citations (
  id uuid primary key default gen_random_uuid(),
  query_id uuid not null references rag.queries(id),
  document_id uuid not null,
  chunk_id uuid,
  page_no integer,
  quoted_text text,
  score numeric,
  created_at timestamptz not null default now()
);

create table if not exists rag.feedback (
  id uuid primary key default gen_random_uuid(),
  query_id uuid not null references rag.queries(id),
  user_id uuid,
  rating integer,
  comment text,
  created_at timestamptz not null default now()
);

create table if not exists rca.reports (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  plant_id uuid,
  asset_id uuid,
  failure_summary text not null,
  timeline jsonb not null default '[]',
  probable_causes jsonb not null default '[]',
  recommendations jsonb not null default '[]',
  missing_data jsonb not null default '[]',
  confidence numeric,
  created_by uuid,
  created_at timestamptz not null default now()
);

create table if not exists compliance.requirements (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  plant_id uuid,
  title text not null,
  requirement_type text,
  standard_name text,
  frequency text,
  metadata_json jsonb not null default '{}',
  created_at timestamptz not null default now()
);

create table if not exists compliance.gaps (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  plant_id uuid,
  asset_id uuid,
  requirement_id uuid references compliance.requirements(id),
  gap_type text not null,
  description text not null,
  severity text,
  evidence_document_id uuid,
  status text not null default 'OPEN',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists report.jobs (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  plant_id uuid,
  requested_by uuid,
  report_type text not null,
  status text not null default 'QUEUED',
  output_file_url text,
  parameters_json jsonb not null default '{}',
  error_message text,
  created_at timestamptz not null default now(),
  completed_at timestamptz
);

create table if not exists audit.events (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid,
  plant_id uuid,
  actor_user_id uuid,
  event_type text not null,
  resource_type text,
  resource_id text,
  correlation_id text,
  metadata_json jsonb not null default '{}',
  occurred_at timestamptz not null default now()
);

create index if not exists audit_events_lookup_idx
  on audit.events (organization_id, event_type, occurred_at desc);

create table if not exists notification.notifications (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid not null,
  plant_id uuid,
  recipient_user_id uuid,
  notification_type text not null,
  title text not null,
  body text,
  status text not null default 'PENDING',
  metadata_json jsonb not null default '{}',
  created_at timestamptz not null default now(),
  delivered_at timestamptz
);

create table if not exists ai.model_calls (
  id uuid primary key default gen_random_uuid(),
  organization_id uuid,
  service_name text not null,
  provider text,
  model text,
  prompt_tokens integer,
  completion_tokens integer,
  latency_ms integer,
  status text not null,
  correlation_id text,
  created_at timestamptz not null default now()
);
