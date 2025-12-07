package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// APIKeyAuth middleware validates API keys
func APIKeyAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")

		// Allow health checks without API key
		if c.Path() == "/health" {
			return c.Next()
		}

		if apiKey == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "missing_api_key",
				"message": "API key is required. Include X-API-Key header.",
			})
		}

		// TODO: Validate API key against database
		// For now, accept any non-empty key
		valid := validateAPIKey(apiKey)
		if !valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "invalid_api_key",
				"message": "The provided API key is invalid or expired.",
			})
		}

		// Store API key info in context for later use
		c.Locals("api_key", apiKey)

		return c.Next()
	}
}

// validateAPIKey checks if an API key is valid
// TODO: Implement actual validation against database
func validateAPIKey(key string) bool {
	// Placeholder: accept any key that starts with "beon_"
	if len(key) < 5 {
		return false
	}

	// In production, this would:
	// 1. Check if key exists in database
	// 2. Check if key is enabled
	// 3. Check if key has not expired
	// 4. Check rate limits for the key

	return true
}

// RateLimitByAPIKey applies rate limiting based on API key tier
func RateLimitByAPIKey() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// apiKey := c.Locals("api_key")

		// TODO: Get rate limit from API key tier
		// For now, use default rate limit

		return c.Next()
	}
}

// RequestLogger logs incoming requests
func RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// TODO: Log to ClickHouse for analytics

		return c.Next()
	}
}
