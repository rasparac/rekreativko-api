// Package main Rekreativko API
//
//	@title						Rekreativko API
//	@version					1.0
//	@description				Recreation and sports activity management platform API
//	@description				This API provides endpoints for managing activity groups, sessions, RSVPs, and user profiles.
//	@description				All authenticated endpoints require a Bearer token in the Authorization header.
//	@termsOfService				https://github.com/rasparac/rekreativko-api
//
//	@contact.name				Rekreativko Support
//	@contact.url				https://github.com/rasparac/rekreativko-api
//	@contact.email				support@rekreativko.com
//
//	@license.name				MIT
//	@license.url				https://github.com/rasparac/rekreativko-api/blob/main/LICENSE
//
//	@host						localhost:8080
//	@BasePath					/
//	@schemes					http https
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type "Bearer" followed by a space and JWT token.
//
//	@tag.name					Authentication
//	@tag.description			User authentication and account management endpoints
//
//	@tag.name					Activity Groups
//	@tag.description			Endpoints for managing activity groups (sports clubs, teams, etc.)
//
//	@tag.name					Sessions
//	@tag.description			Endpoints for managing activity sessions (practices, games, events)
//
//	@tag.name					Session Templates
//	@tag.description			Endpoints for managing recurring session templates
//
//	@tag.name					Members
//	@tag.description			Endpoints for managing group memberships and roles
//
//	@tag.name					RSVPs
//	@tag.description			Endpoints for managing session RSVPs and attendees
//
//	@tag.name					Profiles
//	@tag.description			User profile management endpoints
//
//	@x-tagGroups				[{"name":"User Management","tags":["Authentication","Profiles"]},{"name":"Activity Management","tags":["Activity Groups","Sessions","Session Templates","Members","RSVPs"]}]
package main
