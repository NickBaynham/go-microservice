package repository

import (
	"context"
	"errors"
	"log"
	"time"

	"go-microservice/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrDuplicateEmail is returned when inserting a user whose email already exists.
var ErrDuplicateEmail = errors.New("email already exists")

type UserRepository struct {
	col *mongo.Collection
}

func NewUserRepository(db *mongo.Database) *UserRepository {
	col := db.Collection("users")

	// Unique index on email
	idx := mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := col.Indexes().CreateOne(ctx, idx); err != nil {
		log.Printf("user repository: failed to ensure unique email index: %v", err)
	}

	return &UserRepository{col: col}
}

// Count returns the number of users in the collection.
func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	return r.col.CountDocuments(ctx, bson.M{})
}

func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	user.ID = primitive.NewObjectID()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	if user.Role == "" {
		user.Role = "user"
	}
	_, err := r.col.InsertOne(ctx, user)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateEmail
	}
	return err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*models.User, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errors.New("invalid user id")
	}
	var user models.User
	err = r.col.FindOne(ctx, bson.M{"_id": oid}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, errors.New("user not found")
	}
	return &user, err
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.col.FindOne(ctx, bson.M{"email": email}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, errors.New("user not found")
	}
	return &user, err
}

func (r *UserRepository) FindAll(ctx context.Context) ([]models.User, error) {
	cursor, err := r.col.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var users []models.User
	return users, cursor.All(ctx, &users)
}

func (r *UserRepository) Update(ctx context.Context, id string, update bson.M) (*models.User, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, errors.New("invalid user id")
	}
	update["updated_at"] = time.Now()
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var user models.User
	err = r.col.FindOneAndUpdate(ctx,
		bson.M{"_id": oid},
		bson.M{"$set": update},
		opts,
	).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, errors.New("user not found")
	}
	if mongo.IsDuplicateKeyError(err) {
		return nil, ErrDuplicateEmail
	}
	return &user, err
}

// IncrementFailedLogin increments failed_login_attempts and sets locked_until when attempts reach maxAttempts.
func (r *UserRepository) IncrementFailedLogin(ctx context.Context, id string, maxAttempts int, lockout time.Duration) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return errors.New("invalid user id")
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var user models.User
	err = r.col.FindOneAndUpdate(ctx,
		bson.M{"_id": oid},
		bson.M{
			"$inc": bson.M{"failed_login_attempts": 1},
			"$set": bson.M{"updated_at": time.Now()},
		},
		opts,
	).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return errors.New("user not found")
	}
	if err != nil {
		return err
	}
	if maxAttempts > 0 && user.FailedLoginAttempts >= maxAttempts {
		until := time.Now().Add(lockout)
		_, err = r.col.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{
			"$set": bson.M{"locked_until": until, "updated_at": time.Now()},
		})
		return err
	}
	return nil
}

// ClearLoginLockout resets failed-login state after a successful login or password reset.
func (r *UserRepository) ClearLoginLockout(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return errors.New("invalid user id")
	}
	_, err = r.col.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{
		"$set": bson.M{
			"failed_login_attempts": 0,
			"updated_at":            time.Now(),
		},
		"$unset": bson.M{"locked_until": ""},
	})
	return err
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return errors.New("invalid user id")
	}
	res, err := r.col.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return errors.New("user not found")
	}
	return nil
}
