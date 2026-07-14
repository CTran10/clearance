alter table transactions
    add column if not exists kind text not null default 'PAYMENT';

alter table transactions
    add column if not exists funding_source text;

alter table transactions
    add column if not exists external_reference text;

alter table transactions
    alter column merchant_id drop not null;

do $$
begin
    if not exists (
        select 1 from pg_constraint where conname = 'transactions_kind_check'
    ) then
        alter table transactions add constraint transactions_kind_check
            check (kind in ('PAYMENT', 'DEPOSIT'));
    end if;
    if not exists (
        select 1 from pg_constraint where conname = 'transactions_kind_fields_check'
    ) then
        alter table transactions add constraint transactions_kind_fields_check
            check (
                (kind = 'PAYMENT' and merchant_id is not null and funding_source is null and external_reference is null)
                or
                (kind = 'DEPOSIT' and merchant_id is null and funding_source is not null and external_reference is not null)
            );
    end if;
    if not exists (
        select 1 from pg_constraint where conname = 'transactions_external_account_check'
    ) then
        alter table transactions add constraint transactions_external_account_check
            check (account_id not in ('clearing', 'external-settlement'));
    end if;
end
$$;

create unique index if not exists ux_transactions_deposit_source_reference
    on transactions (funding_source, external_reference, currency)
    where kind = 'DEPOSIT';

create index if not exists ix_transactions_account_created_id
    on transactions (account_id, created_at desc, id desc);

create index if not exists ix_transactions_account_status_kind_created_id
    on transactions (account_id, status, kind, created_at desc, id desc);

create table if not exists deposit_idempotency_keys (
    key text primary key check (key ~ '^[A-Za-z0-9._:-]{1,128}$'),
    request_hash char(64) not null check (request_hash ~ '^[0-9a-f]{64}$'),
    transaction_id text not null references transactions(id) on delete restrict,
    response_json jsonb not null,
    created_at timestamptz not null default now()
);

create index if not exists ix_deposit_idempotency_transaction_id
    on deposit_idempotency_keys (transaction_id);

create table if not exists operator_actions (
    id text primary key,
    action_type text not null check (action_type in ('DEPOSIT', 'DLQ_REPLAY', 'DLQ_DISCARD', 'OUTBOX_REQUEUE', 'PROCESSED_PRUNE')),
    target_id text not null,
    reason text not null check (char_length(reason) between 1 and 256),
    created_at timestamptz not null default now()
);

alter table outbox_events
    drop constraint if exists outbox_events_event_type_check;

alter table outbox_events
    add constraint outbox_events_event_type_check check (
        event_type in (
            'TransactionCreated',
            'RiskEvaluated',
            'TransactionAuthorized',
            'TransactionFailed',
            'FundsDeposited'
        )
    );
