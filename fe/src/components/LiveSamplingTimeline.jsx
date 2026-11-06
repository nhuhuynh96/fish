import React from 'react';

export default function LiveSamplingTimeline({ jobPhase, latestEvent }) {
  const steps = [
    { key: 'drain', label: '1. Xả nước cũ', desc: 'Theo thời gian' },
    { key: 'fill', label: '2. Bơm đầy + chờ ổn định', desc: 'Tới khi phao đầy' },
    { key: 'measure', label: '3. Đo', desc: 'pH + nhiệt độ, rồi TDS' },
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
        Lịch đo tự động: xả nước cũ theo thời gian, kiểm tra phao đã tắt, bơm nước mới tới khi phao đầy, chờ nước ổn định, đo pH và nhiệt độ (cảm biến RS485), rồi TDS. Đo xong giữ nước trong bình để đầu dò pH luôn ngập. Thời gian xả, bơm, chờ chỉnh trong tab lịch đo.
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
