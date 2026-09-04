package respond

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func JSON(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data})
}

func OK(c *gin.Context, data any) {
	JSON(c, http.StatusOK, data)
}

func Error(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func BadRequest(c *gin.Context, message string) {
	Error(c, http.StatusBadRequest, message)
}

func InternalError(c *gin.Context, message string) {
	Error(c, http.StatusInternalServerError, message)
}
