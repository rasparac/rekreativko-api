package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rasparac/rekreativko-api/notifications/internal/domain"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
)

type notificationModel struct {
	id                 uuid.UUID
	eventID            uuid.UUID
	recipientAccountID uuid.UUID
	notificationType   string
	data               []byte
	readAt             sql.NullTime
	createdAt          time.Time
}

// NotificationFilter defines query filters for listing a recipient's notifications
type NotificationFilter struct {
	RecipientAccountID uuid.UUID
	UnreadOnly         bool
	// Limit caps the number of rows returned, newest first; 0 means no cap.
	Limit int
}

// NotificationRepository defines the interface for notification persistence
type NotificationRepository interface {
	// Create inserts the notification. It is idempotent per (event_id, recipient):
	// created is false when one already exists for that event and recipient.
	Create(ctx context.Context, notification *domain.Notification) (created bool, err error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, error)
	ListByRecipient(ctx context.Context, filter NotificationFilter) ([]*domain.Notification, error)
	CountUnread(ctx context.Context, recipientAccountID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, notification *domain.Notification) error
	// DeleteExpired deletes every notification created before createdBefore,
	// and read ones created before readCreatedBefore. Returns how many it deleted.
	DeleteExpired(ctx context.Context, readCreatedBefore, createdBefore time.Time) (int64, error)
}

type notificationManager struct {
	tx     *postgres.TransactionManager
	logger *logger.Logger
}

// NewNotificationRepository creates a new notification repository
func NewNotificationRepository(
	tx *postgres.TransactionManager,
	logger *logger.Logger,
) NotificationRepository {
	return &notificationManager{
		tx:     tx,
		logger: logger,
	}
}

func (m *notificationManager) Create(ctx context.Context, notification *domain.Notification) (bool, error) {
	model, err := notificationModelFromDomain(notification)
	if err != nil {
		return false, fmt.Errorf("build notification model: %w", err)
	}

	query := `
		INSERT INTO notifications.notification (
			id, event_id, recipient_account_id, type, data, read_at, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)
		ON CONFLICT (event_id, recipient_account_id) DO NOTHING
	`

	q := m.tx.Querier(ctx)

	tag, err := q.Exec(
		ctx,
		query,
		model.id,
		model.eventID,
		model.recipientAccountID,
		model.notificationType,
		model.data,
		model.readAt,
		model.createdAt,
	)
	if err != nil {
		return false, fmt.Errorf("failed to create notification: %w", err)
	}

	return tag.RowsAffected() > 0, nil
}

func (m *notificationManager) GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, error) {
	query := `
		SELECT id, event_id, recipient_account_id, type, data, read_at, created_at
		FROM notifications.notification
		WHERE id = $1
	`

	q := m.tx.Querier(ctx)

	var model notificationModel
	err := q.QueryRow(ctx, query, id).Scan(
		&model.id,
		&model.eventID,
		&model.recipientAccountID,
		&model.notificationType,
		&model.data,
		&model.readAt,
		&model.createdAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotificationNotFound
		}
		return nil, fmt.Errorf("failed to get notification: %w", err)
	}

	return notificationModelToDomain(&model)
}

func (m *notificationManager) ListByRecipient(
	ctx context.Context,
	filter NotificationFilter,
) ([]*domain.Notification, error) {
	qb := &postgres.QueryBuilder{
		BaseQuery: `
			SELECT id, event_id, recipient_account_id, type, data, read_at, created_at
			FROM notifications.notification
			WHERE 1=1`,
		Args: make([]any, 0),
	}

	qb.AddCondition("recipient_account_id = ", filter.RecipientAccountID)

	if filter.UnreadOnly {
		qb.AddRawCondition("read_at IS NULL")
	}

	qb.BaseQuery += " ORDER BY created_at DESC, id DESC"

	if filter.Limit > 0 {
		qb.ParamCount++
		qb.BaseQuery += fmt.Sprintf(" LIMIT $%d", qb.ParamCount)
		qb.Args = append(qb.Args, filter.Limit)
	}

	query, args := qb.Build()

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*domain.Notification
	for rows.Next() {
		var model notificationModel
		err := rows.Scan(
			&model.id,
			&model.eventID,
			&model.recipientAccountID,
			&model.notificationType,
			&model.data,
			&model.readAt,
			&model.createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}

		notification, err := notificationModelToDomain(&model)
		if err != nil {
			return nil, fmt.Errorf("failed to convert notification: %w", err)
		}

		notifications = append(notifications, notification)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("error iterating notifications: %w", rows.Err())
	}

	return notifications, nil
}

func (m *notificationManager) CountUnread(ctx context.Context, recipientAccountID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM notifications.notification
		WHERE recipient_account_id = $1
		  AND read_at IS NULL
	`

	q := m.tx.Querier(ctx)

	var count int
	err := q.QueryRow(ctx, query, recipientAccountID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count unread notifications: %w", err)
	}

	return count, nil
}

func (m *notificationManager) MarkRead(ctx context.Context, notification *domain.Notification) error {
	query := `
		UPDATE notifications.notification
		SET read_at = $2
		WHERE id = $1
	`

	q := m.tx.Querier(ctx)

	var readAt sql.NullTime
	if notification.ReadAt() != nil {
		readAt = sql.NullTime{Time: *notification.ReadAt(), Valid: true}
	}

	_, err := q.Exec(ctx, query, notification.ID(), readAt)
	if err != nil {
		return fmt.Errorf("failed to mark notification read: %w", err)
	}

	return nil
}

func (m *notificationManager) DeleteExpired(
	ctx context.Context,
	readCreatedBefore, createdBefore time.Time,
) (int64, error) {
	query := `
		DELETE FROM notifications.notification
		WHERE created_at < $2
		   OR (created_at < $1 AND read_at IS NOT NULL)
	`

	q := m.tx.Querier(ctx)

	tag, err := q.Exec(ctx, query, readCreatedBefore, createdBefore)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired notifications: %w", err)
	}

	return tag.RowsAffected(), nil
}

func notificationModelFromDomain(n *domain.Notification) (*notificationModel, error) {
	data, err := json.Marshal(n.Data())
	if err != nil {
		return nil, fmt.Errorf("marshal notification data: %w", err)
	}

	model := &notificationModel{
		id:                 n.ID(),
		eventID:            n.EventID(),
		recipientAccountID: n.RecipientAccountID(),
		notificationType:   string(n.Type()),
		data:               data,
		createdAt:          n.CreatedAt(),
	}

	if n.ReadAt() != nil {
		model.readAt = sql.NullTime{Time: *n.ReadAt(), Valid: true}
	}

	return model, nil
}

func notificationModelToDomain(model *notificationModel) (*domain.Notification, error) {
	var data map[string]any
	if len(model.data) > 0 {
		if err := json.Unmarshal(model.data, &data); err != nil {
			return nil, fmt.Errorf("unmarshal notification data: %w", err)
		}
	}

	var readAt *time.Time
	if model.readAt.Valid {
		readAt = &model.readAt.Time
	}

	return domain.Reconstruct(
		model.id,
		model.eventID,
		model.recipientAccountID,
		domain.NotificationType(model.notificationType),
		data,
		readAt,
		model.createdAt,
	), nil
}
