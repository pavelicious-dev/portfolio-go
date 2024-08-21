package database

import (
	"context"
	"fmt"
	"portfolio/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func GetTrades(symbol string) ([]models.Trade, error) {
	filter := bson.M{"asset": bson.M{"$regex": "^" + symbol}}
	opts := options.Find().SetSort(bson.D{{Key: "date", Value: 1}})
	res, err := GetDB().Collection("trades").Find(context.Background(), filter, opts)

	if err != nil {
		return nil, err
	}

	var trades []models.Trade

	if err = res.All(context.Background(), &trades); err != nil {
		return nil, err
	}

	return trades, nil
}

func ImportTrades(trades []models.Trade) error {
	db := GetDB()
	collection := db.Collection("trades")

	collection.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.M{
			"transactionId": 1,
		},
		Options: options.Index().SetUnique(true),
	})

	tradeDocs := make([]interface{}, len(trades))
	for i, trade := range trades {
		tradeDocs[i] = trade
	}

	opt := options.InsertManyOptions{}
	res, err := collection.InsertMany(context.Background(), tradeDocs, opt.SetOrdered(false))
	if err != nil {
		return err
	}

	fmt.Printf(" - inserted %d new trades\n", len(res.InsertedIDs))

	return nil
}
