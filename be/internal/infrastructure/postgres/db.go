package postgres

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

func Open(dsn string, maxOpen int) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	if maxOpen <= 0 {
		maxOpen = 25
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxOpen / 2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	log.Println("[PostgreSQL] Kết nối Database thành công!")

	// Tự động khởi tạo schema
	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return db, nil
}

func initSchema(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS devices (
		id VARCHAR(64) PRIMARY KEY,
		online BOOLEAN DEFAULT false,
		ip VARCHAR(45) DEFAULT '',
		state VARCHAR(32) DEFAULT 'IDLE',
		last_seen TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		rssi INT DEFAULT 0,
		uptime BIGINT DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS measurements (
		id VARCHAR(64) PRIMARY KEY,
		device_id VARCHAR(64) NOT NULL,
		timestamp BIGINT NOT NULL,
		status VARCHAR(32) NOT NULL,
		duration_ms BIGINT DEFAULT 0,
		sensors TEXT[] DEFAULT '{}',
		temperature DOUBLE PRECISION,
		ph DOUBLE PRECISION,
		turbidity DOUBLE PRECISION,
		tds DOUBLE PRECISION,
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_measurements_device_created ON measurements (device_id, created_at DESC);

	CREATE TABLE IF NOT EXISTS sampling_events (
		id VARCHAR(64) PRIMARY KEY,
		device_id VARCHAR(64) NOT NULL,
		stage VARCHAR(32) NOT NULL,
		state VARCHAR(32) NOT NULL,
		message TEXT NOT NULL,
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_events_device_created ON sampling_events (device_id, created_at DESC);
	`
	_, err := db.Exec(query)
	if err != nil {
		return err
	}
	log.Println("[PostgreSQL] Đã khởi tạo bảng và index thành công!")
	return nil
}
