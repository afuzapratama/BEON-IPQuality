package metrics

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestHTTPMiddlewareAndIPStats(t *testing.T) {
	before := Stats()
	app := fiber.New()
	app.Use(HTTPMiddleware())
	app.Get("/check/:ip", func(c *fiber.Ctx) error {
		RecordIPCheck("clean", false, 0.1, "mmdb")
		return c.SendStatus(fiber.StatusOK)
	})
	response, err := app.Test(httptest.NewRequest("GET", "/check/8.8.8.8", nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	after := Stats()
	if after.TotalRequests != before.TotalRequests+1 || after.TotalIPs != before.TotalIPs+1 {
		t.Fatalf("stats did not increment: before=%#v after=%#v", before, after)
	}
}
