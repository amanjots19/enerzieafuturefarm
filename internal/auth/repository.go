package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Collection and field names, per schema.md.
const (
	usersCollection    = "users"
	otpCodesCollection = "otp_codes"

	fieldID        = "_id"
	fieldPhone     = "phone"
	fieldCreatedAt = "createdAt"
)

// ErrUserNotFound is returned when a token names a user who no longer exists.
var ErrUserNotFound = errors.New("auth: user not found")

// ErrCodeNotFound is returned when a phone number has no pending code.
var ErrCodeNotFound = errors.New("auth: no pending code")

// Repository stores users and one-time codes.
type Repository struct {
	users *mongo.Collection
	codes *mongo.Collection
}

// NewRepository builds the auth repository over the given database.
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		users: db.Collection(usersCollection),
		codes: db.Collection(otpCodesCollection),
	}
}

// CreateCode stores a pending challenge.
func (r *Repository) CreateCode(ctx context.Context, code OTPCode) error {
	if _, err := r.codes.InsertOne(ctx, code); err != nil {
		return fmt.Errorf("auth: create code: %w", err)
	}
	return nil
}

// LatestCode returns the newest code for a phone number, whether or not it is
// still usable — the caller decides that, so a spent or locked code produces
// the right message instead of looking like no code at all.
func (r *Repository) LatestCode(ctx context.Context, phone string) (OTPCode, error) {
	var code OTPCode
	err := r.codes.FindOne(ctx,
		bson.D{{Key: fieldPhone, Value: phone}},
		options.FindOne().SetSort(bson.D{{Key: fieldCreatedAt, Value: -1}}),
	).Decode(&code)

	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return OTPCode{}, ErrCodeNotFound
	case err != nil:
		return OTPCode{}, fmt.Errorf("auth: latest code: %w", err)
	}
	return code, nil
}

// CountCodesSince reports how many codes a number has requested in a window.
// It backs the rate limit.
func (r *Repository) CountCodesSince(ctx context.Context, phone string, since time.Time) (int64, error) {
	n, err := r.codes.CountDocuments(ctx, bson.D{
		{Key: fieldPhone, Value: phone},
		{Key: fieldCreatedAt, Value: bson.D{{Key: "$gte", Value: since}}},
	})
	if err != nil {
		return 0, fmt.Errorf("auth: count recent codes: %w", err)
	}
	return n, nil
}

// RecordFailedAttempt increments a code's attempt counter. Counting in the
// database rather than in memory means the lockout survives a restart and
// holds across replicas.
func (r *Repository) RecordFailedAttempt(ctx context.Context, id bson.ObjectID) error {
	_, err := r.codes.UpdateByID(ctx, id, bson.D{
		{Key: "$inc", Value: bson.D{{Key: "attempts", Value: 1}}},
	})
	if err != nil {
		return fmt.Errorf("auth: record failed attempt: %w", err)
	}
	return nil
}

// ConsumeCode marks a code spent so it cannot be replayed.
//
// The filter includes consumed:false, so two concurrent verifications of the
// same code cannot both succeed — the second matches nothing.
func (r *Repository) ConsumeCode(ctx context.Context, id bson.ObjectID) (bool, error) {
	res, err := r.codes.UpdateOne(ctx,
		bson.D{{Key: fieldID, Value: id}, {Key: "consumed", Value: false}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "consumed", Value: true}}}},
	)
	if err != nil {
		return false, fmt.Errorf("auth: consume code: %w", err)
	}
	return res.ModifiedCount == 1, nil
}

// UpsertUser returns the user for a phone number, creating them on first
// sign-in. The unique index on phone makes this safe under a race: a losing
// concurrent insert fails and the retry finds the winner's document.
func (r *Repository) UpsertUser(ctx context.Context, phone string, now time.Time) (User, error) {
	update := bson.D{
		{Key: "$set", Value: bson.D{{Key: "updatedAt", Value: now}}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: fieldPhone, Value: phone},
			{Key: fieldCreatedAt, Value: now},
		}},
	}

	var user User
	err := r.users.FindOneAndUpdate(ctx,
		bson.D{{Key: fieldPhone, Value: phone}}, update,
		options.FindOneAndUpdate().
			SetUpsert(true).
			SetReturnDocument(options.After),
	).Decode(&user)
	if err != nil {
		return User{}, fmt.Errorf("auth: upsert user: %w", err)
	}
	return user, nil
}

// UserByID loads a user named by a token.
func (r *Repository) UserByID(ctx context.Context, id bson.ObjectID) (User, error) {
	var user User
	err := r.users.FindOne(ctx, bson.D{{Key: fieldID, Value: id}}).Decode(&user)

	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return User{}, ErrUserNotFound
	case err != nil:
		return User{}, fmt.Errorf("auth: get user: %w", err)
	}
	return user, nil
}

// EnsureIndexes creates the indexes schema.md specifies for auth.
//
// Both matter for correctness, not speed: the unique phone index is what makes
// the user upsert race-safe, and the TTL index is what actually deletes spent
// codes — there is no cleanup job.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	if _, err := r.users.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: fieldPhone, Value: 1}},
		Options: options.Index().SetName("phone_1").SetUnique(true),
	}); err != nil {
		return fmt.Errorf("auth: ensure users indexes: %w", err)
	}

	_, err := r.codes.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "expiresAt", Value: 1}},
			Options: options.Index().SetName("expiresAt_ttl").SetExpireAfterSeconds(0),
		},
		{
			Keys:    bson.D{{Key: fieldPhone, Value: 1}, {Key: fieldCreatedAt, Value: -1}},
			Options: options.Index().SetName("phone_1_createdAt_-1"),
		},
	})
	if err != nil {
		return fmt.Errorf("auth: ensure otp indexes: %w", err)
	}
	return nil
}
