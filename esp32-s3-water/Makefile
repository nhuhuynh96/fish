.PHONY: help ports docker-build flash monitor clean

TARGET ?= esp32s3-generic
# Tự động tìm cổng USB Serial/Modem trên macOS nếu không truyền tham số
PORT ?= $(shell ls /dev/cu.usb* 2>/dev/null | head -n 1)
BAUD ?= 115200

help:
	@echo "ESP32-S3 TinyGo Water Project (bơm / xả / phao)"
	@echo ""
	@echo "Target: $(TARGET)"
	@echo "Cổng đang phát hiện: $(PORT)"
	@echo ""
	@echo "Lệnh có sẵn:"
	@echo "  make ports         - Liệt kê các cổng USB đang cắm"
	@echo "  make docker-build  - Dùng Docker biên dịch ra build/firmware.bin"
	@echo "  make flash         - Nạp file build/firmware.bin vào ESP32-S3"
	@echo "  make monitor       - Mở Serial Monitor xem log"
	@echo "  make clean         - Xóa thư mục build"

ports:
	@echo "==> Các cổng USB Serial đang kết nối:"
	@ls -l /dev/cu.usb* 2>/dev/null || echo "Không tìm thấy cổng USB nào!"

docker-build:
	@mkdir -p build
	@echo "==> Đang biên dịch bằng Docker TinyGo (Target: $(TARGET))..."
	docker run --rm -v "$$(pwd)":/src -w /src tinygo/tinygo:0.42.0 tinygo build -target=$(TARGET) -o build/firmware.bin ./cmd/main.go
	@echo "✓ Biên dịch thành công -> build/firmware.bin"

flash:
	@if [ ! -f build/firmware.bin ]; then \
		echo "Chưa có file build/firmware.bin, đang tự động build..."; \
		$(MAKE) docker-build; \
	fi
	@echo "==> Đang nạp firmware vào ESP32-S3 qua cổng $(PORT)..."
	esptool --chip esp32s3 --port $(PORT) write-flash 0x0 build/firmware.bin

monitor:
	@echo "==> Đang mở Serial Monitor tại $(PORT)..."
	~/.platformio/penv/bin/pio device monitor -p $(PORT) -b $(BAUD)

clean:
	rm -rf build
