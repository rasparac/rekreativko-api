package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDOB(t *testing.T) *DateOfBirth {
	t.Helper()

	dob, err := NewDateOfBirth(time.Now().AddDate(-20, 0, 0))
	require.NoError(t, err)

	return dob
}

func validNickname(t *testing.T) *Nickname {
	n, err := NewNickname("john_doe")
	require.NoError(t, err)
	return n
}

func validLocation(t *testing.T) *Location {
	loc, err := NewLocation("Berlin", "DE", nil, nil)
	require.NoError(t, err)
	return loc
}

func TestNewUserProfile(t *testing.T) {
	id := uuid.New()

	up := NewUserProfile(id)

	require.NotNil(t, up)
	assert.Equal(t, id, up.ID())
	assert.False(t, up.IsDeleted())
	assert.NotZero(t, up.CreatedAt())
	assert.NotZero(t, up.UpdatedAt())
	assert.Empty(t, up.ActivityInterests())
	assert.Len(t, up.Events(), 1)
}

func TestUpdateProfile_Success(t *testing.T) {
	up := NewUserProfile(uuid.New())

	fullName, err := NewFullName("John Doe")
	require.NoError(t, err)

	err = up.UpdateProfile(fullName, validDOB(t), "hello bio")

	require.NoError(t, err)
	assert.Equal(t, "hello bio", up.Bio())
	assert.Equal(t, "John Doe", up.FullName().Value())
}

func TestUpdateProfile_BioTooLong(t *testing.T) {
	up := NewUserProfile(uuid.New())

	longBio := make([]byte, bioMaxLength+1)
	err := up.UpdateProfile(nil, nil, string(longBio))

	assert.ErrorIs(t, err, ErrUserProfileBioTooLong)
}

func TestUpdateProfile_Deleted(t *testing.T) {
	up := NewUserProfile(uuid.New())
	require.NoError(t, up.Delete())

	err := up.UpdateProfile(nil, nil, "bio")

	assert.ErrorIs(t, err, ErrUserProfileDeleted)
}

func TestSetNickname(t *testing.T) {
	up := NewUserProfile(uuid.New())
	require.NoError(t, up.SetNickname(validNickname(t)))

	newNick, _ := NewNickname("johnny")

	err := up.SetNickname(newNick)

	require.NoError(t, err)
	assert.Equal(t, "johnny", up.Nickname().Value())
}

func TestSetNickname_Nil(t *testing.T) {
	up := NewUserProfile(uuid.New())
	require.NoError(t, up.SetNickname(validNickname(t)))

	err := up.SetNickname(nil)

	assert.ErrorIs(t, err, ErrInvalidNickname)
}

func TestSetLocation(t *testing.T) {
	up := NewUserProfile(uuid.New())
	up.location = validLocation(t)

	newLoc, err := NewLocation("Munich", "DE", nil, nil)
	require.NoError(t, err)

	err = up.SetLocation(newLoc)

	require.NoError(t, err)
	assert.Equal(t, "Munich", up.Location().City())
	assert.Len(t, up.Events(), 2)
}

func TestAddActivityInterest(t *testing.T) {
	up := NewUserProfile(uuid.New())

	interest, err := NewActivityInterest(ActivityTypeRunning, ActivityLevelBeginner)
	require.NoError(t, err)

	err = up.AddactivityInterest(interest)

	require.NoError(t, err)
	assert.Len(t, up.ActivityInterests(), 1)
	assert.True(t, up.HasActivityInterest(ActivityTypeRunning))
}

func TestAddActivityInterest_Duplicate(t *testing.T) {
	up := NewUserProfile(uuid.New())

	interest, _ := NewActivityInterest(ActivityTypeRunning, ActivityLevelBeginner)
	require.NoError(t, up.AddactivityInterest(interest))

	err := up.AddactivityInterest(interest)

	assert.ErrorIs(t, err, ErrDuplicateInterests)
}

func TestRemoveActivityInterest(t *testing.T) {
	up := NewUserProfile(uuid.New())

	interest, _ := NewActivityInterest(ActivityTypeRunning, ActivityLevelBeginner)
	require.NoError(t, up.AddactivityInterest(interest))

	err := up.RemoveActivityInterest(ActivityTypeRunning)

	require.NoError(t, err)
	assert.False(t, up.HasActivityInterest(ActivityTypeRunning))
}

func TestUpdateActivityInterestLevel(t *testing.T) {
	up := NewUserProfile(uuid.New())

	interest, _ := NewActivityInterest(ActivityTypeRunning, ActivityLevelBeginner)
	require.NoError(t, up.AddactivityInterest(interest))

	err := up.UpdateAcitivityInterestLevel(ActivityTypeRunning, ActivityLevelAdvanced)

	require.NoError(t, err)

	updated, err := up.GetActivityInterest(ActivityTypeRunning)
	require.NoError(t, err)
	assert.Equal(t, ActivityLevelAdvanced, updated.Level())
}

func TestDeleteUserProfile(t *testing.T) {
	up := NewUserProfile(uuid.New())

	err := up.Delete()

	require.NoError(t, err)
	assert.True(t, up.IsDeleted())
	assert.NotNil(t, up.DeleteAt())
}

func TestAnonymize(t *testing.T) {
	up := NewUserProfile(uuid.New())
	require.NoError(t, up.SetNickname(validNickname(t)))
	require.NoError(t, up.Delete())

	err := up.Anonymize()

	require.NoError(t, err)
	assert.NotNil(t, up.Nickname())
	assert.Nil(t, up.FullName())
	assert.Nil(t, up.ProfilePicture())
	assert.Nil(t, up.DateOfBirth())
	assert.Empty(t, up.Bio())
	assert.Empty(t, up.ActivityInterests())
}

func TestAnonymize_NotDeleted(t *testing.T) {
	up := NewUserProfile(uuid.New())

	err := up.Anonymize()

	assert.ErrorIs(t, err, ErrUserProfileDeleted)
}

func TestClearEvents(t *testing.T) {
	up := NewUserProfile(uuid.New())

	require.NotEmpty(t, up.Events())

	up.ClearEvents()

	assert.Empty(t, up.Events())
}
