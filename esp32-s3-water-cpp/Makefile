.PHONY: build flash monitor clean

PIO ?= $(shell command -v pio 2>/dev/null || echo $(HOME)/.platformio/penv/bin/pio)
PORT ?= $(shell ls /dev/cu.usb* 2>/dev/null | head -n 1)

build:
	$(PIO) run -d .

flash:
	@echo "==> Nạp firmware esp32-s3-water (PORT=$(PORT))..."
	$(PIO) run -d . -t upload --upload-port $(PORT)

monitor:
	$(PIO) device monitor -p $(PORT) -b 115200

clean:
	$(PIO) run -d . -t clean
