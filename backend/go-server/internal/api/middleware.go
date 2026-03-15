package api

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// LoggingMiddleware logs request details
func LoggingMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		// Process request
		err := c.Next()

		// Log after request
		log.Printf("[%s] %s %s - %d (%v)",
			c.Method(),
			c.Path(),
			c.IP(),
			c.Response().StatusCode(),
			time.Since(start),
		)

		return err
	}
}

// CORSMiddleware configures CORS headers
func CORSMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Method() == "OPTIONS" {
			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}

// AuthMiddleware validates JWT tokens
func AuthMiddleware() fiber.Handler {
	jwtSecret := os.Getenv("JWT_SECRET")

	return func(c *fiber.Ctx) error {
		// Skip auth for health check and WebSocket upgrade
		if c.Path() == "/health" || len(c.Path()) >= 3 && c.Path()[:3] == "/ws" {
			return c.Next()
		}

		// Dev mode: skip auth if JWT_SECRET is not configured
		if jwtSecret == "" {
			return c.Next()
		}

		// Get token from Authorization header
		auth := c.Get("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing or invalid Authorization header",
			})
		}

		token := strings.TrimPrefix(auth, "Bearer ")

		// Validate JWT (HMAC-SHA256)
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token format",
			})
		}

		// Verify signature: HMAC-SHA256(header.payload, secret)
		signingInput := parts[0] + "." + parts[1]
		mac := hmac.New(sha256.New, []byte(jwtSecret))
		mac.Write([]byte(signingInput))
		expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token signature",
			})
		}

		// Token is valid — decode payload for user info
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err == nil {
			c.Locals("jwt_payload", string(payload))
		}

		return c.Next()
	}
}

// RecoveryMiddleware recovers from panics
func RecoveryMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic recovered: %v", r)
				c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Internal server error",
				})
			}
		}()
		return c.Next()
	}
}

// TwilioSignatureMiddleware validates Twilio request signatures
// See: https://www.twilio.com/docs/usage/security#validating-requests
func TwilioSignatureMiddleware(authToken, publicBaseURL string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Log all incoming Twilio webhook requests
		log.Printf("Twilio webhook received: %s %s", c.Method(), c.Path())

		// Skip validation if auth token is not configured (development mode)
		if authToken == "" {
			log.Println("Warning: Twilio signature validation skipped - no auth token configured")
			return c.Next()
		}

		signature := c.Get("X-Twilio-Signature")
		if signature == "" {
			log.Printf("Twilio signature missing for %s", c.Path())
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Missing Twilio signature",
			})
		}

		// Build the URL that Twilio signed
		url := publicBaseURL + c.OriginalURL()

		// Get POST parameters and sort them
		var params []string
		c.Request().PostArgs().VisitAll(func(key, value []byte) {
			params = append(params, string(key)+"="+string(value))
		})
		sort.Strings(params)

		// Build the data string: URL + sorted params
		data := url
		for _, param := range params {
			parts := strings.SplitN(param, "=", 2)
			if len(parts) == 2 {
				data += parts[0] + parts[1]
			}
		}

		// Calculate expected signature
		mac := hmac.New(sha1.New, []byte(authToken))
		mac.Write([]byte(data))
		expectedSig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

		// Compare signatures
		if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
			log.Printf("Twilio signature mismatch for %s", c.Path())
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Invalid Twilio signature",
			})
		}

		return c.Next()
	}
}

// TwilioSignatureMiddlewareFromEnv creates Twilio signature middleware using environment variables
func TwilioSignatureMiddlewareFromEnv() fiber.Handler {
	authToken := os.Getenv("TWILIO_AUTH_TOKEN")
	publicBaseURL := os.Getenv("PUBLIC_BASE_URL")
	return TwilioSignatureMiddleware(authToken, publicBaseURL)
}
