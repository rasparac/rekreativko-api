package mapper

import (
	"time"

	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/account-profile/internal/interfaces/http/dtos"
)

const dateOfBirthLayout = "2006-01-02"

func UpdateProfileRequestToParams(req *dtos.UpdateProfileRequest) (application.UpdateProfileParams, error) {
	var updateProfileParams application.UpdateProfileParams

	if req.Nickname != nil {
		updateProfileParams.Nickname = req.Nickname
	}

	if req.FullName != nil {
		updateProfileParams.FullName = req.FullName
	}

	if req.Bio != nil {
		updateProfileParams.Bio = req.Bio
	}

	// Omitting date_of_birth leaves the existing value untouched; an explicitly
	// empty string clears it (parsed to time.Time{}, the service's clear sentinel).
	if req.DateOfBirth != nil {
		if *req.DateOfBirth == "" {
			updateProfileParams.DateOfBirth = &time.Time{}
		} else {
			dob, err := time.Parse(dateOfBirthLayout, *req.DateOfBirth)
			if err != nil {
				return updateProfileParams, err
			}
			updateProfileParams.DateOfBirth = &dob
		}
	}

	// Map flattened location fields. Omitting all four leaves the existing
	// location untouched (handled in the service); an explicitly empty
	// location_city clears it.
	if req.LocationCity != nil || req.LocationCountry != nil || req.LocationLatitude != nil || req.LocationLongitude != nil {
		updateProfileParams.Location = &application.Location{
			City:      req.LocationCity,
			Country:   req.LocationCountry,
			Latitude:  req.LocationLatitude,
			Longitude: req.LocationLongitude,
		}
	}

	// Map profile picture URL
	if req.ProfilePictureURL != nil {
		updateProfileParams.ProfilePicture = &application.ProfilePicture{
			URL: *req.ProfilePictureURL,
		}
	}

	if len(req.ActivityInterests) > 0 {
		ai := make([]application.ActivityInterest, 0, len(req.ActivityInterests))
		for _, interest := range req.ActivityInterests {
			ai = append(ai, application.ActivityInterest{
				Name:  interest.Name,
				Level: interest.Level,
			})
		}
		updateProfileParams.ActivityInterest = ai
	}

	return updateProfileParams, nil
}

func DomainProfileToResponse(p *domain.AccountProfile) dtos.AccountProfileResponse {
	resp := dtos.AccountProfileResponse{
		ID:        p.ID(),
		Bio:       p.Bio(),
		CreatedAt: p.CreatedAt(),
		UpdatedAt: p.UpdatedAt(),
	}

	if p.DateOfBirth() != nil {
		dob := p.DateOfBirth().Value().Format(dateOfBirthLayout)
		resp.DateOfBirth = &dob
	}

	if p.FullName() != nil {
		resp.FullName = p.FullName().Value()
	}

	if p.Nickname() != nil {
		resp.Nickname = p.Nickname().Value()
	}

	if p.ProfilePicture() != nil {
		resp.ProfilePicture = &dtos.ProfilePicture{
			URL: p.ProfilePicture().URL(),
		}
	}

	if p.Location() != nil {
		resp.Location = mapLocation(p.Location())
	}

	if p.ActivityInterests() != nil {
		resp.ActivityInterest = mapActivityInterests(p.ActivityInterests())
	}

	return resp
}

func mapActivityInterests(ai []*domain.ActivityInterest) []dtos.ActivityInterest {
	resp := make([]dtos.ActivityInterest, 0, len(ai))
	for _, activityInterest := range ai {
		resp = append(resp, dtos.ActivityInterest{
			Name:  string(activityInterest.ActivityType()),
			Level: string(activityInterest.Level()),
		})
	}

	return resp
}

func mapLocation(loc *domain.Location) *dtos.Location {
	resp := &dtos.Location{
		City:    loc.City(),
		Country: loc.Country(),
	}

	if loc.HasCoordinates() {
		resp.Coordinates = &dtos.Coordinates{
			Latitude:  loc.Coordinates().Latitude(),
			Longitude: loc.Coordinates().Longitude(),
		}
	}

	return resp
}
