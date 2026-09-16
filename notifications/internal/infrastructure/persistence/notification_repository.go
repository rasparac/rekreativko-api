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
	Limit              int
	PageToken          string
}

// NotificationRepository defines the interface for notification persistence
type NotificationRepository interface {
	Create(ctx context.Context, notification *domain.Notification) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, error)
	ListByRecipient(ctx context.Context, filter NotificationFilter) ([]*domain.Notification, string, error)
	CountUnread(ctx context.Context, recipientAccountID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, notification *domain.Notification) error
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

func (m *notificationManager) Create(ctx context.Context, notification *domain.Notification) error {
	model, err := notificationModelFromDomain(notification)
	if err != nil {
		return fmt.Errorf("build notification model: %w", err)
	}

	query := `
		INSERT INTO notifications.notification (
			id, recipient_account_id, type, data, read_at, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6
		)
	`

	q := m.tx.Querier(ctx)

	_, err = q.Exec(
		ctx,
		query,
		model.id,
		model.recipientAccountID,
		model.notificationType,
		model.data,
		model.readAt,
		model.createdAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create notification: %w", err)
	}

	return nil
}

func (m *notificationManager) GetByID(ctx context.Context, id uuid.UUID) (*domain.Notification, error) {
	query := `
		SELECT id, recipient_account_id, type, data, read_at, created_at
		FROM notifications.notification
		WHERE id = $1
	`

	q := m.tx.Querier(ctx)

	var model notificationModel
	err := q.QueryRow(ctx, query, id).Scan(
		&model.id,
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
) ([]*domain.Notification, string, error) {
	qb := &postgres.QueryBuilder{
		BaseQuery: `
			SELECT id, recipient_account_id, type, data, read_at, created_at
			FROM notifications.notification
			WHERE 1=1`,
		Args: make([]any, 0),
	}

	qb.AddCondition("recipient_account_id = ", filter.RecipientAccountID)

	if filter.UnreadOnly {
		qb.AddRawCondition("read_at IS NULL")
	}

	cursor, err := postgres.DecodePageToken(filter.PageToken)
	if err != nil {
		return nil, "", err
	}
	if cursor != nil {
		sortValue, err := time.Parse(time.RFC3339Nano, cursor.SortValue)
		if err != nil {
			return nil, "", postgres.ErrInvalidPageToken
		}
		qb.AddKeysetCondition("created_at", "DESC", sortValue, cursor.ID)
	}

	qb.BaseQuery += " ORDER BY created_at DESC, id DESC"

	if filter.Limit > 0 {
		qb.ParamCount++
		qb.BaseQuery += fmt.Sprintf(" LIMIT $%d", qb.ParamCount)
		qb.Args = append(qb.Args, filter.Limit+1)
	}

	query, args := qb.Build()

	q := m.tx.Querier(ctx)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*domain.Notification
	for rows.Next() {
		var model notificationModel
		err := rows.Scan(
			&model.id,
			&model.recipientAccountID,
			&model.notificationType,
			&model.data,
			&model.readAt,
			&model.createdAt,
		)
		if err != nil {
			return nil, "", fmt.Errorf("failed to scan notification: %w", err)
		}

		notification, err := notificationModelToDomain(&model)
		if err != nil {
			return nil, "", fmt.Errorf("failed to convert notification: %w", err)
		}

		notifications = append(notifications, notification)
	}

	if rows.Err() != nil {
		return nil, "", fmt.Errorf("error iterating notifications: %w", rows.Err())
	}

	page, nextPageToken := postgres.BuildPage(notifications, filter.Limit, func(n *domain.Notification) (string, uuid.UUID) {
		return n.CreatedAt().UTC().Format(time.RFC3339Nano), n.ID()
	})

	return page, nextPageToken, nil
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

func notificationModelFromDomain(n *domain.Notification) (*notificationModel, error) {
	data, err := json.Marshal(n.Data())
	if err != nil {
		return nil, fmt.Errorf("marshal notification data: %w", err)
	}

	model := &notificationModel{
		id:                 n.ID(),
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
		model.recipientAccountID,
		domain.NotificationType(model.notificationType),
		data,
		readAt,
		model.createdAt,
	), nil
}
