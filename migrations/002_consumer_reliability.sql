alter table outbox_events
    add column if not exists partition_key text;

update outbox_events as outbox
   set partition_key = transactions.account_id
  from transactions
 where outbox.partition_key is null
   and outbox.aggregate_id = transactions.id;

update outbox_events
   set partition_key = aggregate_id
 where partition_key is null;

alter table outbox_events
    alter column partition_key set not null;

create table if not exists processed_events (
    consumer_name text not null check (consumer_name ~ '^[A-Za-z0-9._:-]{1,128}$'),
    event_id text not null check (event_id ~ '^[A-Za-z0-9._:-]{1,128}$'),
    payload_sha256 char(64) not null check (payload_sha256 ~ '^[0-9a-f]{64}$'),
    processed_at timestamptz not null default now(),
    primary key (consumer_name, event_id)
);
