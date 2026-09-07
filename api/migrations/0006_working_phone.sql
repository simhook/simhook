-- +goose Up
-- The stale sweep tells a slow phone from a silent one by when it last
-- reported on any message: a phone that is still reporting is behind, not
-- gone, and its overdue messages are left alone.
alter table devices add column last_report_at timestamptz;

-- A six-digit code is only ever looked up with its account and kind, so two
-- accounts drawing the same code must not break each other on insert.
alter table user_tokens drop constraint user_tokens_token_hash_key;

-- +goose Down
alter table user_tokens add constraint user_tokens_token_hash_key unique (token_hash);
alter table devices drop column last_report_at;
