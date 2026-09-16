package postgres

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// ErrInvalidPageToken is returned when a client-supplied page token can't be decoded.
// Callers should map this to a 400, never a 500 - it's a malformed/tampered client input.
var ErrInvalidPageToken = errors.New("invalid page token")

// PageCursor is the decoded contents of an opaque page token: the sort value and
// unique id of the last row on the previous page. Repositories use it to resume
// a keyset-paginated query exactly where the previous page left off, instead of
// using OFFSET (which gets slower and can skip/duplicate rows under concurrent
// writes as the offset grows).
//
// SortValue is carried as a string so one cursor shape works for every column
// type a repository might order by (RFC3339Nano for a timestamp, a formatted
// float for a computed expression like a Haversine distance, etc.) - each
// repository parses it back to the right Go type itself, since only the
// repository knows what its own ORDER BY actually is.
type PageCursor struct {
	SortValue string    `json:"s"`
	ID        uuid.UUID `json:"i"`
}

// EncodePageToken builds an opaque page token from the last row's sort value and id.
func EncodePageToken(sortValue string, id uuid.UUID) string {
	raw, _ := json.Marshal(PageCursor{SortValue: sortValue, ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

// DecodePageToken parses a page token produced by EncodePageToken. An empty
// token is a valid "first page" request and returns a nil cursor with no error.
func DecodePageToken(token string) (*PageCursor, error) {
	if token == "" {
		return nil, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidPageToken
	}

	var cursor PageCursor
	if err := json.Unmarshal(raw, &cursor); err != nil {
		return nil, ErrInvalidPageToken
	}

	return &cursor, nil
}

// BuildPage splits a slice fetched with LIMIT pageSize+1 into the page to
// return and the token for the next page. Callers query for pageSize+1 rows;
// if more than pageSize came back, there's another page - trim the extra row
// and encode a token from the last row that's actually being returned.
func BuildPage[T any](rows []T, pageSize int, keyOf func(T) (sortValue string, id uuid.UUID)) (page []T, nextPageToken string) {
	if pageSize > 0 && len(rows) > pageSize {
		page = rows[:pageSize]
		sortValue, id := keyOf(page[pageSize-1])
		return page, EncodePageToken(sortValue, id)
	}

	return rows, ""
}
