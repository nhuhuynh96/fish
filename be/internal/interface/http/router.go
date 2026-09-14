package httpiface

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/events"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/scheduler"
	"github.com/nhuhuynh/iot-fish/internal/interface/http/handler"
	"github.com/nhuhuynh/iot-fish/internal/interface/http/middleware"
	fishuc "github.com/nhuhuynh/iot-fish/internal/usecase/fish"
)

type RouterDeps struct {
	FishSvc    *fishuc.Service
	Sched      *scheduler.Scheduler
	Hub        *events.Hub
	CORSOrigin string
}

func NewRouter(deps RouterDeps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS(deps.CORSOrigin))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "iot-fish-backend"})
	})

	// WebSocket endpoint for Realtime UI Updates
	r.GET("/ws", func(c *gin.Context) {
		deps.Hub.HandleWS(c.Writer, c.Request)
	})

	fishHandler := handler.NewFishHandler(deps.FishSvc, deps.Sched)

	api := r.Group("/api")
	{
		devices := api.Group("/devices")
		{
			devices.GET("", fishHandler.ListDevices)
			devices.GET("/:id/latest", fishHandler.GetLatest)
			devices.GET("/:id/history", fishHandler.ListHistory)
			devices.GET("/:id/events", fishHandler.ListEvents)
			devices.POST("/:id/measure", fishHandler.Measure)
			devices.POST("/:id/pump", fishHandler.Pump)
			devices.POST("/:id/schedule", fishHandler.SetSchedule)
			devices.POST("/:id/auto", fishHandler.ToggleAuto)
			devices.GET("/:id/calibration", fishHandler.GetCalibration)
			devices.PUT("/:id/calibration", fishHandler.UpdateCalibration)
		}
	}

	return r
}
