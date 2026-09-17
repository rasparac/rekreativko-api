package domain

import "errors"

var (
	// ErrLocationNotFound means the answer is definitively "no location" -
	// either Nominatim just said so, or a prior lookup already did and it
	// was cached. Distinct from ErrCacheMiss: this is a final answer, not an
	// instruction to go ask Nominatim.
	ErrLocationNotFound = errors.New("no location found for the given coordinates")
	// ErrCacheMiss means the repository has nothing cached for this bucket
	// yet (positive or negative) - the caller should ask Nominatim.
	ErrCacheMiss = errors.New("no cache entry for the given bucket")
	// ErrGeocodingUnavailable means the Nominatim call itself failed (network
	// error, non-200 response) - distinct from a clean "not found" answer,
	// and deliberately never cached since it's a transient condition worth
	// retrying on the next request.
	ErrGeocodingUnavailable = errors.New("geocoding provider unavailable")
)
