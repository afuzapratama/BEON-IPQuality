package metrics

import (
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/valyala/fasthttp"

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

func TestHTTPLabelsSurviveRequestBufferReuse(t *testing.T) {
	HTTPRequestsTotal.Reset()
	HTTPRequestDuration.Reset()
	defer HTTPRequestsTotal.Reset()
	defer HTTPRequestDuration.Reset()
	app := fiber.New()
	app.Use(HTTPMiddleware())
	app.All("/stable-labels", func(c *fiber.Ctx) error { return c.SendStatus(200) })
	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/stable-labels")
	ctx.Request.Header.SetMethod("POST")
	app.Handler()(&ctx)
	// Reuse the same method buffer, just as fasthttp does across requests.
	ctx.Request.Header.SetMethod("GET")
	app.Handler()(&ctx)
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("metrics gather after reused request: %v", err)
	}
	methods := map[string]bool{}
	for _, family := range families {
		if family.GetName() != "ipquality_http_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "method" {
					methods[label.GetValue()] = true
				}
			}
		}
	}
	if len(methods) != 2 || !methods["POST"] || !methods["GET"] {
		t.Fatalf("retained labels changed: %v", methods)
	}
}
