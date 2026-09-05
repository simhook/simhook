-- +goose Up
-- The phone pulls its messages from the API when a push wakes it, so each
-- message records when the phone is expected to have sent it: the wave's
-- release time plus its place in the wave times the phone's pacing. The
-- stale sweep waits for that moment before calling a report overdue.
alter table messages add column expected_send_at timestamptz;

-- +goose Down
alter table messages drop column expected_send_at;
