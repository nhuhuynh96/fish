import React from 'react';

export default function EventLogStream({ events }) {
  return (
    <div className="glass-card">
      <div className="section-title">
        <svg width="18" height="18" fill="var(--primary)" viewBox="0 0 24 24">
          <path d="M20 2H4c-1.1 0-2 .9-2 2v18l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zM6 9h12v2H6V9zm8 5H6v-2h8v2zm4-6H6V6h12v2z" />
        </svg>
        Luồng Sự Kiện Realtime (Event Stream)
      </div>

      <div className="event-log-container">
        {events && events.length > 0 ? (
          events.map((e, idx) => {
            const timeStr = e.created_at
              ? new Date(e.created_at).toLocaleTimeString()
              : new Date().toLocaleTimeString();

            let badgeColor = 'var(--primary)';
            let badgeBg = 'rgba(56, 189, 248, 0.12)';
            let stageLabel = e.stage || e.state || 'EVENT';

            if (e.stage === 'appended') {
              badgeColor = '#c084fc'; // Purple glow for smart queue append
              badgeBg = 'rgba(192, 132, 252, 0.18)';
              stageLabel = 'GỘP LỆNH (APPEND)';
            } else if (e.stage === 'queued') {
              badgeColor = 'var(--warning)';
              badgeBg = 'rgba(251, 191, 36, 0.18)';
              stageLabel = 'XẾP HÀNG (QUEUED)';
            } else if (e.stage === 'schedule_updated' || e.stage === 'auto_toggle') {
              badgeColor = '#38bdf8';
              badgeBg = 'rgba(56, 189, 248, 0.18)';
              stageLabel = 'LỊCH ĐO';
            } else if (e.stage === 'idle' || e.stage === 'measuring_complete') {
              badgeColor = 'var(--success)';
              badgeBg = 'rgba(52, 211, 153, 0.18)';
            }

            return (
              <div key={idx} className="event-item">
                <span className="event-time">[{timeStr}]</span>
                <span
                  style={{
                    fontSize: '10px',
                    fontWeight: 800,
                    textTransform: 'uppercase',
                    color: badgeColor,
                    background: badgeBg,
                    padding: '2px 8px',
                    borderRadius: '6px',
                    border: `1px solid ${badgeColor}40`,
                    whiteSpace: 'nowrap',
                  }}
                >
                  {stageLabel}
                </span>
                <span className="event-msg">{e.message}</span>
              </div>
            );
          })
        ) : (
          <div style={{ color: 'var(--text-dim)', fontSize: '13px', padding: '20px', textAlign: 'center' }}>
            Chưa có sự kiện nào. Hãy nhấn nút đo để xem tiến trình realtime!
          </div>
        )}
      </div>
    </div>
  );
}
