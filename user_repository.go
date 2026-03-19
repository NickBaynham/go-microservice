package repository

import (
	"context"
	"errors"
	"time"

	"github.com/example/user-service/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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
	col.Indexes().CreateOne(ctx, idx) //nolint:errcheck

	return &UserRepository{col: col}
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
		return errors.New("email already exists")
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
	return &user, err
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
