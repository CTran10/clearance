create table if not exists transactions (
    id text primary key,
    account_id text not null check (account_id ~ '^[A-Za-z0-9._:-]{1,128}$'),
    merchant_id text not null check (merchant_id ~ '^[A-Za-z0-9._:-]{1,128}$'),
    amount_cents bigint not null check (amount_cents > 0),
    currency char(3) not null check (currency ~ '^[A-Z]{3}$'),
    status text not null check (status in ('PENDING', 'AUTHORIZED', 'FAILED')),
    risk_level text check (risk_level in ('LOW', 'HIGH')),
    risk_reason text,
    correlation_id text not null check (correlation_id ~ '^[A-Za-z0-9._:-]{1,128}$'),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists ix_transactions_account_created_at
    on transactions (account_id, created_at desc);

create index if not exists ix_transactions_status_created_at
    on transactions (status, created_at);

create table if not exists idempotency_keys (
    key text primary key check (key ~ '^[A-Za-z0-9._:-]{1,128}$'),
    request_hash char(64) not null,
    transaction_id text not null references transactions(id) on delete restrict,
    response_json jsonb not null,
    created_at timestamptz not null default now()
);

create index if not exists ix_idempotency_transaction_id
    on idempotency_keys (transaction_id);

create table if not exists outbox_events (
    id text primary key,
    event_type text not null check (
        event_type in (
            'TransactionCreated',
            'RiskEvaluated',
            'TransactionAuthorized',
            'TransactionFailed'
        )
    ),
    aggregate_id text not null,
    correlation_id text not null check (correlation_id ~ '^[A-Za-z0-9._:-]{1,128}$'),
    payload jsonb not null,
    status text not null default 'PENDING' check (
        status in ('PENDING', 'PUBLISHED', 'DEAD_LETTERED')
    ),
    attempts integer not null default 0 check (attempts >= 0),
    last_error text,
    published_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

create index if not exists ix_outbox_pending_created_at
    on outbox_events (status, created_at, id);

create table if not exists ledger_entries (
    id text primary key,
    transaction_id text not null references transactions(id) on delete restrict,
    account_id text not null check (account_id ~ '^[A-Za-z0-9._:-]{1,128}$'),
    amount_cents bigint not null check (amount_cents <> 0),
    currency char(3) not null check (currency ~ '^[A-Z]{3}$'),
    created_at timestamptz not null default now(),
    unique (transaction_id, account_id)
);

create index if not exists ix_ledger_entries_account_created_at
    on ledger_entries (account_id, created_at desc);

create table if not exists audit_logs (
    id bigserial primary key,
    action text not null,
    transaction_id text references transactions(id) on delete restrict,
    correlation_id text not null check (correlation_id ~ '^[A-Za-z0-9._:-]{1,128}$'),
    metadata jsonb not null default '{}'::jsonb,
    created_at timestamptz not null default now()
);

create index if not exists ix_audit_logs_transaction_id
    on audit_logs (transaction_id);

create or replace function prevent_ledger_entry_mutation()
returns trigger language plpgsql as $$
begin
    raise exception 'ledger_entries are immutable';
end;
$$;

drop trigger if exists trg_prevent_ledger_entry_update on ledger_entries;
create trigger trg_prevent_ledger_entry_update
    before update or delete on ledger_entries
    for each row execute function prevent_ledger_entry_mutation();
