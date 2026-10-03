CREATE TABLE IF NOT EXISTS otp_codes (
    id           UUID PRIMARY KEY,
    phone_number VARCHAR(15) NOT NULL,
    purpose      TEXT        NOT NULL,
    code_hash    TEXT        NOT NULL,
    ip_address   INET        NOT NULL,
    attempts     SMALLINT    NOT NULL DEFAULT 0,
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT otp_codes_purpose_chk CHECK (purpose IN ('sign_up', 'update_user'))
);

-- Rate limits: the latest code for a phone (and purpose), and codes sent from one IP.
CREATE INDEX IF NOT EXISTS otp_codes_phone_created_idx ON otp_codes (phone_number, created_at DESC);
CREATE INDEX IF NOT EXISTS otp_codes_ip_created_idx ON otp_codes (ip_address, created_at DESC);
