package postgres

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodePageToken(t *testing.T) {
	t.Run("round trips sort value and id", func(t *testing.T) {
		id := uuid.New()
		token := EncodePageToken("2026-08-30T12:00:00Z", id)

		cursor, err := DecodePageToken(token)

		assert.NoError(t, err)
		assert.Equal(t, "2026-08-30T12:00:00Z", cursor.SortValue)
		assert.Equal(t, id, cursor.ID)
	})

	t.Run("empty token is a valid first-page request", func(t *testing.T) {
		cursor, err := DecodePageToken("")

		assert.NoError(t, err)
		assert.Nil(t, cursor)
	})

	t.Run("malformed token is rejected", func(t *testing.T) {
		_, err := DecodePageToken("not-a-valid-token!!!")

		assert.ErrorIs(t, err, ErrInvalidPageToken)
	})

	t.Run("tampered but base64-valid token is rejected", func(t *testing.T) {
		_, err := DecodePageToken("aGVsbG8") // valid base64, not valid JSON

		assert.ErrorIs(t, err, ErrInvalidPageToken)
	})
}

func TestBuildPage(t *testing.T) {
	type row struct {
		id        uuid.UUID
		createdAt string
	}
	keyOf := func(r row) (string, uuid.UUID) { return r.createdAt, r.id }

	t.Run("no next page when fewer rows than page size", func(t *testing.T) {
		rows := []row{{id: uuid.New(), createdAt: "1"}, {id: uuid.New(), createdAt: "2"}}

		page, next := BuildPage(rows, 5, keyOf)

		assert.Equal(t, rows, page)
		assert.Empty(t, next)
	})

	t.Run("no next page when exactly page size rows", func(t *testing.T) {
		rows := []row{{id: uuid.New(), createdAt: "1"}, {id: uuid.New(), createdAt: "2"}}

		page, next := BuildPage(rows, 2, keyOf)

		assert.Equal(t, rows, page)
		assert.Empty(t, next)
	})

	t.Run("trims extra row and returns a token for the next page", func(t *testing.T) {
		last := uuid.New()
		rows := []row{
			{id: uuid.New(), createdAt: "1"},
			{id: last, createdAt: "2"},
			{id: uuid.New(), createdAt: "3"}, // the +1 lookahead row, should be trimmed
		}

		page, next := BuildPage(rows, 2, keyOf)

		assert.Len(t, page, 2)
		assert.NotEmpty(t, next)

		cursor, err := DecodePageToken(next)
		assert.NoError(t, err)
		assert.Equal(t, "2", cursor.SortValue)
		assert.Equal(t, last, cursor.ID)
	})
}
