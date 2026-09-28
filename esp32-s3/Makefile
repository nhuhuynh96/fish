.PHONY: build flash monitor clean

PORT ?= $(shell ls /dev/cu.usb* 2>/dev/null | head -n 1)

build:
	pio run -d .

flash:
	@echo "==> Đang nạp firmware vào ESP32-S3 (PORT=$(PORT))..."
	pio run -d . -t upload --upload-port $(PORT)

monitor:
	pio device monitor -p $(PORT) -b 115200

clean:
	pio run -d . -t clean
