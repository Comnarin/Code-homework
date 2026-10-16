package httpapi

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const cidCookie = "cid"

type cidKey struct{}

// cidMiddleware gives every browser a stable anonymous customer ID. The ID is
// a random UUID, so it works as an unguessable bearer value; it is HttpOnly so
// page scripts cannot read it.
func cidMiddleware(newID func() string, secure bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Cookies(cidCookie)
		if _, err := uuid.Parse(id); err != nil {
			id = newID()
			c.Cookie(&fiber.Cookie{
				Name:     cidCookie,
				Value:    id,
				Path:     "/",
				HTTPOnly: true,
				Secure:   secure,
				SameSite: "Lax",
				Expires:  time.Now().Add(365 * 24 * time.Hour),
			})
		}
		c.Locals(cidKey{}, id)
		return c.Next()
	}
}

func customerID(c fiber.Ctx) string {
	id, _ := c.Locals(cidKey{}).(string)
	return id
}
