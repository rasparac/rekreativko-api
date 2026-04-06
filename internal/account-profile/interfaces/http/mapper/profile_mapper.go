package mapper

import (
	"github.com/rasparac/rekreativko-api/internal/account-profile/application"
	"github.com/rasparac/rekreativko-api/internal/account-profile/domain"
	"github.com/rasparac/rekreativko-api/internal/account-profile/interfaces/http/dtos"
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

	if req.Location != nil {
		updateProfileParams.Location = &application.Location{
			City:      req.Location.City,
			Country:   req.Location.Country,
			Latitude:  &req.Location.Coordinates.Latitude,
			Longitude: &req.Location.Coordinates.Longitude,
		}
	}

	if req.ProfilePicture != nil {
		updateProfileParams.ProfilePicture = &application.ProfilePicture{
			URL: req.ProfilePicture.URL,
		}
	}

	if req.ActivityInterest != nil {
		ai := make([]application.ActivityInterest, 0, len(req.ActivityInterest))
		for _, activityInterest := range req.ActivityInterest {
			ai = append(ai, application.ActivityInterest{
				Name:  activityInterest.Name,
				Level: activityInterest.Level,
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
