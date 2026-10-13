package middleware

import (
	"crypto/sha256"
	"crypto/subtle"

	"github.com/gofiber/fiber/v2"
)

// APIKeyAuth validates an API key against the configured secret. Hashing both
// values first keeps the constant-time comparison length fixed.
func APIKeyAuth(expectedKey string) fiber.Handler {
	expectedHash := sha256.Sum256([]byte(expectedKey))
	return APIKeyAuthWithValidator(func(key string) bool {
		providedHash := sha256.Sum256([]byte(key))
		return subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
	})
}

// ValidateAPIKeyFn is the function type for API key validation.
// Replace this at startup with a real database-backed implementation.
type ValidateAPIKeyFn func(key string) bool

// APIKeyAuthWithValidator delegates validation to fn. A missing validator
// rejects every request so callers cannot accidentally enable permissive auth.
func APIKeyAuthWithValidator(fn ValidateAPIKeyFn) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Allow health checks without API key
		if c.Path() == "/health" || c.Path() == "/metrics" {
			return c.Next()
		}

		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "missing_api_key",
				"message": "API key is required. Include X-API-Key header.",
			})
		}

		if fn == nil || !fn(apiKey) {
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

// AdminKeyAuth disables administration when no dedicated key is configured.
func AdminKeyAuth(key string) fiber.Handler {
	if key == "" {
		return func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNotFound) }
	}
	return APIKeyAuth(key)
}
