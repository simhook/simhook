-- +goose Up
-- Billing through a merchant of record (decision 021). The provider sells
-- one product per paid plan and interval; sandbox and production have
-- different ids, so the table keeps both, and `simhook billing sync` writes
-- it from the plans table.
create table billing_products (
    provider      text not null,
    environment   text not null check (environment in ('sandbox', 'production')),
    plan_id       text not null references plans (id),
    interval      text not null check (interval in ('month', 'year')),
    product_id    text not null,
    price_id      text not null,
    price_cents   int  not null,
    synced_at     timestamptz not null default now(),
    primary key (provider, environment, plan_id, interval)
);

-- Every delivery the provider makes is recorded by its id before it is acted
-- on, so a redelivery does nothing twice.
create table billing_events (
    id            text primary key,
    provider      text not null,
    type          text not null,
    received_at   timestamptz not null default now()
);

-- What the provider last said about a subscription, so an old delivery
-- that arrives after a newer one cannot roll it back; and a change the
-- provider will apply at the next period, so the dashboard can say so.
alter table subscriptions add column provider_updated_at timestamptz;
alter table subscriptions add column pending_plan_id text references plans (id);
alter table subscriptions add column pending_interval text check (pending_interval in ('month', 'year'));
alter table subscriptions add column pending_at timestamptz;
create unique index subscriptions_provider_subscription on subscriptions (provider, provider_subscription_id)
    where provider_subscription_id is not null;

-- +goose Down
drop index if exists subscriptions_provider_subscription;
alter table subscriptions drop column pending_at;
alter table subscriptions drop column pending_interval;
alter table subscriptions drop column pending_plan_id;
alter table subscriptions drop column provider_updated_at;
drop table if exists billing_events;
drop table if exists billing_products;
