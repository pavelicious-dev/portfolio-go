package models

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

type Trade struct {
	TransactionId string `bson:"transactionId"`
	Asset         string
	Date          time.Time
	Units         float64
	UnitPrice     float64 `bson:"unitPrice"`
	Commission    float64
	Total         float64
	ExchangeRate  float64 `bson:"exchangeRate"`
	Multiplier    int64
	IsOptionTrade bool `bson:"isOptionTrade"`
}

func (t Trade) GetOptionInfo() (*OptionInfo, error) {
	if !t.IsOptionTrade {
		return nil, errors.New("not an option trade")
	}
	parts := strings.Fields(t.Asset)
	expiration, _ := time.Parse("20060102", parts[1])

	strike, _ := strconv.ParseFloat(parts[2], 64)

	var optionType OptionType

	switch parts[3] {
	case "CALL":
		optionType = Call
	case "PUT":
		optionType = Put
	}

	return &OptionInfo{
		Symbol:      parts[0],
		Expiration:  expiration,
		StrikePrice: strike,
		Type:        optionType,
	}, nil
}

type OptionType string

const (
	Call OptionType = "CALL"
	Put  OptionType = "PUT"
)

type OptionInfo struct {
	Symbol      string
	StrikePrice float64
	Expiration  time.Time
	Type        OptionType
}
