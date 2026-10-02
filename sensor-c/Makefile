.PHONY: help ports build flash monitor clean

# Cổng USB. Không truyền thì lấy cổng đầu tiên đang cắm.
PORT ?= $(shell ls /dev/cu.usb* 2>/dev/null | head -n 1)
BAUD ?= 115200
PIO := $(HOME)/.platformio/penv/bin/pio

help:
	@echo "ESP32-S3 cảm biến (PlatformIO, board esp32-s3-devkitc-1)"
	@echo ""
	@echo "Cổng: $(PORT)"
	@echo ""
	@echo "  make ports     - Liệt kê cổng USB"
	@echo "  make build     - Biên dịch"
	@echo "  make flash     - Biên dịch và nạp"
	@echo "  make monitor   - Mở serial monitor"
	@echo "  make clean     - Xóa bản build"
	@echo ""
	@echo "Ví dụ: make flash PORT=/dev/cu.usbmodem101"

ports:
	@echo "==> Các cổng USB Serial đang kết nối:"
	@ls -l /dev/cu.usb* 2>/dev/null || echo "Không tìm thấy cổng USB nào!"

build:
	$(PIO) run

flash:
	@test -n "$(PORT)" || (echo "Không tìm thấy cổng USB. Chạy make ports hoặc make flash PORT=/dev/cu.usbserial-xxx" && exit 1)
	@echo "==> Nạp vào $(PORT)..."
	$(PIO) run -t upload --upload-port $(PORT)

monitor:
	@test -n "$(PORT)" || (echo "Không tìm thấy cổng USB. Chạy make ports hoặc make monitor PORT=/dev/cu.usbserial-xxx" && exit 1)
	@echo "==> Serial monitor tại $(PORT) ($(BAUD) baud)..."
	$(PIO) device monitor -p $(PORT) -b $(BAUD)

clean:
	$(PIO) run -t clean
