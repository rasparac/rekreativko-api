package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/account-profile/internal/infrastructure/persistence"
	"github.com/rasparac/rekreativko-api/account-profile/internal/metrics"
	"github.com/rasparac/rekreativko-api/shared/domainerror"
	"github.com/rasparac/rekreativko-api/shared/domainevent"
	"github.com/rasparac/rekreativko-api/shared/logger"
	"github.com/rasparac/rekreativko-api/shared/store/postgres"
	"github.com/rasparac/rekreativko-api/shared/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const accountProfileSchema = "account_profile"

type (
	service struct {
		logger         *logger.Logger
		txManager      *postgres.TransactionManager
		accountProfile persistence.AccountProfileReaderWriter
		tracer         trace.Tracer
		eventWriter    domainevent.EventWriter
		metrics        *metrics.Metrics
	}
)

func NewService(
	logger *logger.Logger,
	txManager *postgres.TransactionManager,
	accountProfile persistence.AccountProfileReaderWriter,
	eventWriter domainevent.EventWriter,
	metrics *metrics.Metrics,
) *service {

	return &service{
		logger:         logger.WithName("account_profile.service"),
		accountProfile: accountProfile,
		eventWriter:    eventWriter,
		txManager:      txManager,
		tracer:         telemetry.Tracer(telemetry.TracerAccountProfileService),
		metrics:        metrics,
	}
}

func (s *service) CreateProfile(ctx context.Context, createProfile CreateProfileParams) (*domain.AccountProfile, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"account_profile.service.CreateProfile",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "CreateProfile",
		"account_id", createProfile.AccountID,
	)

	newProfile := domain.NewAccountProfile(createProfile.AccountID)

	span.SetAttributes(attribute.String(
		"account_id", createProfile.AccountID.String(),
	))
	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		err := s.accountProfile.CreateAccountProfile(tCtx, newProfile)
		if err != nil {
			return fmt.Errorf("create account profile: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			accountProfileSchema,
			newProfile.Events(),
		)
		if err != nil {
			return fmt.Errorf("publish events: %w", err)
		}

		newProfile.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "create a new account", "error", err)
		return nil, mapToAppErr(err)
	}

	s.metrics.ProfileCreated.Inc()

	log.Debug(ctx, "profile created")
	span.SetStatus(codes.Ok, "profile created")

	return newProfile, nil
}

func (s *service) GetProfile(ctx context.Context, filter ProfileFilter) (*domain.AccountProfile, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"account_profile.service.GetProfile",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "GetProfile",
		"account_id", filter.AccountID,
	)

	span.SetAttributes(attribute.String(
		"account_id", filter.AccountID.String(),
	))

	profile, err := s.accountProfile.FindBy(ctx, persistence.AccountProfileFilter{
		AccountID: filter.AccountID,
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "get an account", "error", err)
		return nil, mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "account found")

	log.Debug(ctx, "account found")

	return profile, nil
}

