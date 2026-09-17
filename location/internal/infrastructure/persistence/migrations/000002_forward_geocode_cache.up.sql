CREATE TABLE location.forward_geocode_cache(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    -- normalized "street|city|country" query, see
    -- domain.ForwardGeocodeQuery.CacheKey - the actual cache key.
    query_key varchar(512) NOT NULL,
    latitude double precision NOT NULL DEFAULT 0,
    longitude double precision NOT NULL DEFAULT 0,
    display_name varchar(512) NOT NULL DEFAULT '',
    city varchar(255) NOT NULL DEFAULT '',
    country varchar(255) NOT NULL DEFAULT '',
    country_code varchar(2) NOT NULL DEFAULT '',
    -- true when Nominatim was asked and definitively returned no match for
    -- this query - cached too, so a query with no resolvable address doesn't
    -- burn a fresh (rate-limited) Nominatim call on every request.
    not_found boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW()
);

-- cache lookup key
CREATE UNIQUE INDEX idx_forward_geocode_cache_query_key ON location.forward_geocode_cache(query_key);
