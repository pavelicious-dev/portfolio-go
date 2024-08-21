package main

import (
	"fmt"
	"portfolio/database"
	"portfolio/models"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	database.Connect()
	defer database.Close()

	app := fiber.New()

	app.Get("/trades/:symbol", func(c fiber.Ctx) error {
		symbol := strings.ToUpper(c.Params("symbol"))

		trades, err := database.GetTrades(symbol)
		if err != nil {
			log.Error(err)
			return c.SendStatus(fiber.StatusBadRequest)
		}

		filtered := []models.Trade{}
		for _, trade := range trades {
			if trade.IsOptionTrade {
				fmt.Println(trade.GetOptionInfo())
				filtered = append(filtered, trade)
			}
		}

		return c.JSON(filtered)
	})

	log.Fatal(app.Listen(":3000"))
}
