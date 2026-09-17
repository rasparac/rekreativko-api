CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

CREATE SCHEMA IF NOT EXISTS location;

CREATE TABLE location.geocode_cache(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    -- lat/long rounded to a fixed-precision grid cell (~110m), see
    -- domain.Coordinates.Bucket - the actual cache key.
    lat_bucket double precision NOT NULL,
    lng_bucket double precision NOT NULL,
    city varchar(255) NOT NULL DEFAULT '',
    country varchar(255) NOT NULL DEFAULT '',
    country_code varchar(2) NOT NULL DEFAULT '',
    -- true when Nominatim was asked and definitively returned no address for
    -- this bucket - cached too, so a bucket with no resolvable address
    -- doesn't burn a fresh (rate-limited) Nominatim call on every request.
    not_found boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

-- cache lookup key
CREATE UNIQUE INDEX idx_geocode_cache_bucket ON location.geocode_cache(lat_bucket, lng_bucket);
