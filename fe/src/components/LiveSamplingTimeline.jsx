import React from 'react';

export default function LiveSamplingTimeline({ jobPhase, latestEvent }) {
  const steps = [
    { key: 'fill', label: '1. Chỉnh mực', desc: 'Nhiệt độ và pH: phao 1. TDS: phao 2' },
    { key: 'measure', label: '2. Bật nguồn và đo', desc: 'Từng sensor' },
    { key: 'final_drain', label: '3. Xả 90s', desc: 'Sau khi đo xong' },
  ];

  const getStepStatus = (index) => {
    if (jobPhase < 0) return '';
    if (jobPhase === index) return 'active';
    if (jobPhase > index) return 'completed';
    return '';
  };

  return (
    <div className="glass-card" style={{ marginBottom: '24px' }}>
      <div className="section-title">
        <svg width="18" height="18" fill="var(--primary)" viewBox="0 0 24 24">
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z" />
        </svg>
        Tiến Trình Đo Cảm Biến
      </div>
      <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '12px' }}>
        Lịch đo tự động: pH và nhiệt độ ở phao 1 (đo pH trước), TDS ở phao 2. Thiếu thì bơm, dư thì xả. Mỗi sensor bật nguồn, chờ (pH 30 giây), đọc rồi tắt; pH và TDS không bật cùng lúc. Đo xong xả 90 giây.
      </p>

      <div className="timeline-stepper">
        {steps.map((s, idx) => {
          const status = getStepStatus(idx);
          return (
            <div key={s.key} className={`step-item ${status}`}>
              <div className="step-circle">
                {status === 'completed' ? '✓' : idx + 1}
              </div>
              <div className="step-label">{s.label}</div>
            </div>
          );
        })}
      </div>

      {latestEvent && (
        <div
          style={{
            marginTop: '16px',
            padding: '10px 16px',
            borderRadius: '12px',
            background: 'rgba(56, 189, 248, 0.08)',
            border: '1px solid rgba(56, 189, 248, 0.2)',
            fontSize: '13px',
            display: 'flex',
            alignItems: 'center',
            gap: '10px',
          }}
        >
          <span style={{ color: 'var(--primary)', fontWeight: 700 }}>[Realtime Event]:</span>
          <span style={{ color: '#fff' }}>{latestEvent.message}</span>
        </div>
      )}
    </div>
  );
}
