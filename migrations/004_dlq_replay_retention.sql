alter table processed_events
    add column if not exists source_topic text;

alter table processed_events
    add column if not exists source_partition integer;

alter table processed_events
    add column if not exists source_offset bigint;

alter table processed_events
    add column if not exists last_seen_at timestamptz;

update processed_events
   set last_seen_at = processed_at
 where last_seen_at is null;

alter table processed_events
    alter column last_seen_at set default now();

alter table processed_events
    alter column last_seen_at set not null;

create index if not exists ix_processed_events_retention
    on processed_events (last_seen_at, consumer_name, event_id);

create table if not exists dead_letter_messages (
    id text primary key check (id ~ '^[A-Za-z0-9._:-]{1,128}$'),
    consumer_name text not null check (consumer_name ~ '^[A-Za-z0-9._:-]{1,128}$'),
    event_id text check (event_id is null or event_id ~ '^[A-Za-z0-9._:-]{1,128}$'),
    source_topic text not null check (source_topic ~ '^[A-Za-z0-9._-]{1,249}$'),
    source_partition integer not null check (source_partition >= 0),
    source_offset bigint not null check (source_offset >= 0),
    message_key bytea not null,
    headers jsonb not null,
    payload bytea not null,
    payload_sha256 char(64) not null check (payload_sha256 ~ '^[0-9a-f]{64}$'),
    error_class text not null check (error_class in ('decode_error', 'validation_error', 'handler_error')),
    error_message text not null check (char_length(error_message) between 1 and 512),
    state text not null default 'OPEN' check (state in ('OPEN', 'REPUBLISHED', 'DISCARDED')),
    first_failed_at timestamptz not null,
    last_failed_at timestamptz not null,
    kafka_published_at timestamptz,
    replay_count integer not null default 0 check (replay_count >= 0),
    version integer not null default 1 check (version > 0),
    unique (consumer_name, source_topic, source_partition, source_offset)
);

create index if not exists ix_dead_letter_messages_state_failed
    on dead_letter_messages (state, first_failed_at, id);

create index if not exists ix_dead_letter_messages_event_id
    on dead_letter_messages (event_id)
    where event_id is not null;

create table if not exists dead_letter_replay_attempts (
    id text primary key,
    dead_letter_id text not null references dead_letter_messages(id) on delete restrict,
    reason text not null check (char_length(reason) between 1 and 256),
    result text not null default 'PENDING' check (result in ('PENDING', 'PUBLISHED', 'FAILED')),
    error_message text check (error_message is null or char_length(error_message) <= 512),
    created_at timestamptz not null default now(),
    completed_at timestamptz
);

create index if not exists ix_dead_letter_replay_attempts_message_created
    on dead_letter_replay_attempts (dead_letter_id, created_at desc);
