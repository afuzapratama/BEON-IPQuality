package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestAPIKeyAuth(t *testing.T) {
	app := fiber.New()
	app.Use(APIKeyAuth("01234567890123456789012345678901"))
	app.Get("/protected", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	app.Get("/health", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	tests := []struct {
		name   string
		path   string
		key    string
		status int
	}{
		{name: "missing", path: "/protected", status: fiber.StatusUnauthorized},
		{name: "wrong", path: "/protected", key: "01234567890123456789012345678902", status: fiber.StatusUnauthorized},
		{name: "exact", path: "/protected", key: "01234567890123456789012345678901", status: fiber.StatusNoContent},
		{name: "health remains public", path: "/health", status: fiber.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", test.path, nil)
			if test.key != "" {
				req.Header.Set("X-API-Key", test.key)
			}
			response, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != test.status {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.status)
			}
		})
	}
}

func TestNilAPIKeyValidatorRejects(t *testing.T) {
	app := fiber.New()
	app.Use(APIKeyAuthWithValidator(nil))
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "any-key")
	response, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.StatusCode, fiber.StatusUnauthorized)
	}
}
