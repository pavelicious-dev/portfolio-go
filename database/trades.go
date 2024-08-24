package database

import (
	"context"
	"fmt"
	"portfolio/models"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TradeQuery struct {
	Symbol string
	Year   string
	Month  string
}

func GetTrades(query TradeQuery) ([]models.Trade, error) {
	filter := bson.M{}

	if query.Symbol != "" {
		filter["asset"] = bson.M{"$regex": "^" + strings.ToUpper(query.Symbol)}
	}

	dateQuery := bson.A{}

	if query.Year != "" {
		year, err := strconv.ParseInt(query.Year, 10, 32)
		if err == nil {
			dateQuery = append(dateQuery, bson.M{"$expr": bson.M{"$eq": bson.A{bson.M{"$year": "$date"}, year}}})
		}
	}

	if query.Month != "" {
		month, err := strconv.ParseInt(query.Month, 10, 32)
		if err == nil {
			dateQuery = append(dateQuery, bson.M{"$expr": bson.M{"$eq": bson.A{bson.M{"$month": "$date"}, month}}})
		}
	}

	if len(dateQuery) > 0 {
		filter["$and"] = dateQuery
	}

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
