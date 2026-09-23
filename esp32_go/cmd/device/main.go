package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nhuhuynh/iot-fish/esp32_go/internal/infrastructure/hwsim"
	inframqtt "github.com/nhuhuynh/iot-fish/esp32_go/internal/infrastructure/mqtt"
	mqttiface "github.com/nhuhuynh/iot-fish/esp32_go/internal/interface/mqtt"
	"github.com/nhuhuynh/iot-fish/esp32_go/internal/pkg/config"
	samplinguc "github.com/nhuhuynh/iot-fish/esp32_go/internal/usecase/sampling"
)

func main() {
	cfg := config.Load()
	log.Println("==================================================")
	log.Println("   ESP32 FIRMWARE (Go, hexagonal) — SIMULATOR     ")
	log.Printf("   Device: %s | MQTT: %s\n", cfg.DeviceID, cfg.MQTTBroker)
	log.Println("==================================================")

	hw := hwsim.NewChamber()
	svc := samplinguc.New(cfg.DeviceID, hw, nil)
	handler := mqttiface.NewHandler(svc)

	bus, err := inframqtt.NewClient(cfg, handler.OnCommand)
	if err != nil {
		log.Fatalf("[MQTT] connect: %v", err)
	}
	defer bus.Close()
	svc.SetBus(bus)
	svc.Begin()

	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	tel := time.NewTicker(30 * time.Second)
	defer tel.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case now := <-tick.C:
			svc.Tick(now)
		case <-tel.C:
			log.Printf("[Main] telemetry state=%s queue=%d", svc.StateName(), svc.QueueLen())
		case <-stop:
			log.Println("[Main] dừng device")
			return
		}
	}
}
