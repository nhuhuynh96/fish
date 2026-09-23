package mqttiface

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/nhuhuynh/iot-fish/esp32_go/internal/domain/sampling"
	samplinguc "github.com/nhuhuynh/iot-fish/esp32_go/internal/usecase/sampling"
)

type Handler struct {
	svc *samplinguc.Service
}

func NewHandler(svc *samplinguc.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) OnCommand(_ string, payload []byte) {
	var dto sampling.Command
	if err := json.Unmarshal(payload, &dto); err != nil {
		log.Printf("[Command] JSON không hợp lệ: %v", err)
		return
	}
	dto.Action = sampling.ActionType(strings.ToLower(strings.TrimSpace(string(dto.Action))))
	dto.FillLevel = sampling.NormalizeFillLevel(dto.FillLevel)
	if dto.Action == "" {
		log.Println("[Command] Thiếu action")
		return
	}
	h.svc.Enqueue(dto)
}