func (s *service) UpdateProfile(ctx context.Context, accountID uuid.UUID, toUpdateProfile UpdateProfileParams) error {
	ctx, span := s.tracer.Start(
		ctx,
		"account_profile.service.UpdateProfile",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "UpdateProfile",
		"account_id", accountID,
	)

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		profile, err := s.accountProfile.FindBy(tCtx, persistence.AccountProfileFilter{
			AccountID: &accountID,
		})
		if err != nil {
			return fmt.Errorf("get profile: %w", err)
		}

		if toUpdateProfile.Nickname != nil {
			newNickname, nickNameErr := domain.NewNickname(*toUpdateProfile.Nickname)
			if nickNameErr != nil {
				return fmt.Errorf("new nickname: %w", err)
			}
			err = profile.SetNickname(newNickname)
			if err != nil {
				return fmt.Errorf("set nickname: %w", err)
			}
		}

		if toUpdateProfile.FullName != nil ||
			toUpdateProfile.DateOfBirth != nil ||
			toUpdateProfile.Bio != nil {
			var fullName *domain.FullName
			if toUpdateProfile.FullName != nil && *toUpdateProfile.FullName != "" {
				newFullName, err := domain.NewFullName(*toUpdateProfile.FullName)
				if err != nil {
					return fmt.Errorf("new full name: %w", err)
				}
				fullName = newFullName
			} else if toUpdateProfile.FullName != nil && *toUpdateProfile.FullName == "" {
				fullName = nil
			} else {
				fullName = profile.FullName()
			}

			var dateOfBirth *domain.DateOfBirth
			if toUpdateProfile.DateOfBirth != nil && !toUpdateProfile.DateOfBirth.IsZero() {
				newDateOfBirth, err := domain.NewDateOfBirth(*toUpdateProfile.DateOfBirth)
				if err != nil {
					return fmt.Errorf("new date of birth: %w", err)
				}
				dateOfBirth = newDateOfBirth
			} else if toUpdateProfile.DateOfBirth != nil && toUpdateProfile.DateOfBirth.IsZero() {
				dateOfBirth = nil
			} else {
				dateOfBirth = profile.DateOfBirth()
			}

			bio := profile.Bio()
			if toUpdateProfile.Bio != nil {
				bio = *toUpdateProfile.Bio
			}

			err = profile.UpdateProfile(fullName, dateOfBirth, bio)
			if err != nil {
				return fmt.Errorf("update profile: %w", err)
			}
		}

		// Location fields are omitted entirely (Location == nil) whenever the
		// request doesn't touch location at all - leave the existing value
		// untouched in that case, same as full_name/bio above. An explicitly
		// empty location_city is the sentinel for "clear my location".
		if toUpdateProfile.Location != nil {
			if toUpdateProfile.Location.City != nil && *toUpdateProfile.Location.City == "" {
				if err := profile.SetLocation(nil); err != nil {
					return fmt.Errorf("set location: %w", err)
				}
			} else {
				existing := profile.Location()

				city := ""
				if toUpdateProfile.Location.City != nil {
					city = *toUpdateProfile.Location.City
				} else if existing != nil {
					city = existing.City()
				}

				country := ""
				if toUpdateProfile.Location.Country != nil {
					country = *toUpdateProfile.Location.Country
				} else if existing != nil {
					country = existing.Country()
				}

				// A new pair is only applied when BOTH fields are sent; a lone
				// field (or neither) falls back to the existing pair rather
				// than dropping coordinates entirely.
				lat := toUpdateProfile.Location.Latitude
				lng := toUpdateProfile.Location.Longitude
				if (lat == nil || lng == nil) && existing != nil && existing.HasCoordinates() {
					existingLat := existing.Coordinates().Latitude()
					existingLng := existing.Coordinates().Longitude()
					lat = &existingLat
					lng = &existingLng
				}

				newLocation, err := domain.NewLocation(city, country, lat, lng)
				if err != nil {
					return fmt.Errorf("new location: %w", err)
				}
				if err := profile.SetLocation(newLocation); err != nil {
					return fmt.Errorf("set location: %w", err)
				}
			}
		}

		if toUpdateProfile.ProfilePicture != nil {
			newProfilePicture, err := domain.NewProfilePicture(
				toUpdateProfile.ProfilePicture.URL,
				time.Now().UTC(),
			)
			if err != nil {
				return fmt.Errorf("new profile picture: %w", err)
			}
			err = profile.SetProfilePicture(newProfilePicture)
			if err != nil {
				return fmt.Errorf("set profile picture: %w", err)
			}
		} else if toUpdateProfile.ProfilePicture == nil {
			err = profile.SetProfilePicture(nil)
			if err != nil {
				return fmt.Errorf("set profile picture: %w", err)
			}
		}

		if toUpdateProfile.ActivityInterest != nil {
			newInterests := make([]*domain.ActivityInterest, 0, len(toUpdateProfile.ActivityInterest))

			for _, ai := range toUpdateProfile.ActivityInterest {
				interest, err := domain.NewActivityInterest(
					domain.ActivityType(ai.Name),
					domain.ActivityLevel(ai.Level),
				)
				if err != nil {
					return fmt.Errorf("new activity interest: %w", err)
				}
				newInterests = append(newInterests, interest)
			}

			if err := profile.UpdateActivityInterests(newInterests); err != nil {
				return fmt.Errorf("update activity interests: %w", err)
			}
		}

		err = s.accountProfile.UpdateAccountProfile(tCtx, profile)
		if err != nil {
			return fmt.Errorf("update profile: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			accountProfileSchema,
			profile.Events(),
		)
		if err != nil {
			return fmt.Errorf("publish events: %w", err)
		}

		profile.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "update profile", "error", err)
		return mapToAppErr(err)
	}

	s.metrics.ProfileUpdated.Inc()

	span.SetStatus(codes.Ok, "profile updated")

	log.Debug(ctx, "profile updated")

	return nil
}

