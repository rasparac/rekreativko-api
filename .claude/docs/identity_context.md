Identity Context Design

1. User Registration (email or phone number)
2. Login/Logout
3. JWT token generation and validation
4. Refresh Token Management
5. Password Reset

Does not handle:
- User Profiles (nicname, city, etc...)
- User Prefrences
- Authorization Rules


Aggregates:

```
type Account struct {
    ID                  uuid.UUID
    PhoneNumber         *string  // NULLABLE - either email or phone required, not both
    Email               *string  // NULLABLE - either email or phone required, not both
    PasswordHash        string
    Status              AccountStatus
    FailedLoginAttempts int
    LockedUntil         *time.Time

    CreatedAt   time.Time
    UpdatedAt   time.Time
    DeletedAt   *time.Time  // soft delete
}

type AccountStatus string

const (
    AccountStatusPending AccountStatus = "pending" // not verified
    AccountStatusActive  AccountStatus = "active"
    AccountStatusSuspended AccountStatus = "suspended"
    AccountStatusDeleted AccountStatus = "deleted"
)

type RefreshToken struct {
    ID          uuid.UUID
    AccountID   uuid.UUID
    Token       string  // stores full token (hashed in practice)
    ExpiresAt   time.Time

    CreatedAt   time.Time
    RevokedAt   *time.Time
}


type VerificationCode struct {
    ID          uuid.UUID
    AccountID   uuid.UUID
    Code        string // 6 digits
    Type        VerificationCodeType
    ExpiresAt   time.Time
    CreatedAt   time.Time
    UsedAt      *time.Time
}

type VerificationCodeType string

const (
    VerificationCodeTypeEmail VerificationCodeType = "email"
    VerificationCodeTypePhone VerificationCodeType = "phone"
)
```

Business Rules:

***Registration Rules***
1. Email or Phone (not both required)
- register with email
- register with phone number
- register with email and phone number - optional

2. Uniqueness
- email must be unique across all accounts
- phone number must be unique across all accounts
- cannot register with existing email
- cannot register with existing phone numb

3. Password Requirements
- minimum 8 characters
- at least one uppercase letter
- at least one lowercase letter
- at least one number
- at least one special character

4. Verification Flow
```
User registers  -> Account Created (status pending)
                -> Verification code sent
                -> User enters verification code
                -> Account Activated (status active)
```

Login Rules:

1. Login methods:
- login with email
- login with phone number
- no login for suspended accounts
- no login for deleted accounts
- no login with unverified accounts

2. Token Generation

```
User logs in    -> Generate Access Token (JWT, 15 min)
                -> Generate Refresh Token (JWT, 30 days)
                -> Store Refresg token hash in DB
```

3. Failed Login Attempts
- track failed  login attempts
- after 5 failed attempts, lock account for 15 min
- after 10 failed attempts, suspend account

***Token rules***

1. Access token (JWT)
- sub: account id
- exp: 15 min
- iat: current time
- iss: reakreativko

Used for: Every API Request
Storage: Clinet-side (memory or httpOnly cookie)

2. Refresh token (JWT)



3. Token refresh flow
```
Access token expires    -> Client sends refresh token
                        -> Validate refresh token
                        -> Generate new access token
                        -> Generate new refresh token
                        -> Revoke old refresh token
                        -> Store new refresh token hash in DB

```

Database schema:

All tables use the `identity` schema for namespace isolation (DDD bounded context).

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;
CREATE EXTENSION IF NOT EXISTS citext WITH SCHEMA public;

CREATE SCHEMA IF NOT EXISTS "identity";

-- Accounts table
CREATE TABLE IF NOT EXISTS identity.accounts(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    email CITEXT DEFAULT NULL,           -- NULLABLE - either email or phone required
    phone_number CITEXT DEFAULT NULL,    -- NULLABLE - either email or phone required
    password text NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'pending',
    failed_login_attempts int NOT NULL DEFAULT 0,
    locked_until timestamptz DEFAULT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    deleted_at timestamptz DEFAULT NULL
);

-- Unique constraints on email/phone only for non-deleted accounts
CREATE UNIQUE INDEX accounts_phonenumber_uq_idx ON identity.accounts(phone_number)
WHERE (deleted_at IS NULL);

