package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func CORS(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		o := origin
		if o == "" {
			o = "*"
		}
		c.Header("Access-Control-Allow-Origin", o)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		c.Header("Access-Control-Expose-Headers", "Content-Length")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
