package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrInvalidRefreshToken is returned when the refresh token is unknown or expired.
var ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")

// RefreshTokenDoc is a stored refresh session (hashed token only).
type RefreshTokenDoc struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	UserID    primitive.ObjectID `bson:"user_id"`
	TokenHash string             `bson:"token_hash"`
	FamilyID  string             `bson:"family_id"`
	ExpiresAt time.Time          `bson:"expires_at"`
	CreatedAt time.Time          `bson:"created_at"`
}

type RefreshTokenRepository struct {
	col *mongo.Collection
}

func NewRefreshTokenRepository(db *mongo.Database) *RefreshTokenRepository {
	col := db.Collection("refresh_tokens")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "token_hash", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0),
		},
		{Keys: bson.D{{Key: "user_id", Value: 1}}},
	})

	return &RefreshTokenRepository{col: col}
}

// Insert stores a new refresh session.
func (r *RefreshTokenRepository) Insert(ctx context.Context, userID primitive.ObjectID, tokenHash, familyID string, expiresAt time.Time) error {
	doc := RefreshTokenDoc{
		ID:        primitive.NewObjectID(),
		UserID:    userID,
		TokenHash: tokenHash,
		FamilyID:  familyID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	_, err := r.col.InsertOne(ctx, doc)
	return err
}

// ConsumeAndRotate validates the presented hash, then atomically replaces it with newHash on the same
// document (same user_id, family_id, _id). If the update fails (e.g. duplicate newHash or DB error),
// the original refresh row is unchanged so the client can retry with the same refresh token.
func (r *RefreshTokenRepository) ConsumeAndRotate(ctx context.Context, presentedHash, newHash string, newExpires time.Time) (*RefreshTokenDoc, error) {
	now := time.Now()
	opts := options.FindOneAndUpdate().
		SetReturnDocument(options.After)

	var doc RefreshTokenDoc
	err := r.col.FindOneAndUpdate(ctx,
		bson.M{
			"token_hash": presentedHash,
			"expires_at": bson.M{"$gt": now},
		},
		bson.M{
			"$set": bson.M{
				"token_hash":  newHash,
				"expires_at":  newExpires,
				"created_at":  now,
			},
		},
		opts,
	).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, err
	}
	return &RefreshTokenDoc{
		UserID:    doc.UserID,
		FamilyID:  doc.FamilyID,
		TokenHash: presentedHash,
	}, nil
}

// DeleteByHash removes a session (logout).
func (r *RefreshTokenRepository) DeleteByHash(ctx context.Context, tokenHash string) (bool, error) {
	res, err := r.col.DeleteOne(ctx, bson.M{
		"token_hash": tokenHash,
		"expires_at": bson.M{"$gt": time.Now()},
	})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// DeleteAllForUser removes every refresh session for a user (password reset, account deletion).
func (r *RefreshTokenRepository) DeleteAllForUser(ctx context.Context, userID primitive.ObjectID) (int64, error) {
	res, err := r.col.DeleteMany(ctx, bson.M{"user_id": userID})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
