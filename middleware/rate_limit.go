package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type rateWindow struct {
	count  int
	starts time.Time
}

// RateLimit bounds expensive per-user actions in one server process.
func RateLimit(max int, duration time.Duration, key func(*fiber.Ctx) string) fiber.Handler {
	var mu sync.Mutex
	entries := map[string]rateWindow{}
	go func() {
		ticker := time.NewTicker(duration)
		defer ticker.Stop()
		for now := range ticker.C {
			mu.Lock()
			for k, entry := range entries {
				if now.Sub(entry.starts) >= duration {
					delete(entries, k)
				}
			}
			mu.Unlock()
		}
	}()
	return func(c *fiber.Ctx) error {
		now := time.Now()
		identity := key(c)
		mu.Lock()
		entry := entries[identity]
		if entry.starts.IsZero() || now.Sub(entry.starts) >= duration {
			entry = rateWindow{starts: now}
		}
		if entry.count >= max {
			mu.Unlock()
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "操作过于频繁，请稍后再试", "message": "操作过于频繁，请稍后再试"})
		}
		entry.count++
		entries[identity] = entry
		mu.Unlock()
		return c.Next()
	}
}
