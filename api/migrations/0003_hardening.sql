-- +goose Up
-- Wrong one-time codes are counted; after too many the code is burned and a
-- new one has to be requested, so a six-digit code cannot be walked.
alter table user_tokens add column attempts int not null default 0;

-- The product default for check-ins is 20 minutes (the config default handed
-- to newly paired phones); the column said 30.
alter table devices alter column heartbeat_interval_minutes set default 20;

-- +goose Down
alter table devices alter column heartbeat_interval_minutes set default 30;
alter table user_tokens drop column attempts;
