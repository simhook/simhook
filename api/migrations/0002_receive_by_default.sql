-- +goose Up
-- New phones forward incoming SMS from the start. The permission is granted
-- during setup, and an off switch nobody knew about was the surest way to
-- lose messages.
alter table devices alter column receive_enabled set default true;

-- +goose Down
alter table devices alter column receive_enabled set default false;