func (s *service) GetProfiles(ctx context.Context, filter ProfilesFilter) ([]*domain.AccountProfile, string, error) {
	ctx, span := s.tracer.Start(
		ctx,
		"account_profile.service.GetProfiles",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "GetProfiles",
		"account_ids", filter.AccountIDs,
		"nicknames", filter.Nicknames,
	)

	profiles, nextPageToken, err := s.accountProfile.FindAllBy(ctx, persistence.AccountProfilesFilter{
		ByAccounIDs:       filter.AccountIDs,
		ByNicknames:       filter.Nicknames,
		ByLocationCountry: filter.LocationCountry,
		ByLocationCity:    filter.LocationCity,
		DateOfBirthOver:   filter.DateOfBirthOver,
		DateOfBirthUnder:  filter.DateOfBirthUnder,
		IncludeDeleted:    filter.IncludeDeleted,
		SortBy:            filter.SortBy,
		SortOrder:         filter.SortOrder,

		Limit:     filter.Limit,
		PageToken: filter.PageToken,
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "list account profiles", "error", err)
		return nil, "", mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "get profiles finished")

	log.Debug(ctx, "get profiles finished")

	return profiles, nextPageToken, nil
}

func (s *service) DeleteProfile(ctx context.Context, accountID uuid.UUID) error {
	ctx, span := s.tracer.Start(
		ctx,
		"account_profile.service.DeleteProfile",
	)
	defer span.End()

	log := s.logger.WithValues(
		"method", "DeleteProfile",
		"account_id", accountID,
	)

	err := s.txManager.WithTransaction(ctx, func(tCtx context.Context) error {
		profile, err := s.accountProfile.FindBy(tCtx, persistence.AccountProfileFilter{
			AccountID: &accountID,
		})
		if err != nil {
			return fmt.Errorf("find profile: %w", err)
		}

		profile.Delete()

		err = s.accountProfile.DeleteAccountProfile(tCtx, profile.ID())
		if err != nil {
			return fmt.Errorf("delete profile: %w", err)
		}

		err = s.eventWriter.InsertEvents(
			tCtx,
			accountProfileSchema,
			profile.Events(),
		)
		if err != nil {
			return fmt.Errorf("publish events: %w", err)
		}

		profile.ClearEvents()

		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		log.Error(ctx, "delete profile", "error", err)
		return mapToAppErr(err)
	}

	span.SetStatus(codes.Ok, "profile deleted")

	s.metrics.ProfileDeleted.Inc()

	log.Debug(ctx, "profile deleted")

	return nil
}

func mapToAppErr(err error) *domainerror.AppError {
	// Check for pgx.ErrNoRows first - it's handled in domain.MapErrToAppError
	// because it's not a *pgconn.PgError

	pgErr := postgres.GetPgxError(err)
	if pgErr != nil {
		return domainerror.MapPostgresError(pgErr)
	}

	return domain.MapErrToAppError(err)
}
