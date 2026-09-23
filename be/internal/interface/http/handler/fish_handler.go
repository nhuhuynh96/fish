package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/scheduler"
	"github.com/nhuhuynh/iot-fish/internal/interface/http/respond"
	"github.com/nhuhuynh/iot-fish/internal/pkg/advice"
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
		Level  int    `json:"level"` // inlet: 1 = phao 1, 2 = phao 2
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.BadRequest(c, "invalid body: target and state required")
		return
	}

	target := strings.ToLower(strings.TrimSpace(req.Target))
	if target != "inlet" && target != "drain" {
		respond.BadRequest(c, "target must be \"inlet\" or \"drain\"")
		return
	}

	if err := h.svc.SetPump(c.Request.Context(), deviceID, target, req.State, req.Level); err != nil {
		respond.InternalError(c, err.Error())
		return
	}

	action := "inlet_off"
	if target == "drain" {
		if req.State {
			action = "drain_on"
		} else {
			action = "drain_off"
		}
	} else if req.State {
		action = "inlet_on"
	}
	respond.OK(c, gin.H{
		"message": "Pump command sent",
		"device":  deviceID,
		"target":  target,
		"state":   req.State,
		"level":   req.Level,
		"action":  action,
	})
}

// 2a. Xóa toàn bộ hàng đợi lệnh trên ESP32
func (h *FishHandler) ClearQueue(c *gin.Context) {
	deviceID := c.Param("id")
	if deviceID == "" {
		respond.BadRequest(c, "missing device id")
		return
	}

	if err := h.svc.ClearQueue(c.Request.Context(), deviceID); err != nil {
		respond.InternalError(c, err.Error())
		return
	}

	respond.OK(c, gin.H{
		"message": "Clear queue command sent",
		"device":  deviceID,
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
		h.sched.EnsureDevice(deviceID)
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

// 2b2. Lấy cấu hình lịch đo hiện tại từ backend scheduler
func (h *FishHandler) GetSchedule(c *gin.Context) {
	deviceID := c.Param("id")
	if deviceID == "" {
		respond.BadRequest(c, "missing device id")
		return
	}

	if h.sched == nil {
		respond.OK(c, gin.H{
			"device_id":     deviceID,
			"auto_enabled":  false,
			"temp_interval": 60,
			"ph_interval":   120,
			"turb_interval": 180,
			"tds_interval":  300,
		})
		return
	}

	sched := h.sched.GetDeviceSchedule(deviceID)
	if sched == nil {
		respond.OK(c, gin.H{
			"device_id":     deviceID,
			"auto_enabled":  false,
			"temp_interval": 60,
			"ph_interval":   120,
			"turb_interval": 180,
			"tds_interval":  300,
		})
		return
	}

	respond.OK(c, gin.H{
		"device_id":     deviceID,
		"auto_enabled":  sched.AutoEnabled,
		"temp_interval": int(sched.TempInterval.Seconds()),
		"ph_interval":   int(sched.PhInterval.Seconds()),
		"turb_interval": int(sched.TurbInterval.Seconds()),
		"tds_interval":  int(sched.TdsInterval.Seconds()),
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

// 6. Lấy cấu hình hiệu chuẩn cảm biến
func (h *FishHandler) GetCalibration(c *gin.Context) {
	deviceID := c.Param("id")
	cal, err := h.svc.GetCalibration(c.Request.Context(), deviceID)
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	respond.OK(c, cal)
}

// 7. Cập nhật cấu hình hiệu chuẩn cảm biến
func (h *FishHandler) UpdateCalibration(c *gin.Context) {
	deviceID := c.Param("id")
	var req struct {
		PHNeutralV *float64 `json:"ph_neutral_v"`
		PHSlope    *float64 `json:"ph_slope"`
		TDSTempC   *float64 `json:"tds_temp_c"`
		TDSRefV    *float64 `json:"tds_ref_v"`
		TDSRefPPM  *float64 `json:"tds_ref_ppm"`
		TDSMaxPPM  *float64 `json:"tds_max_ppm"`
		TurbVClear *float64 `json:"turb_v_clear"`
		TurbVDirty *float64 `json:"turb_v_dirty"`
		TurbNTUMax *float64 `json:"turb_ntu_max"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.BadRequest(c, "invalid body")
		return
	}

	cal, err := h.svc.GetCalibration(c.Request.Context(), deviceID)
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	if req.PHNeutralV != nil && *req.PHNeutralV > 0 {
		cal.PHNeutralV = *req.PHNeutralV
	}
	if req.PHSlope != nil && *req.PHSlope > 0 {
		cal.PHSlope = *req.PHSlope
	}
	if req.TDSTempC != nil && *req.TDSTempC > 0 {
		cal.TDSTempC = *req.TDSTempC
	}
	if req.TDSRefV != nil {
		cal.TDSRefV = *req.TDSRefV
	}
	if req.TDSRefPPM != nil {
		cal.TDSRefPPM = *req.TDSRefPPM
	}
	if req.TDSMaxPPM != nil && *req.TDSMaxPPM > 0 {
		cal.TDSMaxPPM = *req.TDSMaxPPM
	}
	if req.TurbVClear != nil && *req.TurbVClear > 0 {
		cal.TurbVClear = *req.TurbVClear
	}
	if req.TurbVDirty != nil && *req.TurbVDirty > 0 {
		cal.TurbVDirty = *req.TurbVDirty
	}
	if req.TurbNTUMax != nil && *req.TurbNTUMax > 0 {
		cal.TurbNTUMax = *req.TurbNTUMax
	}

	if err := h.svc.UpdateCalibration(c.Request.Context(), cal); err != nil {
		respond.InternalError(c, err.Error())
		return
	}

	respond.OK(c, gin.H{
		"message":     "Calibration updated",
		"device":      deviceID,
		"calibration": cal,
	})
}

// 8. Phân tích xu hướng và đề xuất xử lý hồ
func (h *FishHandler) GetAdvice(c *gin.Context) {
	deviceID := c.Param("id")
	if deviceID == "" {
		respond.BadRequest(c, "missing device id")
		return
	}

	var req struct {
		VolumeL   float64 `json:"volume_l"`
		Species   string  `json:"species"`
		HasFilter *bool   `json:"has_filter"`
	}
	_ = c.ShouldBindJSON(&req)

	res, err := h.svc.GetAdvice(c.Request.Context(), deviceID, advice.Profile{
		VolumeL:   req.VolumeL,
		Species:   req.Species,
		HasFilter: req.HasFilter,
	})
	if err != nil {
		if errors.Is(err, advice.ErrLLMNotConfigured) || errors.Is(err, advice.ErrLLMUnavailable) {
			respond.Error(c, http.StatusServiceUnavailable, err.Error())
			return
		}
		respond.InternalError(c, err.Error())
		return
	}
	respond.OK(c, res)
}

func (h *FishHandler) SaveKitReading(c *gin.Context) {
	deviceID := c.Param("id")
	if deviceID == "" {
		respond.BadRequest(c, "missing device id")
		return
	}
	var req struct {
		DOMGL      *float64 `json:"do_mg_l"`
		TANMGL     *float64 `json:"tan_mg_l"`
		MeasuredAt string   `json:"measured_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.BadRequest(c, "JSON không hợp lệ")
		return
	}
	row := &fish.KitReading{
		DeviceID: deviceID,
		DOMGL:    req.DOMGL,
		TANMGL:   req.TANMGL,
	}
	if ts := strings.TrimSpace(req.MeasuredAt); ts != "" {
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			respond.BadRequest(c, "measured_at phải là RFC3339")
			return
		}
		row.MeasuredAt = t
	}
	saved, err := h.svc.SaveKitReading(c.Request.Context(), row)
	if err != nil {
		respond.BadRequest(c, err.Error())
		return
	}
	respond.OK(c, saved)
}

func (h *FishHandler) ListKitReadings(c *gin.Context) {
	deviceID := c.Param("id")
	if deviceID == "" {
		respond.BadRequest(c, "missing device id")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := h.svc.ListKitReadings(c.Request.Context(), deviceID, limit)
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	respond.OK(c, list)
}

// 9. Lấy danh sách thiết bị
func (h *FishHandler) ListDevices(c *gin.Context) {
	list, err := h.svc.ListDevices(c.Request.Context())
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	respond.OK(c, list)
}

func (h *FishHandler) GetPondConfig(c *gin.Context) {
	cfg, err := h.svc.GetPondConfig(c.Request.Context())
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	respond.OK(c, cfg)
}

func (h *FishHandler) UpdatePondConfig(c *gin.Context) {
	var req fish.PondConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.BadRequest(c, "invalid body")
		return
	}
	cfg, err := h.svc.UpdatePondConfig(c.Request.Context(), &req)
	if err != nil {
		respond.InternalError(c, err.Error())
		return
	}
	respond.OK(c, cfg)
}
