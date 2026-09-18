import React from 'react';

export default function DeviceLogStream({ logs }) {
  return (
    <div className="glass-card">
      <div className="section-title">
        <svg width="18" height="18" fill="var(--warning)" viewBox="0 0 24 24">
          <path d="M20 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zm0 14H4V8h16v10zM6 10h2v2H6zm0 4h8v2H6z" />
        </svg>
        Serial Log từ ESP32 (MQTT …/log)
      </div>
      <p style={{ color: 'var(--text-muted)', fontSize: '0.8rem', marginBottom: '12px' }}>
        Log firmware gửi qua topic <code>fish/&lt;device_id&gt;/log</code> — xem được khi board chạy nguồn ngoài, không cần USB.
      </p>

      <div className="device-log-container">
        {logs && logs.length > 0 ? (
          logs.map((line, idx) => {
            const timeStr = line.created_at
              ? new Date(line.created_at).toLocaleTimeString()
              : '';
            return (
              <div key={idx} className="device-log-line">
                <span className="event-time">[{timeStr}]</span>
                <span className="device-log-msg">{line.msg}</span>
              </div>
            );
          })
        ) : (
          <div style={{ color: 'var(--text-dim)', fontSize: '13px', padding: '20px', textAlign: 'center' }}>
            Chưa có log. Flash firmware mới rồi gửi lệnh đo — log sẽ hiện ở đây.
          </div>
        )}
      </div>
    </div>
  );
}
