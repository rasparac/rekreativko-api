package mapper

import (
	"github.com/rasparac/rekreativko-api/account-profile/internal/application"
	"github.com/rasparac/rekreativko-api/account-profile/internal/domain"
	"github.com/rasparac/rekreativko-api/account-profile/internal/interfaces/http/dtos"
)

func UpdateProfileRequestToParams(req *dtos.UpdateProfileRequest) application.UpdateProfileParams {
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

	// Map flattened location fields
	if req.LocationCity != nil || req.LocationCountry != nil || req.LocationLatitude != nil || req.LocationLongitude != nil {
		updateProfileParams.Location = &application.Location{}
		if req.LocationCity != nil {
			updateProfileParams.Location.City = *req.LocationCity
		}
		if req.LocationCountry != nil {
			updateProfileParams.Location.Country = *req.LocationCountry
		}
		if req.LocationLatitude != nil && req.LocationLongitude != nil {
			updateProfileParams.Location.Latitude = req.LocationLatitude
			updateProfileParams.Location.Longitude = req.LocationLongitude
		}
	}

	// Map profile picture URL
	if req.ProfilePictureURL != nil {
		updateProfileParams.ProfilePicture = &application.ProfilePicture{
			URL: *req.ProfilePictureURL,
		}
	}

	// Map activity interests from string array format "Name:Level"
	if req.ActivityInterests != nil && len(req.ActivityInterests) > 0 {
		ai := make([]application.ActivityInterest, 0, len(req.ActivityInterests))
		for _, interest := range req.ActivityInterests {
			// Parse "Name:Level" format (simple split on first colon)
			// For MVP, we'll accept the full string as-is or implement parsing later
			// TODO: Implement proper parsing of "Name:Level" format
			ai = append(ai, application.ActivityInterest{
				Name:  interest,
				Level: "intermediate", // Default level for now
			})
		}
		updateProfileParams.ActivityInterest = ai
	}

	return updateProfileParams
}

func DomainProfileToResponse(p *domain.AccountProfile) dtos.AccountProfileResponse {
	resp := dtos.AccountProfileResponse{
		ID:        p.ID(),
		Bio:       p.Bio(),
		CreatedAt: p.CreatedAt(),
		UpdatedAt: p.UpdatedAt(),
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
