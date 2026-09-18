package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nhuhuynh/iot-fish/internal/domain/fish"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/events"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/memory"
	inframqtt "github.com/nhuhuynh/iot-fish/internal/infrastructure/mqtt"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/postgres"
	"github.com/nhuhuynh/iot-fish/internal/infrastructure/scheduler"
	httpapi "github.com/nhuhuynh/iot-fish/internal/interface/http"
	mqttiface "github.com/nhuhuynh/iot-fish/internal/interface/mqtt"
	"github.com/nhuhuynh/iot-fish/internal/pkg/config"
	fishuc "github.com/nhuhuynh/iot-fish/internal/usecase/fish"
)

func main() {
	cfg := config.Load()

	log.Println("==================================================")
	log.Println("      FISH TANK IOT BACKEND (HEXAGONAL ARCH)      ")
	log.Printf("      Port: %s | MQTT Broker: %s\n", cfg.AppPort, cfg.MQTTBroker)
	log.Println("==================================================")

	// 1. Khởi tạo Storage Repository (PostgreSQL hoặc Memory Store)
	var measRepo fish.MeasurementRepository
	var eventRepo fish.EventRepository
	var devRepo fish.DeviceRepository
	var calRepo fish.CalibrationRepository

	if cfg.DBDSN != "" {
		log.Println("[Storage] Đang kết nối PostgreSQL:", cfg.DBDSN)
		db, err := postgres.Open(cfg.DBDSN, 20)
		if err != nil {
			log.Fatalf("[Storage] Lỗi kết nối PostgreSQL: %v", err)
		}
		defer db.Close()

		pgStore := postgres.NewStore(db)
		measRepo = pgStore
		eventRepo = pgStore
		devRepo = pgStore
		calRepo = pgStore
	} else {
		log.Println("[Storage] Sử dụng In-Memory Store (Chưa cấu hình DB_DSN)")
		memStore := memory.NewStore()
		measRepo = memStore
		eventRepo = memStore
		devRepo = memStore
		calRepo = memStore
	}

	// 2. Khởi tạo WebSocket Realtime Hub
	hub := events.NewHub()

	// 3. Khởi tạo MQTT Client & Command Publisher
	mqttClient, err := inframqtt.NewClient(cfg)
	if err != nil {
		log.Printf("[Warning] Không thể kết nối MQTT Broker ngay: %v (Client sẽ tự thử lại)", err)
	} else {
		defer mqttClient.Close()
	}

	pub := inframqtt.NewCommandPublisher(mqttClient)

	// 4. Khởi tạo Core Service (Usecase Layer)
	fishSvc := fishuc.NewService(measRepo, eventRepo, devRepo, calRepo, pub, hub)

	sched := scheduler.NewScheduler(fishSvc)
	sched.Start()
	defer sched.Stop()

	// Đăng ký lịch mặc định (Auto TẮT) cho mọi device đã có trong DB — không hardcode ID
	if devices, err := fishSvc.ListDevices(context.Background()); err != nil {
		log.Printf("[Scheduler] Không load được danh sách device: %v", err)
	} else {
		for _, d := range devices {
			sched.EnsureDevice(d.ID)
		}
		log.Printf("[Scheduler] Đã đăng ký lịch mặc định cho %d device từ DB", len(devices))
	}

	// Khi ESP32 online / telemetry → tự thêm vào scheduler nếu chưa có
	fishSvc.SetScheduleRegistrar(sched)
	// 5. Khởi động MQTT Subscriber lắng nghe các topic từ ESP32
	if mqttClient != nil {
		sub := mqttiface.NewSubscriber(mqttClient, fishSvc)
		if err := sub.Start(); err != nil {
			log.Printf("[Warning] MQTT subscribe error: %v", err)
		}
	}

	// 6. Khởi tạo HTTP & WebSocket Router
	router := httpapi.NewRouter(httpapi.RouterDeps{
		FishSvc:    fishSvc,
		Sched:      sched,
		Hub:        hub,
		CORSOrigin: cfg.CORSOrigin,
	})

	srv := &http.Server{
		Addr:         ":" + cfg.AppPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("[HTTP API] Server đang lắng nghe tại http://0.0.0.0:%s", cfg.AppPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[HTTP API] Server fatal error: %v", err)
		}
	}()

	// Xử lý Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("[Main] Đang dừng server an toàn...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Println("[Main] Server đã dừng hoàn tất.")
}
