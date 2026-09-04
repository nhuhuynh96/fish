package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/scheduler"
	"github.com/nhuhuynh/iot-fish/internal/interface/http/respond"
	fishuc "github.com/nhuhuynh/iot-fish/internal/usecase/fish"
)

type FishHandler struct {
	svc   *fishuc.Service
	sched *scheduler.Scheduler
}

func NewFishHandler(svc *fishuc.Service, sched *scheduler.Scheduler) *FishHandler {
	return &FishHandler{svc: svc, sched: sched}
}

// 1. Kích hoạt đo chỉ số
func (h *FishHandler) Measure(c *gin.Context) {
	deviceID := c.Param("id")
	if deviceID == "" {
		respond.BadRequest(c, "missing device id")
		return
	}

	var req struct {
		Sensors []string `json:"sensors"`
	}
	_ = c.ShouldBindJSON(&req)

	if len(req.Sensors) == 0 {
		req.Sensors = []string{"all"}
	}

	if err := h.svc.TriggerMeasurement(c.Request.Context(), deviceID, req.Sensors); err != nil {
		respond.InternalError(c, err.Error())
		return
	}

	respond.OK(c, gin.H{
		"message": "Measurement command sent to ESP32",
		"device":  deviceID,
		"sensors": req.Sensors,
	})
}

// 2. Điều khiển bơm nạp / bơm xả
func (h *FishHandler) Pump(c *gin.Context) {
	deviceID := c.Param("id")
	if deviceID == "" {
		respond.BadRequest(c, "missing device id")
		return
	}

	var req struct {
		Target string `json:"target"` // "inlet" | "drain"
		State  bool   `json:"state"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.BadRequest(c, "invalid body: target and state required")
		return
	}

	if err := h.svc.SetPump(c.Request.Context(), deviceID, req.Target, req.State); err != nil {
		respond.InternalError(c, err.Error())
		return
	}

	respond.OK(c, gin.H{
		"message": "Pump command sent",
		"device":  deviceID,
		"target":  req.Target,
		"state":   req.State,
	})
}

// 2b. Cấu hình lịch đo tự động định kỳ
func (h *FishHandler) SetSchedule(c *gin.Context) {
	deviceID := c.Param("id")
	if deviceID == "" {
		respond.BadRequest(c, "missing device id")
		return
	}

	var req struct {
		AutoEnabled  bool `json:"auto_enabled"`
		TempInterval int  `json:"temp_interval"`
		PhInterval   int  `json:"ph_interval"`
		TurbInterval int  `json:"turb_interval"`
		TdsInterval  int  `json:"tds_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.BadRequest(c, "invalid body")
		return
	}

	// Default fallback values if 0
	if req.TempInterval <= 0 {
		req.TempInterval = 60
	}
	if req.PhInterval <= 0 {
		req.PhInterval = 120
	}
	if req.TurbInterval <= 0 {
		req.TurbInterval = 180
	}
	if req.TdsInterval <= 0 {
		req.TdsInterval = 300
	}

	if h.sched != nil {
		h.sched.SetDeviceSchedule(deviceID, req.AutoEnabled, req.TempInterval, req.PhInterval, req.TurbInterval, req.TdsInterval)
	}

	if err := h.svc.SetSchedule(c.Request.Context(), deviceID, req.AutoEnabled, req.TempInterval, req.PhInterval, req.TurbInterval, req.TdsInterval); err != nil {
		respond.InternalError(c, err.Error())
		return
	}

	respond.OK(c, gin.H{
		"message":       "Schedule updated successfully",
		"device":        deviceID,
		"auto_enabled":  req.AutoEnabled,
		"temp_interval": req.TempInterval,
		"ph_interval":   req.PhInterval,
		"turb_interval": req.TurbInterval,
		"tds_interval":  req.TdsInterval,
	})
}

// 2c. Bật / tắt tự động đo
func (h *FishHandler) ToggleAuto(c *gin.Context) {
	deviceID := c.Param("id")
	if deviceID == "" {
		respond.BadRequest(c, "missing device id")
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.BadRequest(c, "invalid body: enabled required")
		return
	}

	if h.sched != nil {
		h.sched.ToggleAuto(deviceID, req.Enabled)
	}

	if err := h.svc.ToggleAuto(c.Request.Context(), deviceID, req.Enabled); err != nil {
		respond.InternalError(c, err.Error())
		return
	}

	respond.OK(c, gin.H{
		"message": "Auto measurement toggled",
		"device":  deviceID,
		"enabled": req.Enabled,
	})
}

// 3. Lấy dữ liệu đo mới nhất
func (h *FishHandler) GetLatest(c *gin.Context) {
	deviceID := c.Param("id")
	data, err := h.svc.GetLatest(c.Request.Context(), deviceID)
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	if data == nil {
		respond.OK(c, nil)
		return
	}
	respond.OK(c, data)
}

// 4. Lấy lịch sử đo
func (h *FishHandler) ListHistory(c *gin.Context) {
	deviceID := c.Param("id")
	limitStr := c.DefaultQuery("limit", "20")
	limit, _ := strconv.Atoi(limitStr)

	list, err := h.svc.ListHistory(c.Request.Context(), deviceID, limit)
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	respond.OK(c, list)
}

// 5. Lấy danh sách sự kiện tiến trình
func (h *FishHandler) ListEvents(c *gin.Context) {
	deviceID := c.Param("id")
	limitStr := c.DefaultQuery("limit", "30")
	limit, _ := strconv.Atoi(limitStr)

	list, err := h.svc.ListEvents(c.Request.Context(), deviceID, limit)
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	respond.OK(c, list)
}

// 6. Lấy danh sách thiết bị
func (h *FishHandler) ListDevices(c *gin.Context) {
	list, err := h.svc.ListDevices(c.Request.Context())
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	respond.OK(c, list)
}
