package dtos

type (

	// UpdateAccountSettingsRequest represents the request body for updating account settings.
	// Example:
	/*
		{
			"settings":{
			  "activity.search_radius": 10,
			  "notification.email.enabled": true,
			  "notification.push.enabled": false,
			  "notification.sms.enabled": false,
			  "preference.language": "en",
			  "preference.theme": "light",
			  "preference.timezone": "UTC",
			  "privacy.profile.public": false
			}
		}
	*/
	UpdateAccountSettingsRequest struct {
		Settings map[string]any `json:"settings"`
	}

	Setting struct {
		Value       any    `json:"value"`
		Type        string `json:"type"`
		Label       string `json:"label"`
		Category    string `json:"category"`
		Description string `json:"description"`
	}

	GetAccountSettingsResponse struct {
		Settings map[string]Setting `json:"settings"`
	}
)
