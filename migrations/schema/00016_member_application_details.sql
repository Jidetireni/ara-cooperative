-- +goose Up
-- Fields from the paper membership application form (the source of truth).
-- They stay nullable so members who joined before this change keep loading;
-- the profile endpoint requires the form's mandatory fields on every save.
ALTER TABLE members
    ADD COLUMN title VARCHAR(30),
    ADD COLUMN middle_name VARCHAR(255),
    ADD COLUMN alternate_phone VARCHAR(20),
    ADD COLUMN place_of_birth VARCHAR(255),
    ADD COLUMN nationality VARCHAR(100),
    ADD COLUMN state_of_origin VARCHAR(100),
    ADD COLUMN home_town VARCHAR(255),
    ADD COLUMN lga VARCHAR(255),
    ADD COLUMN next_of_kin_address TEXT,
    ADD COLUMN workplace_company VARCHAR(255),
    ADD COLUMN workplace_address TEXT,
    ADD COLUMN workplace_since DATE,
    ADD COLUMN workplace_position VARCHAR(255),
    ADD COLUMN workplace_responsibilities TEXT,
    ADD COLUMN monthly_contribution_pledge BIGINT CHECK (monthly_contribution_pledge >= 0),
    ADD COLUMN introduced_by_name VARCHAR(255),
    ADD COLUMN introduced_by_number VARCHAR(50),
    ADD COLUMN declaration_accepted_at TIMESTAMPTZ;

-- Beneficiaries (bye-law 12 nominees): people designated to receive the
-- member's funds in case of any eventuality. The form has four rows.
CREATE TABLE member_beneficiaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL CHECK (position BETWEEN 1 AND 4),
    name VARCHAR(255) NOT NULL CHECK (btrim(name) <> ''),
    relationship VARCHAR(100) NOT NULL CHECK (btrim(relationship) <> ''),
    address TEXT NOT NULL CHECK (btrim(address) <> ''),
    phone VARCHAR(20) NOT NULL CHECK (btrim(phone) <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (member_id, position)
);

-- Passport photograph, stored in Cloudinary. Only the public ID
-- (ara/<env>/members/<member id>/passport) and upload version are kept;
-- clients build the delivery URL from them.
ALTER TABLE members
    ADD COLUMN passport_photo_public_id VARCHAR(255),
    ADD COLUMN passport_photo_version BIGINT,
    ADD CONSTRAINT members_passport_photo CHECK ((passport_photo_public_id IS NULL) = (passport_photo_version IS NULL));

-- +goose Down
DROP TABLE member_beneficiaries;
ALTER TABLE members
    DROP CONSTRAINT members_passport_photo,
    DROP COLUMN passport_photo_public_id,
    DROP COLUMN passport_photo_version,
    DROP COLUMN title,
    DROP COLUMN middle_name,
    DROP COLUMN alternate_phone,
    DROP COLUMN place_of_birth,
    DROP COLUMN nationality,
    DROP COLUMN state_of_origin,
    DROP COLUMN home_town,
    DROP COLUMN lga,
    DROP COLUMN next_of_kin_address,
    DROP COLUMN workplace_company,
    DROP COLUMN workplace_address,
    DROP COLUMN workplace_since,
    DROP COLUMN workplace_position,
    DROP COLUMN workplace_responsibilities,
    DROP COLUMN monthly_contribution_pledge,
    DROP COLUMN introduced_by_name,
    DROP COLUMN introduced_by_number,
    DROP COLUMN declaration_accepted_at;