CREATE UNIQUE INDEX accounts_email_uq_idx ON identity.accounts(email)
WHERE (deleted_at IS NULL);

CREATE INDEX accounts_status_idx ON identity.accounts(status);
CREATE INDEX accounts_updated_at_idx ON identity.accounts(updated_at);
CREATE INDEX accounts_created_at_idx ON identity.accounts(created_at);

-- Refresh tokens table
CREATE TABLE IF NOT EXISTS identity.refresh_tokens(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id uuid NOT NULL,
    token text NOT NULL,  -- stores full token
    created_at timestamptz NOT NULL DEFAULT NOW(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz DEFAULT NULL
);

ALTER TABLE identity.refresh_tokens
    ADD CONSTRAINT fk_account_id FOREIGN KEY (account_id)
    REFERENCES identity.accounts(id) ON DELETE CASCADE;

CREATE INDEX refresh_tokens_account_id_idx ON identity.refresh_tokens(account_id);
CREATE UNIQUE INDEX refresh_tokens_token_uq_idx ON identity.refresh_tokens(token);
CREATE INDEX refresh_tokens_created_at_idx ON identity.refresh_tokens(created_at);
CREATE INDEX refresh_tokens_revoked_at_idx ON identity.refresh_tokens(revoked_at)
WHERE (revoked_at IS NOT NULL);
CREATE INDEX refresh_tokens_expires_at_idx ON identity.refresh_tokens(expires_at);

-- Verification codes table
CREATE TABLE IF NOT EXISTS identity.verification_codes(
    id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
    account_id uuid NOT NULL,
    code varchar(6) NOT NULL,
    type varchar(20) NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    used_at timestamptz DEFAULT NULL
);

ALTER TABLE identity.verification_codes
    ADD CONSTRAINT fk_account_id FOREIGN KEY (account_id)
    REFERENCES identity.accounts(id) ON DELETE CASCADE;

CREATE INDEX verification_codes_account_id_idx ON identity.verification_codes(account_id);
CREATE UNIQUE INDEX verification_codes_account_code_uq_idx ON identity.verification_codes(code);
CREATE INDEX verification_codes_expires_at_idx ON identity.verification_codes(expires_at);

-- Trigger to auto-update updated_at timestamp
CREATE OR REPLACE FUNCTION identity.update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE TRIGGER update_updated_at_column
    BEFORE UPDATE ON identity.accounts
    FOR EACH ROW
    EXECUTE PROCEDURE identity.update_updated_at_column();
```

***API Contract***

API will use application/json content type


1. Register
```
POST /api/v1/auth/register

{
    "identifier": "+1234567890" or "WYc0w@example.com",
    "password": "mysecretpassword"
}

Response:
201 Created

400 Bad Request:
{
    "error": "invalid request"
    "message": "todo"
}


```
2. Verify Account
```
POST /api/v1/auth/verify

{
    "identifier": "+1234567890" or "WYc0w@example.com",
    "code": "123456"
}

Response:
200 OK
400 Bad Request
500 Internal server error

```
3. Resend Verification Code
```
POST /api/v1/auth/verify/resend

{
    "phone_number": "+1234567890",
    "email": "WYc0w@example.com"
}

Response:
200 OK
400 Bad Request
500 Internal server error

```
4. Login
```
POST /api/v1/auth/login

{
    "identifier": "+1234567890" or "WYc0w@example.com",
    "password": "mysecretpassword"
}

Response:
200 OK
401 Unauthorized
403 Forbidden
423 Locked
500 Internal server error

```
5. Refresh Token
```
POST /api/v1/auth/refresh

{
    "refresh_token": "token"
}

Response:
200 OK
401 Unauthorized
500 Internal server error
```
6. Logout
```
POST /api/v1/auth/logout

{
    "refresh_token": "token"
}

Response:
200 OK
500 Internal server error
```


***Security***

1. HTTPS
2. Token rotation
- refresh tokens are one-time use
- old token revoked when new one issued
3. Security Headers
4. Rate Limiting
- account registration - 5 per hour per IP
- login - 10 per hour per IP
- verification - 5 per hour per account
5. Storage - password and token
- access token: client memory (or httpOnly cookie), Authorization header
- refresh token: httpOnly, secure, sameSit cookie
- password: database, bycrypt


***Testing***

TODO