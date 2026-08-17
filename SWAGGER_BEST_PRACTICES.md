# Swagger/OpenAPI Best Practices for Mobile Development

This guide outlines best practices for creating comprehensive API documentation that mobile developers will love.

## Table of Contents
1. [Quick Start](#quick-start)
2. [Best Practices](#best-practices)
3. [Handler Annotation Guide](#handler-annotation-guide)
4. [Common Patterns](#common-patterns)
5. [Mobile-Specific Tips](#mobile-specific-tips)

## Quick Start

### Generate Swagger Docs
```bash
# Generate swagger documentation
task docs:swagger

# Format swagger annotations
task docs:fmt

# View swagger UI (in dev mode)
# http://localhost:8080/swagger/index.html
```

### Installing swag CLI (if not installed)
```bash
go install github.com/swaggo/swag/cmd/swag@latest
```

## Best Practices

### 1. **Complete Endpoint Documentation**

Every endpoint should have:
- ✅ Summary (short, action-oriented)
- ✅ Description (detailed explanation)
- ✅ All parameters with examples
- ✅ All response codes with schemas
- ✅ Security requirements
- ✅ Tags for grouping

**Example:**
```go
// CreateRSVP handles POST /api/v1/sessions/{sessionId}/rsvp
//
//	@Summary		Create an RSVP for a session
//	@Description	Creates a new RSVP for the authenticated user for a specific session. Priority members bypass time restrictions.
//	@Tags			RSVPs
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			sessionId	path		string								true	"Session ID"	example(123e4567-e89b-12d3-a456-426655440000)
//	@Param			request		body		dtos.CreateRSVPRequest				true	"RSVP data"
//	@Success		201			{object}	api.Response[dtos.CreateRSVPResponse]	"RSVP created successfully"
//	@Failure		400			{object}	api.Response[any]						"Invalid request"
//	@Failure		401			{object}	api.Response[any]						"Unauthorized"
//	@Failure		409			{object}	api.Response[any]						"Already RSVPed or session full"
//	@Failure		500			{object}	api.Response[any]						"Internal server error"
//	@Router			/api/v1/sessions/{sessionId}/rsvp [post]
func (h *Handler) CreateRSVP(w http.ResponseWriter, r *http.Request) {
	// implementation
}
```

### 2. **Rich DTOs with Examples**

Mobile developers need to see example values. Add examples to ALL DTO fields:

```go
type CreateRSVPRequest struct {
	Status string `json:"status" validate:"required,oneof=going not_going maybe" example:"going"`
}

type AttendeeResponse struct {
	ID              uuid.UUID `json:"id" example:"123e4567-e89b-12d3-a456-426655440000"`
	SessionID       uuid.UUID `json:"session_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	ActivityGroupID uuid.UUID `json:"activity_group_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	UserID          uuid.UUID `json:"user_id" example:"123e4567-e89b-12d3-a456-426655440000"`
	Status          string    `json:"status" example:"going" enums:"going,pending,not_going,maybe,promoted"`
	Source          string    `json:"source" example:"rsvp_manual" enums:"rsvp_manual,auto_confirmed,auto_pending"`
	CreatedAt       time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt       time.Time `json:"updated_at" example:"2024-01-01T00:00:00Z"`
}
```

### 3. **Enum Documentation**

Always document enums so mobile devs know valid values:

```go
// @Param status query string false "Filter by status" Enums(going, pending, not_going, maybe, promoted)
```

Or in DTOs:
```go
Status string `json:"status" enums:"going,pending,not_going,maybe,promoted"`
```

### 4. **Error Response Consistency**

Document ALL possible error codes and provide examples:

```go
//	@Failure		400			{object}	api.Response[any]	"Bad Request - Invalid input"
//	@Failure		401			{object}	api.Response[any]	"Unauthorized - Missing or invalid token"
//	@Failure		403			{object}	api.Response[any]	"Forbidden - Insufficient permissions"
//	@Failure		404			{object}	api.Response[any]	"Not Found - Resource doesn't exist"
//	@Failure		409			{object}	api.Response[any]	"Conflict - Duplicate resource or constraint violation"
//	@Failure		422			{object}	api.Response[any]	"Unprocessable Entity - Validation failed"
//	@Failure		429			{object}	api.Response[any]	"Too Many Requests - Rate limit exceeded"
//	@Failure		500			{object}	api.Response[any]	"Internal Server Error"
```

### 5. **Pagination Documentation**

Always document pagination parameters and response structure:

```go
//	@Param			limit		query		int		false	"Limit number of results (default 20, max 100)"	minimum(1)	maximum(100)	default(20)
//	@Param			offset		query		int		false	"Offset for pagination (default 0)"				minimum(0)	default(0)
```

### 6. **Query Parameter Examples**

Provide examples for all query parameters:

```go
//	@Param			session_id			query		string	false	"Filter by session ID"			example(123e4567-e89b-12d3-a456-426655440000)
//	@Param			activity_group_id	query		string	false	"Filter by activity group ID"	example(123e4567-e89b-12d3-a456-426655440000)
//	@Param			status				query		string	false	"Filter by status"				Enums(going, pending, not_going, maybe)
```

### 7. **Response Wrappers**

Document your standard response wrapper:

```go
// Standard successful response wrapper
type Response[T any] struct {
	Success bool   `json:"success" example:"true"`
	Data    T      `json:"data"`
	Message string `json:"message" example:"Operation successful"`
}

// Standard error response
type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Error   Error  `json:"error"`
}

type Error struct {
	Code    string `json:"code" example:"validation_error"`
	Message string `json:"message" example:"Validation failed"`
	Details any    `json:"details,omitempty"`
}
```

## Handler Annotation Guide

### Minimal Required Annotations

```go
//	@Summary		Short description (max 120 chars)
//	@Tags			TagName
//	@Router			/api/v1/endpoint [method]
```

### Complete Annotations (Recommended for Mobile)

```go
//	@Summary		Short description
//	@Description	Detailed multi-line description
//	@Description	Can have multiple lines
//	@Tags			TagName
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			paramName	type	dataType	required	"description"	additional
//	@Success		200			{object}	ResponseType	"Success message"
//	@Failure		400			{object}	ErrorType		"Error message"
//	@Router			/api/v1/endpoint [method]
```

### Parameter Types

- `path` - URL path parameter: `/users/{id}`
- `query` - Query parameter: `/users?status=active`
- `header` - HTTP header: `Authorization: Bearer token`
- `body` - Request body (usually JSON)
- `formData` - Form data (multipart/form-data)

## Common Patterns

### 1. Create Endpoint
```go
//	@Summary		Create a resource
//	@Description	Creates a new resource with the provided data
//	@Tags			Resources
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateResourceRequest		true	"Resource data"
//	@Success		201		{object}	api.Response[CreateResourceResponse]	"Resource created successfully"
//	@Failure		400		{object}	api.Response[any]			"Invalid request"
//	@Failure		401		{object}	api.Response[any]			"Unauthorized"
//	@Failure		409		{object}	api.Response[any]			"Resource already exists"
//	@Failure		500		{object}	api.Response[any]			"Internal server error"
//	@Router			/api/v1/resources [post]
```

### 2. Get Endpoint
```go
//	@Summary		Get a resource
//	@Description	Retrieves a resource by ID
//	@Tags			Resources
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string						true	"Resource ID"	example(123e4567-e89b-12d3-a456-426655440000)
//	@Success		200	{object}	api.Response[ResourceResponse]	"Resource retrieved successfully"
//	@Failure		400	{object}	api.Response[any]			"Invalid ID"
//	@Failure		401	{object}	api.Response[any]			"Unauthorized"
//	@Failure		404	{object}	api.Response[any]			"Resource not found"
//	@Failure		500	{object}	api.Response[any]			"Internal server error"
//	@Router			/api/v1/resources/{id} [get]
```

### 3. List Endpoint
```go
//	@Summary		List resources
//	@Description	Lists all resources with optional filtering and pagination
//	@Tags			Resources
//	@Produce		json
//	@Security		BearerAuth
//	@Param			status		query		string	false	"Filter by status"				Enums(active, inactive)
//	@Param			limit		query		int		false	"Limit (default 20, max 100)"	minimum(1)	maximum(100)	default(20)
//	@Param			offset		query		int		false	"Offset (default 0)"			minimum(0)	default(0)
//	@Success		200			{object}	api.Response[ResourceListResponse]	"Resources retrieved successfully"
//	@Failure		400			{object}	api.Response[any]					"Invalid parameters"
//	@Failure		401			{object}	api.Response[any]					"Unauthorized"
//	@Failure		500			{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/resources [get]
```

### 4. Update Endpoint
```go
//	@Summary		Update a resource
//	@Description	Updates an existing resource
//	@Tags			Resources
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string						true	"Resource ID"
//	@Param			request	body		UpdateResourceRequest		true	"Updated data"
//	@Success		200		{object}	api.Response[ResourceResponse]	"Resource updated successfully"
//	@Failure		400		{object}	api.Response[any]			"Invalid request"
//	@Failure		401		{object}	api.Response[any]			"Unauthorized"
//	@Failure		404		{object}	api.Response[any]			"Resource not found"
//	@Failure		500		{object}	api.Response[any]			"Internal server error"
//	@Router			/api/v1/resources/{id} [put]
```

### 5. Delete Endpoint
```go
//	@Summary		Delete a resource
//	@Description	Deletes a resource by ID
//	@Tags			Resources
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string				true	"Resource ID"
//	@Success		200	{object}	api.Response[any]	"Resource deleted successfully"
//	@Failure		400	{object}	api.Response[any]	"Invalid ID"
//	@Failure		401	{object}	api.Response[any]	"Unauthorized"
//	@Failure		404	{object}	api.Response[any]	"Resource not found"
//	@Failure		500	{object}	api.Response[any]	"Internal server error"
//	@Router			/api/v1/resources/{id} [delete]
```

## Mobile-Specific Tips

### 1. **Always Include Examples**
Mobile devs will use these to understand the API instantly:
```go
example:"2024-01-15T14:30:00Z"
example:"active"
example:42
```

### 2. **Document Rate Limits**
```go
//	@Description	Rate limit: 100 requests per minute per user
//	@Failure		429	{object}	api.Response[any]	"Rate limit exceeded"
```

### 3. **Document Auth Requirements Clearly**
```go
//	@Security		BearerAuth
//	@Description	Requires authentication. Include: Authorization: Bearer <token>
```

### 4. **Version Your API**
```go
//	@version	1.0
//	@BasePath	/api/v1
```

### 5. **Provide Enum Values**
Mobile devs need to know all valid values:
```go
Status string `json:"status" enums:"draft,active,completed,cancelled"`
```

### 6. **Document Validation Rules**
```go
Email    string `json:"email" validate:"required,email" example:"user@example.com"`
Password string `json:"password" validate:"required,min=8,max=72" example:"SecurePass123!"`
Name     string `json:"name" validate:"required,min=2,max=100" example:"John Doe"`
```

### 7. **UUID Format Everywhere**
Always provide UUID examples in the proper format:
```go
example:"123e4567-e89b-12d3-a456-426655440000"
```

### 8. **Timestamp Format**
Use ISO 8601 (RFC3339) consistently:
```go
example:"2024-01-15T14:30:00Z"
```

### 9. **Pagination Metadata**
Always include pagination info in list responses:
```go
type ListResponse struct {
	Items  []Item `json:"items"`
	Total  int    `json:"total" example:"150"`
	Limit  int    `json:"limit" example:"20"`
	Offset int    `json:"offset" example:"0"`
}
```

### 10. **Error Codes**
Use consistent, descriptive error codes:
```go
type ErrorResponse struct {
	Code    string `json:"code" example:"validation_error" enums:"validation_error,not_found,unauthorized,forbidden,conflict,internal_error"`
	Message string `json:"message" example:"Validation failed for field 'email'"`
	Details any    `json:"details,omitempty"`
}
```

## Advanced: Custom Annotations

### Request Examples
```go
//	@Param			request	body	CreateRSVPRequest	true	"RSVP data"	SchemaExample({"status":"going"})
```

### Response Examples
```go
//	@Success		200	{object}	AttendeeResponse	"Success"	SchemaExample({"id":"123e4567-e89b-12d3-a456-426655440000","status":"going"})
```

### Custom Headers
```go
//	@Header			200	{string}	X-Request-ID	"Request ID for tracking"
//	@Header			200	{string}	X-RateLimit-Remaining	"Remaining requests"
```

## Testing Your Documentation

### 1. Generate and Check
```bash
task docs:swagger
# Check docs/swagger.json for completeness
```

### 2. Test in Swagger UI
```bash
# Start your server in dev mode
# Open http://localhost:8080/swagger/index.html
# Try each endpoint - click "Try it out"
```

### 3. Export for Mobile Team
```bash
# Mobile devs can import docs/swagger.json into:
# - Postman
# - Insomnia
# - OpenAPI Generator (to generate client SDKs)
# - Swagger Codegen
```

## Checklist for Each Endpoint

- [ ] Summary is clear and concise
- [ ] Description explains what the endpoint does
- [ ] All parameters are documented with examples
- [ ] All response codes are documented (at least 200, 400, 401, 500)
- [ ] Request body schema is complete with examples
- [ ] Response body schema is complete with examples
- [ ] Security requirements are specified
- [ ] Enum values are documented
- [ ] Validation rules are clear
- [ ] Tag is assigned for grouping
- [ ] UUIDs use standard format
- [ ] Timestamps use ISO 8601 format

## Common Mistakes to Avoid

❌ Missing examples
❌ Undocumented query parameters
❌ Missing error responses
❌ Vague descriptions
❌ Missing enum values
❌ Inconsistent response formats
❌ Missing validation rules
❌ No pagination documentation
❌ Missing security annotations
❌ Wrong parameter types (path vs query)

## Resources

- [Swag Documentation](https://github.com/swaggo/swag)
- [OpenAPI Specification](https://swagger.io/specification/)
- [Swagger Editor](https://editor.swagger.io/) - Validate your swagger.json
- [OpenAPI Generator](https://openapi-generator.tech/) - Generate client SDKs

## Quick Reference Card

```go
// Standard endpoint pattern
//	@Summary		Brief action (Create/Get/Update/Delete/List) + Resource
//	@Description	What it does + any important notes
//	@Tags			ResourceName
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string		true	"Resource ID"	example(uuid)
//	@Param			request	body		DTOType		true	"Request data"
//	@Success		200		{object}	ResponseDTO	"Success message"
//	@Failure		400		{object}	ErrorDTO	"Bad request"
//	@Failure		401		{object}	ErrorDTO	"Unauthorized"
//	@Failure		404		{object}	ErrorDTO	"Not found"
//	@Failure		500		{object}	ErrorDTO	"Server error"
//	@Router			/api/v1/resource/{id} [method]
```
