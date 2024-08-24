package main

import (
	"portfolio/database"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	database.Connect()
	defer database.Close()

	app := fiber.New()

	app.Get("/trades", func(c fiber.Ctx) error {
		trades, err := database.GetTrades(database.TradeQuery{
			Symbol: c.Query("symbol"),
			Year:   c.Query("year"),
			Month:  c.Query("month"),
		})

		if err != nil {
			log.Error(err)
			return c.SendStatus(fiber.StatusBadRequest)
		}
		return c.JSON(trades)
	})

	app.Get("/trades/:year/:month", func(c fiber.Ctx) error {

		return nil
	})

	log.Fatal(app.Listen(":3000"))
}
