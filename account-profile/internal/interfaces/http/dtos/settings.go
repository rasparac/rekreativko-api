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
		// Settings is a map of setting keys to their values
		// Example: {"notification.email.enabled": true, "preference.language": "en"}
		Settings map[string]interface{} `json:"settings" swaggertype:"object"`
	}

	Setting struct {
		Value       interface{} `json:"value" swaggertype:"primitive,string" example:"true"`
		Type        string      `json:"type" example:"boolean"`
		Label       string      `json:"label" example:"Email Notifications"`
		Category    string      `json:"category" example:"notification"`
		Description string      `json:"description" example:"Enable or disable email notifications"`
	}

	GetAccountSettingsResponse struct {
		Settings map[string]Setting `json:"settings"`
	}
)
