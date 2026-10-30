package store

import (
	"context"
	"time"

	"carcasever/internal/game"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Mongo struct {
	accounts *mongo.Collection
	games    *mongo.Collection
	scores   *mongo.Collection
}

func Connect(ctx context.Context, uri, dbName string) (*Mongo, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetServerSelectionTimeout(8*time.Second))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database(dbName)
	m := &Mongo{
		accounts: db.Collection("accounts"),
		games:    db.Collection("games"),
		scores:   db.Collection("scores"),
	}
	_, _ = m.accounts.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "token", Value: 1}}, Options: options.Index().SetUnique(true),
	})
	_, _ = m.accounts.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "login", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true),
	})
	_, _ = m.games.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "code", Value: 1}}, Options: options.Index().SetUnique(true),
	})
	return m, nil
}

func (m *Mongo) PutAccount(ctx context.Context, a Account) error {
	_, err := m.accounts.ReplaceOne(ctx, bson.M{"_id": a.ID}, a, options.Replace().SetUpsert(true))
	return err
}

func (m *Mongo) AccountByToken(ctx context.Context, token string) (Account, error) {
	var a Account
	err := m.accounts.FindOne(ctx, bson.M{"token": token}).Decode(&a)
	if err == mongo.ErrNoDocuments {
		return Account{}, ErrNotFound
	}
	return a, err
}

func (m *Mongo) AccountByID(ctx context.Context, id string) (Account, error) {
	var a Account
	err := m.accounts.FindOne(ctx, bson.M{"_id": id}).Decode(&a)
	if err == mongo.ErrNoDocuments {
		return Account{}, ErrNotFound
	}
	return a, err
}

func (m *Mongo) AccountByLogin(ctx context.Context, login string) (Account, error) {
	var a Account
	err := m.accounts.FindOne(ctx, bson.M{"login": login}).Decode(&a)
	if err == mongo.ErrNoDocuments {
		return Account{}, ErrNotFound
	}
	return a, err
}

func (m *Mongo) PutGame(ctx context.Context, g *game.State) error {
	_, err := m.games.ReplaceOne(ctx, bson.M{"_id": g.ID}, g, options.Replace().SetUpsert(true))
	return err
}

func (m *Mongo) Game(ctx context.Context, id string) (*game.State, error) {
	var g game.State
	err := m.games.FindOne(ctx, bson.M{"_id": id}).Decode(&g)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (m *Mongo) GameByCode(ctx context.Context, code string) (*game.State, error) {
	var g game.State
	err := m.games.FindOne(ctx, bson.M{"code": code}).Decode(&g)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (m *Mongo) OpenLobbies(ctx context.Context) ([]*game.State, error) {
	cur, err := m.games.Find(ctx, bson.M{
		"open":   true,
		"status": bson.M{"$in": []string{game.StatusLobby, game.StatusPlace, game.StatusMeeple}},
	}, options.Find().SetSort(bson.D{{Key: "updatedAt", Value: -1}}).SetLimit(50))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []*game.State
	for cur.Next(ctx) {
		var g game.State
		if err := cur.Decode(&g); err != nil {
			return nil, err
		}
		out = append(out, &g)
	}
	return out, cur.Err()
}

func (m *Mongo) LiveGames(ctx context.Context) ([]*game.State, error) {
	cur, err := m.games.Find(ctx, bson.M{
		"status": bson.M{"$in": []string{game.StatusPlace, game.StatusMeeple}},
	})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []*game.State
	for cur.Next(ctx) {
		var g game.State
		if err := cur.Decode(&g); err != nil {
			return nil, err
		}
		out = append(out, &g)
	}
	return out, cur.Err()
}

func (m *Mongo) DeleteGame(ctx context.Context, id string) error {
	_, err := m.games.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (m *Mongo) AddScore(ctx context.Context, id, name string, delta int) error {
	_, err := m.scores.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$inc": bson.M{"score": delta},
		"$set": bson.M{"name": name},
	}, options.Update().SetUpsert(true))
	return err
}

func (m *Mongo) Leaderboard(ctx context.Context, limit int) ([]Standing, error) {
	if limit <= 0 {
		limit = 20
	}
	cur, err := m.scores.Find(ctx, bson.M{"score": bson.M{"$gt": 0}},
		options.Find().SetSort(bson.D{{Key: "score", Value: -1}, {Key: "name", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []Standing
	for cur.Next(ctx) {
		var row Standing
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if out == nil {
		out = []Standing{}
	}
	return out, cur.Err()
}
