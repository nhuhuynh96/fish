import React from 'react';

export default function Header({
  devices,
  selectedDevice,
  setSelectedDevice,
  wsConnected,
  deviceStatus,
  currentState,
}) {
  const isOnline = deviceStatus?.online ?? false;
  const isMeasuring = currentState && currentState !== 'IDLE' && currentState !== 'UNKNOWN';

  return (
    <header className="header-wrapper">
      <div className="brand-section">
        <div className="brand-logo">
          <svg viewBox="0 0 24 24">
            <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v.93zm6.9-2.54c-.26-.81-1-1.39-1.9-1.39h-1v-3c0-.55-.45-1-1-1H8v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z" />
          </svg>
        </div>
        <div className="brand-title">
          <h1>Smart Fish Tank Controller</h1>
          <p>Hệ thống Quan trắc & Lấy mẫu Tự động Hồ cá</p>
        </div>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: '12px', flexWrap: 'wrap' }}>
        {/* Device Selector */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <label style={{ fontSize: '12px', color: 'var(--text-muted)' }}>Thiết bị:</label>
          <select
            value={selectedDevice}
            onChange={(e) => setSelectedDevice(e.target.value)}
            style={{
              background: 'rgba(15, 23, 42, 0.8)',
              border: '1px solid var(--card-border)',
              borderRadius: '10px',
              padding: '6px 12px',
              color: '#fff',
              fontSize: '13px',
              fontFamily: 'var(--font-mono)',
              outline: 'none',
              cursor: 'pointer',
            }}
          >
            {devices.length > 0 ? (
              devices.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.id} {d.online ? '🟢' : '⚪'}
                </option>
              ))
            ) : (
              <option value={selectedDevice}>{selectedDevice} (Default)</option>
            )}
          </select>
        </div>

        {/* ESP32 Status Badge */}
        <div className="device-status-badge">
          <div
            className={`status-dot ${
              isMeasuring ? 'measuring' : isOnline ? 'online' : 'offline'
            }`}
          />
          <span style={{ fontWeight: 600 }}>
            {isMeasuring ? `Đang đo: ${currentState}` : isOnline ? 'ESP32 Online' : 'ESP32 Sẵn sàng'}
          </span>
        </div>

        {/* WS Connection Indicator */}
        <div
          style={{
            fontSize: '11px',
            padding: '6px 10px',
            borderRadius: '20px',
            background: wsConnected ? 'rgba(52, 211, 153, 0.1)' : 'rgba(248, 113, 113, 0.1)',
            color: wsConnected ? 'var(--success)' : 'var(--danger)',
            border: `1px solid ${wsConnected ? 'rgba(52, 211, 153, 0.2)' : 'rgba(248, 113, 113, 0.2)'}`,
          }}
        >
          {wsConnected ? '⚡ Realtime Live' : '❌ Mất kết nối WS'}
        </div>
      </div>
    </header>
  );
}
