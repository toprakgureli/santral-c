-- +goose Up
-- Stores each agent's SIP password, AES-GCM encrypted at rest, so the
-- provisioning service can rebuild the PJSIP config and hand the softphone
-- its credentials.
ALTER TABLE users ADD COLUMN sip_secret varchar(255);

-- +goose Down
ALTER TABLE users DROP COLUMN sip_secret;
