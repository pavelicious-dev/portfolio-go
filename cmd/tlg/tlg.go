package main

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"portfolio/database"
	"portfolio/models"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func ParseTlgFile(path string) ([]models.Trade, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)

	trades := []models.Trade{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if slices.ContainsFunc([]string{"STK_TRD", "OPT_TRD", "FOP_TRD"}, func(s string) bool { return strings.HasPrefix(line, s) }) {
			trade, err := ParseTrade(line)
			if err != nil {
				return nil, err
			}
			trades = append(trades, trade)
		}
	}
	return trades, nil
}

func ParseTrade(line string) (models.Trade, error) {
	parts := strings.Split(line, "|")

	var asset = parts[2]

	isOption := slices.Contains([]string{"OPT_TRD", "FOP_TRD"}, parts[0])
	if isOption {
		asset = parts[3]
	}

	date, _ := time.Parse("20060102 15:04:05", parts[7]+" "+parts[8])
	quantity, _ := strconv.ParseFloat(parts[10], 64)
	multiplier, _ := strconv.ParseInt(parts[11], 10, 0)
	price, _ := strconv.ParseFloat(parts[12], 64)
	commission, _ := strconv.ParseFloat(parts[14], 64)
	rate, _ := strconv.ParseFloat(parts[15], 64)

	return models.Trade{
		TransactionId: parts[1],
		Asset:         asset,
		Date:          date,
		Units:         quantity,
		UnitPrice:     math.Abs(price / quantity),
		Commission:    commission,
		Total:         quantity*price + commission,
		ExchangeRate:  rate,
		Multiplier:    multiplier,
		IsOptionTrade: isOption,
	}, nil
}

func main() {
	importTrades(os.Args[1])
}

func importTrades(path string) {
	godotenv.Load()

	database.Connect()
	defer database.Close()

	files, err := os.ReadDir(path)
	if err != nil {
		panic(err)
	}

	db := database.GetDB()
	collection := db.Collection("trades")

	collection.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.M{
			"transactionId": 1,
		},
		Options: options.Index().SetUnique(true),
	})

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tlg") {
			trades, _ := ParseTlgFile(path + string(os.PathSeparator) + file.Name())
			fmt.Printf("%s (%d trades)", file.Name(), len(trades))

			tradeDocs := make([]interface{}, len(trades))
			for i, trade := range trades {
				tradeDocs[i] = trade
			}

			opt := options.InsertManyOptions{}
			res, _ := collection.InsertMany(context.Background(), tradeDocs, opt.SetOrdered(false))
			fmt.Printf(" - inserted %d new trades\n", len(res.InsertedIDs))
		}
	}
}
