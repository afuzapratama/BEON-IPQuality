package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCheckIPFailsClosedWithoutMMDB(t *testing.T) {
	SetMMDBReader(nil)
	SetCache(nil)
	app := fiber.New()
	app.Get("/check/:ip", CheckIP())

	response, err := app.Test(httptest.NewRequest("GET", "/check/8.8.8.8", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.StatusCode, fiber.StatusServiceUnavailable)
	}
}

func TestHealthFailsWithoutMMDB(t *testing.T) {
	SetMMDBReader(nil)
	app := fiber.New()
	app.Get("/health", HealthCheck("test"))
	response, err := app.Test(httptest.NewRequest("GET", "/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.StatusCode, fiber.StatusServiceUnavailable)
	}
}
