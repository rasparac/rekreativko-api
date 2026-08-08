Core business logic for the project is: Connect people who want to do activties together in their city.

This can be done:
- organizing group activties
- matchung people by location an interests
- managing event participation and capacity



***Domains:***

- Identity And Access Context
- User Profile Contenxt
- Activity Catalog Context
- Activity Group/Event Contenxt - CORE
- Discovery Context



***Identity And Access Context***

Responsibility:
- User authentication and authorization using email and phone number.

Aggregates:
- Account (email/phone number, password, token)

Use case:
- Register new user
- Login user
- Validate JWT
- Refresh token

Does not not know about: profiles, activities, groups

Events:
- Registration: AccountRegisteredEvent, VerificationCodeGeneratedEvent
- Verify Account: VerificationCodeUsedEvent, AccountVerifiedEvent
- Login (success): AccountLogicceededEvent,
- Login (failed): AccountLoginFailedEvent, AccountLockedEvent
- Change Password PasswordChangedEvent
- Resend Code VerificationCodeGeneratedEvent


***User Profile Context***

Responsibility:
- User personal information and preferences.

Aggregates:
- UserProfile (nicname, firstName, lastName, city, interests)
    - userID from identity
    - personal info: nickname, full name, age, height
    - location: city, country, region
    - interests: activity types
    - settings: dynamic settings (notifications, language, theme)
    - profile picture

Use case:
- Get user profile
- Update user profile
- Delete user profile
- Set Location

Doest not know about: activities, groups and events.

***Activity Catalog Context***

Responsibility:
- Types of activities available

Aggregates:
- ActivityType (name, description) - Basketball, Foortbal, Running, Hiking..

Use case:
- List activity types
- Create activity type - admin

Does not not know about: users, groups and events.

***Activity Group/Event Context - CORE***

Responsibility:
- Organizing and managing activity groups/events.

Aggregates:
- Activity Group
    - Activity Group Metadata (name, location, type...)
    - Visibility (public/private)
    - Members (who is in the group)
- Game Session
    - Time and date
    - Capacity
    - Paricipants
    - Status (open, full, completed)

Use case:
- Create activity group
- Update activity group
- Join/Leave group
- Accept / Decline invitation
- List activity groups
- List activity group members
- Manage capacity (first 6 play)

***Discovery Context***

Responsibility:
- Search for activity groups/events

Aggregates:
- GroupListing (read model for search)

Use case:
- Search for activity groups
- Search for activity groups by location
- Search for activity groups by type

Note: This could be a separate read model (CQRS pattern).